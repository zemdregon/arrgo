package worker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultHeartbeatInterval = 15 * time.Second
	defaultStaleAfter        = 45 * time.Second
	defaultStreamName        = "arrgo:library:jobs"
	defaultConsumerGroup     = "arrgo-library-workers"
)

// Clock abstracts time retrieval for deterministic testing.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// IDGenerator produces stable identifiers for new workers.
type IDGenerator func() string

// Config drives Service construction.
type Config struct {
	Repository        Repository
	Cache             Cache
	Metrics           *Metrics
	Logger            *slog.Logger
	Clock             Clock
	IDGenerator       IDGenerator
	HeartbeatInterval time.Duration
	StaleAfter        time.Duration
	Stream            string
	ConsumerGroup     string
	Tracer            trace.Tracer
}

// Service coordinates worker registration, heartbeats, and associated telemetry.
type Service struct {
	repo    Repository
	cache   Cache
	metrics *Metrics
	logger  *slog.Logger
	clock   Clock
	idGen   IDGenerator

	heartbeatInterval time.Duration
	staleAfter        time.Duration
	assignment        Assignment

	tracer trace.Tracer

	metricsMu    sync.Mutex
	knownPools   map[string]struct{}
	knownWorkers map[string]struct{}

	configMu    sync.RWMutex
	poolConfigs map[string]PoolConfig
}

// NewService constructs a worker coordination service.
func NewService(cfg Config) (*Service, error) {
	if cfg.Repository == nil {
		return nil, fmt.Errorf("worker: repository is required")
	}

	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	idGen := cfg.IDGenerator
	if idGen == nil {
		idGen = func() string { return uuid.NewString() }
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	heartbeatInterval := cfg.HeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = defaultHeartbeatInterval
	}
	staleAfter := cfg.StaleAfter
	if staleAfter <= 0 {
		staleAfter = defaultStaleAfter
	}
	stream := cfg.Stream
	if stream == "" {
		stream = defaultStreamName
	}
	group := cfg.ConsumerGroup
	if group == "" {
		group = defaultConsumerGroup
	}

	svc := &Service{
		repo:              cfg.Repository,
		cache:             cfg.Cache,
		metrics:           cfg.Metrics,
		logger:            logger,
		clock:             clock,
		idGen:             idGen,
		heartbeatInterval: heartbeatInterval,
		staleAfter:        staleAfter,
		assignment: Assignment{
			Stream:        stream,
			ConsumerGroup: group,
		},
		knownPools:   make(map[string]struct{}),
		knownWorkers: make(map[string]struct{}),
		poolConfigs:  make(map[string]PoolConfig),
	}

	svc.tracer = cfg.Tracer
	if svc.tracer == nil {
		svc.tracer = otel.Tracer("github.com/arrgo/arrgo/internal/worker")
	}

	svc.poolConfigs["default"] = svc.sanitizePoolConfig(PoolConfig{Pool: "default"})
	svc.knownPools["default"] = struct{}{}

	return svc, nil
}

func (s *Service) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	options := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindInternal)}
	if len(attrs) > 0 {
		options = append(options, trace.WithAttributes(attrs...))
	}
	return s.tracer.Start(ctx, name, options...)
}

func finishSpan(span trace.Span, err *error) {
	if err != nil && *err != nil {
		span.RecordError(*err)
		span.SetStatus(codes.Error, (*err).Error())
	} else {
		span.SetStatus(codes.Ok, "ok")
	}
	span.End()
}

// RegisterWorker validates and persists worker metadata then returns queue assignment info.
func (s *Service) RegisterWorker(ctx context.Context, req RegistrationRequest) (result RegistrationResult, err error) {
	pool := strings.TrimSpace(req.Pool)
	capabilities := sanitizeCapabilities(req.Capabilities)
	endpoint := strings.TrimSpace(req.Endpoint)
	version := strings.TrimSpace(req.Version)

	ctx, span := s.startSpan(ctx, "worker.Service.RegisterWorker",
		attribute.String("worker.pool", pool),
		attribute.StringSlice("worker.capabilities", capabilities),
		attribute.String("worker.endpoint", endpoint),
		attribute.String("worker.version", version),
	)
	defer finishSpan(span, &err)

	if pool == "" {
		err = ErrInvalidPool
		return
	}

	cfg := s.ensurePoolConfig(pool)

	now := s.clock.Now().UTC()

	worker := Worker{
		ID:              s.idGen(),
		Pool:            pool,
		Capabilities:    capabilities,
		Endpoint:        endpoint,
		Version:         version,
		Status:          StatusHealthy,
		RegisteredAt:    now,
		LastHeartbeatAt: now,
	}

	var persisted Worker
	persisted, err = s.saveWorker(ctx, worker, false)
	if err != nil {
		return
	}

	assignment := Assignment{
		Stream:            cfg.Stream,
		ConsumerGroup:     cfg.ConsumerGroup,
		HeartbeatInterval: cfg.HeartbeatInterval,
	}

	span.SetAttributes(attribute.String("worker.id", persisted.ID))
	result = RegistrationResult{Worker: persisted, Assignment: assignment}
	return result, nil
}

// RecordHeartbeat updates worker availability metadata and recalculates metrics.
func (s *Service) RecordHeartbeat(ctx context.Context, input HeartbeatInput) (result HeartbeatResult, err error) {
	trimmedID := strings.TrimSpace(input.WorkerID)
	ctx, span := s.startSpan(ctx, "worker.Service.RecordHeartbeat",
		attribute.String("worker.id", trimmedID),
		attribute.Int("worker.active_jobs", input.ActiveJobs),
	)
	defer finishSpan(span, &err)

	if trimmedID == "" {
		err = ErrInvalidWorkerID
		return
	}

	var worker Worker
	worker, err = s.loadWorker(ctx, trimmedID)
	if err != nil {
		return
	}

	now := s.clock.Now().UTC()
	worker.LastHeartbeatAt = now
	if input.ActiveJobs < 0 {
		worker.ActiveJobs = 0
	} else {
		worker.ActiveJobs = input.ActiveJobs
	}
	worker.Status = StatusHealthy

	var persisted Worker
	persisted, err = s.saveWorker(ctx, worker, false)
	if err != nil {
		return
	}

	cfg := s.poolConfig(persisted.Pool)
	stale := s.clock.Now().Sub(persisted.LastHeartbeatAt) > cfg.StaleAfter
	span.SetAttributes(attribute.Bool("worker.stale", stale))

	result = HeartbeatResult{Worker: persisted, Stale: stale}
	return result, nil
}

// ListWorkers returns the currently known worker metadata, preferring cached values when available.
func (s *Service) ListWorkers(ctx context.Context) (workers []Worker, err error) {
	ctx, span := s.startSpan(ctx, "worker.Service.ListWorkers",
		attribute.Bool("worker.cache_enabled", s.cache != nil),
	)
	defer finishSpan(span, &err)

	if s.cache != nil {
		workers, err = s.cache.ListWorkers(ctx)
		if err != nil {
			s.logger.Warn("worker: cache list workers failed", "error", err)
		} else if len(workers) > 0 {
			span.SetAttributes(attribute.Bool("worker.cache_hit", true), attribute.Int("worker.count", len(workers)))
			return workers, nil
		}
	}

	workers, err = s.repo.ListWorkers(ctx)
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		for _, worker := range workers {
			if err := s.cache.SaveWorker(ctx, worker); err != nil {
				s.logger.Warn("worker: cache hydrate worker failed", "workerID", worker.ID, "error", err)
			}
		}
	}

	span.SetAttributes(attribute.Bool("worker.cache_hit", false), attribute.Int("worker.count", len(workers)))
	return workers, nil
}

// Reconcile refreshes caches and metrics, marking stale workers when necessary.
func (s *Service) Reconcile(ctx context.Context) {
	var err error
	ctx, span := s.startSpan(ctx, "worker.Service.Reconcile")
	defer finishSpan(span, &err)

	s.updateMetrics(ctx)
}

func (s *Service) loadWorker(ctx context.Context, id string) (Worker, error) {
	if s.cache != nil {
		if cached, ok, err := s.cache.GetWorker(ctx, id); err == nil && ok {
			return cached, nil
		} else if err != nil {
			s.logger.Warn("worker: cache get failed", "workerID", id, "error", err)
		}
	}

	worker, err := s.repo.GetWorker(ctx, id)
	if err != nil {
		return Worker{}, err
	}
	if s.cache != nil {
		if err := s.cache.SaveWorker(ctx, worker); err != nil {
			s.logger.Warn("worker: cache populate failed", "workerID", id, "error", err)
		}
	}
	return worker, nil
}

func (s *Service) saveWorker(ctx context.Context, worker Worker, skipMetrics bool) (Worker, error) {
	if err := s.repo.SaveWorker(ctx, worker); err != nil {
		return Worker{}, err
	}
	if s.cache != nil {
		if err := s.cache.SaveWorker(ctx, worker); err != nil {
			s.logger.Warn("worker: cache save failed", "workerID", worker.ID, "error", err)
		}
	}
	if !skipMetrics {
		s.updateMetrics(ctx)
	}
	return worker, nil
}

func (s *Service) updateMetrics(ctx context.Context) {
	if s.metrics == nil {
		return
	}

	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()

	workers, err := s.repo.ListWorkers(ctx)
	if err != nil {
		s.logger.Warn("worker: list workers failed", "error", err)
		return
	}

	now := s.clock.Now()
	activeCounts := make(map[string]int)
	staleCounts := make(map[string]int)
	currentPools := make(map[string]struct{})
	currentWorkers := make(map[string]struct{})

	for _, worker := range workers {
		currentPools[worker.Pool] = struct{}{}
		cfg := s.poolConfig(worker.Pool)
		key := worker.Pool + "\x00" + worker.ID
		currentWorkers[key] = struct{}{}

		age := now.Sub(worker.LastHeartbeatAt)
		if age < 0 {
			age = 0
		}
		s.metrics.heartbeatAge.WithLabelValues(worker.Pool, worker.ID).Set(age.Seconds())

		if age > cfg.StaleAfter {
			staleCounts[worker.Pool]++
			if worker.Status != StatusStale {
				worker.Status = StatusStale
				worker.ActiveJobs = 0
				if _, err := s.saveWorker(ctx, worker, true); err != nil {
					s.logger.Warn("worker: mark stale failed", "workerID", worker.ID, "error", err)
				}
			}
		} else {
			activeCounts[worker.Pool]++
			if worker.Status != StatusHealthy {
				worker.Status = StatusHealthy
				if _, err := s.saveWorker(ctx, worker, true); err != nil {
					s.logger.Warn("worker: mark healthy failed", "workerID", worker.ID, "error", err)
				}
			}
		}
	}

	for key := range s.knownWorkers {
		if _, ok := currentWorkers[key]; !ok {
			parts := strings.SplitN(key, "\x00", 2)
			if len(parts) == 2 {
				s.metrics.heartbeatAge.DeleteLabelValues(parts[0], parts[1])
			}
		}
	}
	s.knownWorkers = currentWorkers

	for pool := range s.knownPools {
		currentPools[pool] = struct{}{}
	}

	for pool := range currentPools {
		s.metrics.activeWorkers.WithLabelValues(pool).Set(float64(activeCounts[pool]))
		s.metrics.staleWorkers.WithLabelValues(pool).Set(float64(staleCounts[pool]))
	}
	s.knownPools = currentPools
}

// ListPoolConfigs returns sanitized configuration for known worker pools.
func (s *Service) ListPoolConfigs(ctx context.Context) ([]PoolConfig, error) {
	_ = ctx

	s.configMu.RLock()
	pools := make([]string, 0, len(s.poolConfigs))
	for pool := range s.poolConfigs {
		pools = append(pools, pool)
	}
	s.configMu.RUnlock()

	sort.Strings(pools)
	configs := make([]PoolConfig, 0, len(pools))
	for _, pool := range pools {
		configs = append(configs, s.ensurePoolConfig(pool))
	}
	return configs, nil
}

// UpsertPoolConfig stores configuration overrides for a worker pool.
func (s *Service) UpsertPoolConfig(ctx context.Context, cfg PoolConfig) (PoolConfig, error) {
	cfg.Pool = strings.TrimSpace(cfg.Pool)
	if cfg.Pool == "" {
		return PoolConfig{}, ErrInvalidPool
	}

	sanitized := s.sanitizePoolConfig(cfg)

	s.configMu.Lock()
	s.poolConfigs[sanitized.Pool] = sanitized
	s.configMu.Unlock()

	if s.metrics != nil {
		s.metricsMu.Lock()
		s.knownPools[sanitized.Pool] = struct{}{}
		s.metricsMu.Unlock()
	}

	s.updateMetrics(ctx)

	return sanitized, nil
}

func (s *Service) poolConfig(pool string) PoolConfig {
	pool = strings.TrimSpace(pool)
	if pool == "" {
		return s.ensurePoolConfig("default")
	}

	s.configMu.RLock()
	cfg, ok := s.poolConfigs[pool]
	s.configMu.RUnlock()
	if ok && cfg.Stream != "" && cfg.ConsumerGroup != "" && cfg.HeartbeatInterval > 0 && cfg.StaleAfter > 0 {
		return cfg
	}
	return s.ensurePoolConfig(pool)
}

func (s *Service) ensurePoolConfig(pool string) PoolConfig {
	pool = strings.TrimSpace(pool)
	if pool == "" {
		pool = "default"
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	if cfg, ok := s.poolConfigs[pool]; ok {
		cfg = s.sanitizePoolConfig(cfg)
		s.poolConfigs[pool] = cfg
		return cfg
	}

	cfg := s.sanitizePoolConfig(PoolConfig{Pool: pool})
	s.poolConfigs[pool] = cfg
	return cfg
}

func (s *Service) sanitizePoolConfig(cfg PoolConfig) PoolConfig {
	cfg.Pool = strings.TrimSpace(cfg.Pool)
	if cfg.Pool == "" {
		cfg.Pool = "default"
	}

	cfg.Description = strings.TrimSpace(cfg.Description)
	cfg.Stream = strings.TrimSpace(cfg.Stream)
	if cfg.Stream == "" {
		cfg.Stream = s.assignment.Stream
	}
	cfg.ConsumerGroup = strings.TrimSpace(cfg.ConsumerGroup)
	if cfg.ConsumerGroup == "" {
		cfg.ConsumerGroup = s.assignment.ConsumerGroup
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = s.heartbeatInterval
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = s.staleAfter
	}
	if cfg.MaxConcurrentJobs < 0 {
		cfg.MaxConcurrentJobs = 0
	}
	return cfg
}

func sanitizeCapabilities(values []string) []string {
	seen := make(map[string]struct{})
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, trimmed)
	}
	sort.Strings(cleaned)
	return cleaned
}
