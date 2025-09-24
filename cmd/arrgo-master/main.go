package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/arrgo/arrgo/internal/api/grpcapi"
	"github.com/arrgo/arrgo/internal/api/httpapi"
	"github.com/arrgo/arrgo/internal/api/librarypb"
	"github.com/arrgo/arrgo/internal/api/workerpb"
	"github.com/arrgo/arrgo/internal/auth"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/worker"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

type masterConfig struct {
	HTTPAddr            string
	GRPCAddr            string
	DatabaseURL         string
	RedisAddr           string
	RedisUsername       string
	RedisPassword       string
	RedisDB             int
	LogLevel            slog.Level
	DisableAuth         bool
	ShutdownTimeout     time.Duration
	BootstrapAdminToken string
	BootstrapAdminLabel string
	BootstrapAdminFile  string
}

func main() {
	cfg := loadConfig()
	logger := newLogger(cfg.LogLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("arrgo master exited with error", "error", err)
		os.Exit(1)
	}
	logger.Info("arrgo master shutdown complete")
}

func run(ctx context.Context, cfg masterConfig, logger *slog.Logger) error {
	baseCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger.Info("starting arrgo master", "http_addr", cfg.HTTPAddr, "grpc_addr", cfg.GRPCAddr, "disable_auth", cfg.DisableAuth)

	redisClient, err := connectRedis(baseCtx, cfg, logger)
	if err != nil {
		return err
	}
	if redisClient != nil {
		defer func() {
			_ = redisClient.Close()
		}()
	}

	pool, err := connectPostgres(baseCtx, cfg, logger)
	if err != nil {
		return err
	}
	if pool != nil {
		defer pool.Close()
	}

	libraryLogger := logger.With("component", "library")
	workerLogger := logger.With("component", "worker")

	var libRepo library.Repository
	if pool != nil {
		postgresRepo, err := library.NewPostgresRepository(pool)
		if err != nil {
			return fmt.Errorf("initialize library repository: %w", err)
		}
		if err := postgresRepo.EnsureSchema(baseCtx); err != nil {
			return fmt.Errorf("ensure library schema: %w", err)
		}
		libRepo = postgresRepo
	} else {
		libRepo = library.NewInMemoryRepository()
		libraryLogger.Warn("postgres url not provided, using in-memory library repository")
	}

	libCfg := library.Config{
		Repository:  libRepo,
		RedisClient: redisClient,
		Logger:      libraryLogger,
	}
	libraryService, err := library.NewService(libCfg)
	if err != nil {
		return fmt.Errorf("initialize library service: %w", err)
	}

	var repo worker.Repository
	if pool != nil {
		postgresRepo, err := worker.NewPostgresRepository(pool)
		if err != nil {
			return fmt.Errorf("initialize worker repository: %w", err)
		}
		if err := postgresRepo.EnsureSchema(baseCtx); err != nil {
			return fmt.Errorf("ensure worker schema: %w", err)
		}
		repo = postgresRepo
	} else {
		repo = worker.NewInMemoryRepository()
		workerLogger.Warn("postgres url not provided, using in-memory worker repository")
	}

	var cache worker.Cache
	if redisClient != nil {
		redisCache, err := worker.NewRedisCache(worker.RedisCacheConfig{Client: redisClient})
		if err != nil {
			workerLogger.Warn("failed to initialize worker cache", "error", err)
		} else {
			cache = redisCache
		}
	}

	metrics := worker.NewMetrics(prometheus.DefaultRegisterer)
	workerService, err := worker.NewService(worker.Config{
		Repository: repo,
		Cache:      cache,
		Metrics:    metrics,
		Logger:     workerLogger,
	})
	if err != nil {
		return fmt.Errorf("initialize worker service: %w", err)
	}

	httpLogger := logger.With("component", "http")

	var authorizer httpapi.Authorizer
	var bootstrapToken *auth.APIKeyMaterial
	if cfg.DisableAuth {
		httpLogger.Warn("authentication disabled; REST and gRPC endpoints accept all requests")
	} else {
		authorizer, bootstrapToken, err = buildAuthorizer(baseCtx, cfg)
		if err != nil {
			return err
		}
		if bootstrapToken != nil {
			httpLogger.Info("bootstrap api key created", "label", bootstrapToken.Key.Label, "role", bootstrapToken.Key.Role)
			if cfg.BootstrapAdminFile != "" {
				if err := writeBootstrapToken(cfg.BootstrapAdminFile, bootstrapToken.Token); err != nil {
					httpLogger.Warn("failed to write bootstrap api key", "path", cfg.BootstrapAdminFile, "error", err)
				} else {
					httpLogger.Info("bootstrap api key written", "path", cfg.BootstrapAdminFile)
				}
			} else {
				httpLogger.Info("bootstrap api key", "token", bootstrapToken.Token)
			}
		}
	}

	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: httpapi.NewServer(libraryService, workerService, authorizer).Routes(),
	}

	grpcServer := grpc.NewServer()
	grpcHandler := grpcapi.NewServer(libraryService, workerService)
	librarypb.RegisterLibraryServiceServer(grpcServer, grpcHandler)
	workerpb.RegisterWorkerServiceServer(grpcServer, grpcHandler)

	errCh := make(chan error, 2)

	go func() {
		httpLogger.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	go func() {
		grpcLogger := logger.With("component", "grpc")
		listener, err := net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			errCh <- fmt.Errorf("grpc listen: %w", err)
			return
		}
		grpcLogger.Info("grpc server listening", "addr", cfg.GRPCAddr)
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- fmt.Errorf("grpc server: %w", err)
		}
	}()

	select {
	case <-baseCtx.Done():
	case err := <-errCh:
		return err
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	httpLogger.Info("shutting down http server")
	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		httpLogger.Warn("http shutdown error", "error", err)
	}

	logger.Info("stopping grpc server")
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		logger.Warn("grpc graceful stop timed out, forcing close")
		grpcServer.Stop()
	}

	return nil
}

func loadConfig() masterConfig {
	httpAddr := strings.TrimSpace(getEnv("ARRGO_HTTP_ADDR", ":8080"))
	grpcAddr := strings.TrimSpace(getEnv("ARRGO_GRPC_ADDR", ":9090"))
	logLevel := parseLogLevel(getEnv("ARRGO_LOG_LEVEL", "info"))
	disableAuth := parseBool(os.Getenv("ARRGO_DISABLE_AUTH"))
	shutdownTimeout := parseDuration(getEnv("ARRGO_SHUTDOWN_TIMEOUT", "10s"), 10*time.Second)

	redisDB := 0
	if raw := strings.TrimSpace(os.Getenv("ARRGO_REDIS_DB")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			redisDB = value
		}
	}

	return masterConfig{
		HTTPAddr:            httpAddr,
		GRPCAddr:            grpcAddr,
		DatabaseURL:         strings.TrimSpace(os.Getenv("ARRGO_DATABASE_URL")),
		RedisAddr:           strings.TrimSpace(os.Getenv("ARRGO_REDIS_ADDR")),
		RedisUsername:       strings.TrimSpace(os.Getenv("ARRGO_REDIS_USERNAME")),
		RedisPassword:       strings.TrimSpace(os.Getenv("ARRGO_REDIS_PASSWORD")),
		RedisDB:             redisDB,
		LogLevel:            logLevel,
		DisableAuth:         disableAuth,
		ShutdownTimeout:     shutdownTimeout,
		BootstrapAdminToken: strings.TrimSpace(os.Getenv("ARRGO_BOOTSTRAP_ADMIN_TOKEN")),
		BootstrapAdminLabel: getEnv("ARRGO_BOOTSTRAP_ADMIN_LABEL", "arrgo-dashboard"),
		BootstrapAdminFile:  strings.TrimSpace(os.Getenv("ARRGO_BOOTSTRAP_ADMIN_TOKEN_FILE")),
	}
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func parseLogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func parseBool(value string) bool {
	result, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return result
}

func parseDuration(raw string, fallback time.Duration) time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	d, err := time.ParseDuration(trimmed)
	if err != nil {
		return fallback
	}
	return d
}

func newLogger(level slog.Level) *slog.Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}

func connectRedis(ctx context.Context, cfg masterConfig, logger *slog.Logger) (*redis.Client, error) {
	if cfg.RedisAddr == "" {
		logger.Warn("redis address not provided; orchestration queue and cache disabled")
		return nil, nil
	}
	options := &redis.Options{
		Addr:     cfg.RedisAddr,
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}
	client := redis.NewClient(options)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	logger.Info("connected to redis", "addr", cfg.RedisAddr, "db", cfg.RedisDB)
	return client, nil
}

func connectPostgres(ctx context.Context, cfg masterConfig, logger *slog.Logger) (*pgxpool.Pool, error) {
	if cfg.DatabaseURL == "" {
		logger.Warn("database url not provided; library and worker metadata will not persist across restarts")
		return nil, nil
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	logger.Info("connected to postgres")
	return pool, nil
}

func buildAuthorizer(ctx context.Context, cfg masterConfig) (httpapi.Authorizer, *auth.APIKeyMaterial, error) {
	idGen := &staticIDGenerator{fallback: func() string { return uuid.NewString() }}
	secretGen := &staticSecretGenerator{}

	if cfg.BootstrapAdminToken != "" {
		id, secret, ok := splitToken(cfg.BootstrapAdminToken)
		if !ok {
			return nil, nil, fmt.Errorf("invalid ARRGO_BOOTSTRAP_ADMIN_TOKEN format")
		}
		idGen.value = id
		secretGen.value = secret
	}

	authService, err := auth.NewService(auth.Config{
		IDGenerator:     idGen.Generate,
		SecretGenerator: secretGen.Generate,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("initialize auth service: %w", err)
	}

	if cfg.BootstrapAdminToken == "" {
		return authService, nil, nil
	}

	label := strings.TrimSpace(cfg.BootstrapAdminLabel)
	if label == "" {
		label = "arrgo-dashboard"
	}
	material, err := authService.CreateAPIKey(ctx, label, "admin")
	if err != nil {
		return nil, nil, fmt.Errorf("bootstrap api key: %w", err)
	}
	return authService, &material, nil
}

func writeBootstrapToken(path, token string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token+"\n"), 0o600)
}

func splitToken(token string) (string, string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", false
	}
	idx := strings.Index(token, ".")
	if idx <= 0 || idx == len(token)-1 {
		return "", "", false
	}
	return token[:idx], token[idx+1:], true
}

type staticIDGenerator struct {
	value    string
	fallback func() string
	used     bool
}

func (g *staticIDGenerator) Generate() string {
	if !g.used && g.value != "" {
		g.used = true
		return g.value
	}
	if g.fallback != nil {
		return g.fallback()
	}
	return uuid.NewString()
}

type staticSecretGenerator struct {
	value string
	used  bool
}

func (g *staticSecretGenerator) Generate() (string, error) {
	if !g.used && g.value != "" {
		g.used = true
		return g.value, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
