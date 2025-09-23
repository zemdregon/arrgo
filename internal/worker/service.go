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

	metricsMu    sync.Mutex
	knownPools   map[string]struct{}
	knownWorkers map[string]struct{}
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
	}

	return svc, nil
}

// RegisterWorker validates and persists worker metadata then returns queue assignment info.
func (s *Service) RegisterWorker(ctx context.Context, req RegistrationRequest) (RegistrationResult, error) {
	pool := strings.TrimSpace(req.Pool)
	if pool == "" {
		return RegistrationResult{}, ErrInvalidPool
	}
	capabilities := sanitizeCapabilities(req.Capabilities)
	endpoint := strings.TrimSpace(req.Endpoint)
	version := strings.TrimSpace(req.Version)
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

	persisted, err := s.saveWorker(ctx, worker, false)
	if err != nil {
		return RegistrationResult{}, err
	}

	assignment := s.assignment
	assignment.HeartbeatInterval = s.heartbeatInterval

	return RegistrationResult{Worker: persisted, Assignment: assignment}, nil
}

// RecordHeartbeat updates worker availability metadata and recalculates metrics.
func (s *Service) RecordHeartbeat(ctx context.Context, input HeartbeatInput) (HeartbeatResult, error) {
	id := strings.TrimSpace(input.WorkerID)
	if id == "" {
		return HeartbeatResult{}, ErrInvalidWorkerID
	}

	worker, err := s.loadWorker(ctx, id)
	if err != nil {
		return HeartbeatResult{}, err
	}

	now := s.clock.Now().UTC()
	worker.LastHeartbeatAt = now
	if input.ActiveJobs < 0 {
		worker.ActiveJobs = 0
	} else {
		worker.ActiveJobs = input.ActiveJobs
	}
	worker.Status = StatusHealthy

	persisted, err := s.saveWorker(ctx, worker, false)
	if err != nil {
		return HeartbeatResult{}, err
	}

	stale := s.clock.Now().Sub(persisted.LastHeartbeatAt) > s.staleAfter

	return HeartbeatResult{Worker: persisted, Stale: stale}, nil
}

// Reconcile refreshes caches and metrics, marking stale workers when necessary.
func (s *Service) Reconcile(ctx context.Context) {
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
		key := worker.Pool + "\x00" + worker.ID
		currentWorkers[key] = struct{}{}

		age := now.Sub(worker.LastHeartbeatAt)
		if age < 0 {
			age = 0
		}
		s.metrics.heartbeatAge.WithLabelValues(worker.Pool, worker.ID).Set(age.Seconds())

		if age > s.staleAfter {
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
