package worker

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"
)

type stubClock struct{ now time.Time }

func (s *stubClock) Now() time.Time { return s.now }

func TestRegisterWorkerPersistsAndCaches(t *testing.T) {
	repo := NewInMemoryRepository()
	cache := newRedisCache(t)
	metrics := NewMetrics(prometheus.NewRegistry())
	clock := &stubClock{now: time.Unix(1_700_000_000, 0)}

	svc, err := NewService(Config{
		Repository: repo,
		Cache:      cache,
		Metrics:    metrics,
		Clock:      clock,
		IDGenerator: func() string {
			return "worker-123"
		},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	result, err := svc.RegisterWorker(context.Background(), RegistrationRequest{
		Pool:         "default",
		Capabilities: []string{"transcode", " scan "},
		Endpoint:     "http://worker-1:8080",
		Version:      "1.0.0",
	})
	if err != nil {
		t.Fatalf("RegisterWorker returned error: %v", err)
	}
	if result.Worker.ID != "worker-123" {
		t.Fatalf("unexpected worker id %s", result.Worker.ID)
	}
	if result.Worker.Status != StatusHealthy {
		t.Fatalf("expected healthy status, got %s", result.Worker.Status)
	}
	if len(result.Worker.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(result.Worker.Capabilities))
	}
	if result.Assignment.Stream == "" || result.Assignment.ConsumerGroup == "" {
		t.Fatalf("expected assignment metadata to be populated")
	}

	stored, err := repo.GetWorker(context.Background(), "worker-123")
	if err != nil {
		t.Fatalf("GetWorker returned error: %v", err)
	}
	if stored.Endpoint != "http://worker-1:8080" {
		t.Fatalf("unexpected stored endpoint %s", stored.Endpoint)
	}

	cached, ok, err := cache.GetWorker(context.Background(), "worker-123")
	if err != nil {
		t.Fatalf("cache lookup returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected worker to be cached")
	}
	if cached.ID != "worker-123" {
		t.Fatalf("unexpected cached worker id %s", cached.ID)
	}

	active := testutil.ToFloat64(metrics.ActiveWorkers().WithLabelValues("default"))
	if active != 1 {
		t.Fatalf("expected active worker gauge to be 1, got %f", active)
	}
}

func TestHeartbeatAndStaleDetection(t *testing.T) {
	repo := NewInMemoryRepository()
	cache := newRedisCache(t)
	metrics := NewMetrics(prometheus.NewRegistry())
	clock := &stubClock{now: time.Unix(1_700_000_000, 0)}

	svc, err := NewService(Config{
		Repository:        repo,
		Cache:             cache,
		Metrics:           metrics,
		Clock:             clock,
		HeartbeatInterval: 5 * time.Second,
		StaleAfter:        15 * time.Second,
		IDGenerator: func() string {
			return "worker-abc"
		},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	if _, err := svc.RegisterWorker(context.Background(), RegistrationRequest{Pool: "imports"}); err != nil {
		t.Fatalf("RegisterWorker returned error: %v", err)
	}

	clock.now = clock.now.Add(5 * time.Second)
	hb, err := svc.RecordHeartbeat(context.Background(), HeartbeatInput{WorkerID: "worker-abc", ActiveJobs: 3})
	if err != nil {
		t.Fatalf("RecordHeartbeat returned error: %v", err)
	}
	if hb.Worker.ActiveJobs != 3 {
		t.Fatalf("expected active jobs to be 3, got %d", hb.Worker.ActiveJobs)
	}
	if hb.Stale {
		t.Fatalf("expected heartbeat to report healthy worker")
	}

	age := testutil.ToFloat64(metrics.HeartbeatAge().WithLabelValues("imports", "worker-abc"))
	if age < 0 {
		t.Fatalf("expected heartbeat age to be non-negative, got %f", age)
	}

	clock.now = clock.now.Add(20 * time.Second)
	svc.Reconcile(context.Background())

	staleGauge := testutil.ToFloat64(metrics.StaleWorkers().WithLabelValues("imports"))
	if staleGauge != 1 {
		t.Fatalf("expected stale worker gauge to be 1, got %f", staleGauge)
	}

	age = testutil.ToFloat64(metrics.HeartbeatAge().WithLabelValues("imports", "worker-abc"))
	if age <= 0 {
		t.Fatalf("expected heartbeat age to advance after reconcile, got %f", age)
	}

	stored, err := repo.GetWorker(context.Background(), "worker-abc")
	if err != nil {
		t.Fatalf("GetWorker returned error: %v", err)
	}
	if stored.Status != StatusStale {
		t.Fatalf("expected worker status stale, got %s", stored.Status)
	}
}

func TestListWorkersFallsBackToRepositoryWhenCacheEmpty(t *testing.T) {
	repo := NewInMemoryRepository()
	cache := newRedisCache(t)
	metrics := NewMetrics(prometheus.NewRegistry())
	clock := &stubClock{now: time.Unix(1_700_000_000, 0)}

	svc, err := NewService(Config{
		Repository:  repo,
		Cache:       cache,
		Metrics:     metrics,
		Clock:       clock,
		IDGenerator: func() string { return "worker-list" },
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	if _, err := svc.RegisterWorker(context.Background(), RegistrationRequest{Pool: "default"}); err != nil {
		t.Fatalf("RegisterWorker returned error: %v", err)
	}

	if err := cache.RemoveWorker(context.Background(), "worker-list"); err != nil {
		t.Fatalf("RemoveWorker returned error: %v", err)
	}

	workers, err := svc.ListWorkers(context.Background())
	if err != nil {
		t.Fatalf("ListWorkers returned error: %v", err)
	}
	if len(workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(workers))
	}
	if workers[0].ID != "worker-list" {
		t.Fatalf("unexpected worker id %s", workers[0].ID)
	}

	cached, err := cache.ListWorkers(context.Background())
	if err != nil {
		t.Fatalf("cache ListWorkers returned error: %v", err)
	}
	if len(cached) != 1 {
		t.Fatalf("expected cache to be repopulated, got %d entries", len(cached))
	}
}

func TestPoolConfigurationOverrides(t *testing.T) {
	repo := NewInMemoryRepository()
	metrics := NewMetrics(prometheus.NewRegistry())
	clock := &stubClock{now: time.Unix(1_700_000_000, 0)}

	svc, err := NewService(Config{
		Repository:        repo,
		Metrics:           metrics,
		Clock:             clock,
		HeartbeatInterval: 10 * time.Second,
		StaleAfter:        30 * time.Second,
		IDGenerator: func() string {
			return "worker-config"
		},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	configs, err := svc.ListPoolConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListPoolConfigs returned error: %v", err)
	}
	if len(configs) == 0 {
		t.Fatalf("expected default pool configuration to be present")
	}

	override, err := svc.UpsertPoolConfig(context.Background(), PoolConfig{
		Pool:              "transcode",
		Description:       "GPU accelerated jobs",
		Stream:            "arrgo:library:transcode",
		ConsumerGroup:     "arrgo-transcode",
		HeartbeatInterval: 20 * time.Second,
		StaleAfter:        50 * time.Second,
		MaxConcurrentJobs: 4,
	})
	if err != nil {
		t.Fatalf("UpsertPoolConfig returned error: %v", err)
	}
	if override.HeartbeatInterval != 20*time.Second {
		t.Fatalf("expected heartbeat interval override to persist")
	}
	if override.Stream != "arrgo:library:transcode" {
		t.Fatalf("unexpected stream override %s", override.Stream)
	}

	result, err := svc.RegisterWorker(context.Background(), RegistrationRequest{Pool: "transcode"})
	if err != nil {
		t.Fatalf("RegisterWorker returned error: %v", err)
	}
	if result.Assignment.ConsumerGroup != "arrgo-transcode" {
		t.Fatalf("expected assignment to use pool consumer group override")
	}
	if result.Assignment.HeartbeatInterval != 20*time.Second {
		t.Fatalf("expected assignment to use pool heartbeat override")
	}

	clock.now = clock.now.Add(35 * time.Second)
	svc.Reconcile(context.Background())

	stored, err := repo.GetWorker(context.Background(), "worker-config")
	if err != nil {
		t.Fatalf("GetWorker returned error: %v", err)
	}
	if stored.Status != StatusHealthy {
		t.Fatalf("expected worker to remain healthy before stale threshold, got %s", stored.Status)
	}

	clock.now = clock.now.Add(20 * time.Second)
	svc.Reconcile(context.Background())

	stored, err = repo.GetWorker(context.Background(), "worker-config")
	if err != nil {
		t.Fatalf("GetWorker returned error: %v", err)
	}
	if stored.Status != StatusStale {
		t.Fatalf("expected worker to be marked stale after pool threshold, got %s", stored.Status)
	}

	configs, err = svc.ListPoolConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListPoolConfigs returned error: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("expected two pool configs after override, got %d", len(configs))
	}
	if configs[0].Pool != "default" || configs[1].Pool != "transcode" {
		t.Fatalf("expected configs to be sorted by pool name, got %#v", configs)
	}
}

func newRedisCache(t *testing.T) *RedisCache {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cache, err := NewRedisCache(RedisCacheConfig{Client: client})
	if err != nil {
		t.Fatalf("NewRedisCache returned error: %v", err)
	}
	return cache
}
