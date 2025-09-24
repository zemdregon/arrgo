package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/arrgo/arrgo/internal/api/workerpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type workerConfig struct {
	MasterAddr        string
	Pool              string
	Capabilities      []string
	Endpoint          string
	Version           string
	LogLevel          slog.Level
	DialTimeout       time.Duration
	HeartbeatOverride time.Duration
	ActiveJobs        int
}

func main() {
	cfg := loadWorkerConfig()
	logger := newLogger(cfg.LogLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := runWorker(ctx, cfg, logger); err != nil {
		logger.Error("arrgo worker exited with error", "error", err)
		os.Exit(1)
	}
	logger.Info("arrgo worker shutdown complete")
}

func runWorker(ctx context.Context, cfg workerConfig, logger *slog.Logger) error {
	logger.Info("connecting to master", "addr", cfg.MasterAddr)
	conn, err := dialMaster(ctx, cfg.MasterAddr, cfg.DialTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := workerpb.NewWorkerServiceClient(conn)
	registerCtx, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
	defer cancel()

	regResp, err := client.RegisterWorker(registerCtx, &workerpb.WorkerRegistrationRequest{
		Pool:         cfg.Pool,
		Capabilities: cfg.Capabilities,
		Endpoint:     cfg.Endpoint,
		Version:      cfg.Version,
	})
	if err != nil {
		return fmt.Errorf("register worker: %w", err)
	}

	worker := regResp.GetWorker()
	assignment := regResp.GetAssignment()
	if worker == nil || assignment == nil {
		return fmt.Errorf("master returned incomplete registration payload")
	}

	heartbeatInterval := time.Duration(assignment.GetHeartbeatIntervalSeconds()) * time.Second
	if cfg.HeartbeatOverride > 0 {
		heartbeatInterval = cfg.HeartbeatOverride
	}
	if heartbeatInterval <= 0 {
		heartbeatInterval = 15 * time.Second
	}

	logger.Info("worker registered",
		"worker_id", worker.GetId(),
		"pool", worker.GetPool(),
		"stream", assignment.GetStream(),
		"consumer_group", assignment.GetConsumerGroup(),
		"heartbeat_interval", heartbeatInterval.String(),
	)

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			hbCtx, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
			resp, err := client.Heartbeat(hbCtx, &workerpb.HeartbeatRequest{
				WorkerId:   worker.GetId(),
				ActiveJobs: int32(cfg.ActiveJobs),
			})
			cancel()
			if err != nil {
				logger.Error("heartbeat failed", "error", err)
				continue
			}
			if resp.GetStale() {
				logger.Warn("master marked worker as stale")
			} else {
				logger.Debug("heartbeat acknowledged", "worker_id", worker.GetId())
			}
		}
	}
}

func loadWorkerConfig() workerConfig {
	httpAddr := strings.TrimSpace(os.Getenv("ARRGO_MASTER_GRPC_ADDR"))
	if httpAddr == "" {
		httpAddr = "localhost:9090"
	}

	pool := strings.TrimSpace(getEnv("ARRGO_WORKER_POOL", "default"))
	caps := parseCapabilities(os.Getenv("ARRGO_WORKER_CAPABILITIES"))
	endpoint := strings.TrimSpace(os.Getenv("ARRGO_WORKER_ENDPOINT"))
	if endpoint == "" {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "arrgo-worker"
		}
		endpoint = fmt.Sprintf("http://%s:9000", hostname)
	}
	version := strings.TrimSpace(getEnv("ARRGO_WORKER_VERSION", "dev"))
	logLevel := parseLogLevel(getEnv("ARRGO_LOG_LEVEL", "info"))
	dialTimeout := parseDuration(getEnv("ARRGO_DIAL_TIMEOUT", "10s"), 10*time.Second)
	heartbeatOverride := parseDuration(strings.TrimSpace(os.Getenv("ARRGO_WORKER_HEARTBEAT_INTERVAL")), 0)
	activeJobs := parseInt(getEnv("ARRGO_WORKER_ACTIVE_JOBS", "0"))

	return workerConfig{
		MasterAddr:        httpAddr,
		Pool:              pool,
		Capabilities:      caps,
		Endpoint:          endpoint,
		Version:           version,
		LogLevel:          logLevel,
		DialTimeout:       dialTimeout,
		HeartbeatOverride: heartbeatOverride,
		ActiveJobs:        activeJobs,
	}
}

func newLogger(level slog.Level) *slog.Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}

func parseCapabilities(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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

func parseInt(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0
	}
	return value
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

func dialMaster(ctx context.Context, addr string, timeout time.Duration) (*grpc.ClientConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := grpc.DialContext(dialCtx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial master %q: %w", addr, err)
	}
	return conn, nil
}
