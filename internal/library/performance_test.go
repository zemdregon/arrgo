package library

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSchedulerLargeLibraryFanout(t *testing.T) {
	t.Parallel()

	const (
		entityCount    = 10000
		concurrencyCap = 512
		maxDuration    = 5 * time.Second
	)

	entities := generateTestEntities(entityCount, "series")
	svc := newRedisService(t, Config{Entities: entities})
	scheduler, err := NewScheduler(SchedulerConfig{
		Service: svc,
		Schedules: []Schedule{{
			EntityType:    "series",
			JobType:       JobTypeScan,
			Cron:          "@every 5m",
			MaxConcurrent: concurrencyCap,
		}},
	})
	if err != nil {
		t.Fatalf("NewScheduler returned error: %v", err)
	}
	schedule := scheduler.schedules[0]

	start := time.Now()
	if err := scheduler.runSchedule(context.Background(), schedule); err != nil {
		t.Fatalf("runSchedule returned error: %v", err)
	}
	duration := time.Since(start)
	if duration > maxDuration {
		t.Fatalf("expected scheduler to complete under %v, took %v", maxDuration, duration)
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != concurrencyCap {
		t.Fatalf("expected %d queued jobs, got %d", concurrencyCap, len(jobs))
	}

	seen := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		if job.State != JobStateQueued {
			t.Fatalf("expected job state %q, got %q", JobStateQueued, job.State)
		}
		if _, ok := seen[job.TargetID]; ok {
			t.Fatalf("duplicate job queued for target %s", job.TargetID)
		}
		seen[job.TargetID] = struct{}{}
	}
	t.Logf("queued %d jobs for %d entities in %v", len(jobs), entityCount, duration)
}

func TestQueueHighVolumeDrain(t *testing.T) {
	t.Parallel()

	const (
		entityCount  = 6000
		batchSize    = 256
		drainTimeout = 10 * time.Second
	)

	entities := generateTestEntities(entityCount, "series")
	svc := newRedisService(t, Config{Entities: entities})
	ctx := context.Background()

	for _, entity := range entities {
		if _, err := svc.TriggerJob(ctx, TriggerJobInput{JobType: JobTypeScan, TargetID: entity.ID}); err != nil {
			t.Fatalf("TriggerJob returned error: %v", err)
		}
	}

	start := time.Now()
	claimedTotal := 0
	iterations := 0
	streamIDs := make(map[string]struct{}, entityCount)
	for claimedTotal < entityCount {
		iterations++
		claimed, err := svc.ClaimJobs(ctx, fmt.Sprintf("perf-worker-%d", iterations), batchSize, 0)
		if err != nil {
			t.Fatalf("ClaimJobs returned error: %v", err)
		}
		if len(claimed) == 0 {
			t.Fatalf("expected claimed jobs on iteration %d", iterations)
		}
		for _, job := range claimed {
			if _, ok := streamIDs[job.StreamID]; ok {
				t.Fatalf("duplicate stream id observed: %s", job.StreamID)
			}
			streamIDs[job.StreamID] = struct{}{}
			if err := svc.AckJob(ctx, job); err != nil {
				t.Fatalf("AckJob returned error: %v", err)
			}
		}
		claimedTotal += len(claimed)
		if iterations > (entityCount/batchSize)+10 {
			t.Fatalf("drain required too many iterations: %d", iterations)
		}
		if time.Since(start) > drainTimeout {
			t.Fatalf("queue drain exceeded timeout of %v", drainTimeout)
		}
	}

	jobs, err := svc.ListJobs(ctx)
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != entityCount {
		t.Fatalf("expected %d jobs recorded, got %d", entityCount, len(jobs))
	}
	for _, job := range jobs {
		if job.State != JobStateDone {
			t.Fatalf("expected job state %q after drain, got %q", JobStateDone, job.State)
		}
	}
	duration := time.Since(start)
	t.Logf("drained %d jobs across %d iterations in %v", claimedTotal, iterations, duration)
}

func BenchmarkSchedulerRunLargeLibrary(b *testing.B) {
	const (
		entityCount    = 4000
		concurrencyCap = 256
	)

	entities := generateTestEntities(entityCount, "series")
	svc := newRedisService(b, Config{Entities: entities})
	scheduler, err := NewScheduler(SchedulerConfig{
		Service: svc,
		Schedules: []Schedule{{
			EntityType:    "series",
			JobType:       JobTypeScan,
			Cron:          "@every 10m",
			MaxConcurrent: concurrencyCap,
		}},
	})
	if err != nil {
		b.Fatalf("NewScheduler returned error: %v", err)
	}
	schedule := scheduler.schedules[0]
	ctx := context.Background()

	resetLibraryStateForBenchmark(b, svc)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resetLibraryStateForBenchmark(b, svc)
		b.StartTimer()
		if err := scheduler.runSchedule(ctx, schedule); err != nil {
			b.Fatalf("runSchedule returned error: %v", err)
		}
		b.StopTimer()
	}
}

func generateTestEntities(count int, entityType string) []Entity {
	entities := make([]Entity, count)
	for i := 0; i < count; i++ {
		entities[i] = Entity{
			ID:         fmt.Sprintf("%s-%d", entityType, i+1),
			EntityType: entityType,
			Title:      fmt.Sprintf("%s %d", entityType, i+1),
		}
	}
	return entities
}

func resetLibraryStateForBenchmark(tb testing.TB, svc *Service) {
	tb.Helper()

	if svc.queue != nil {
		if err := svc.queue.client.FlushAll(context.Background()).Err(); err != nil {
			tb.Fatalf("FlushAll returned error: %v", err)
		}
	}

	svc.mu.Lock()
	svc.jobs = svc.jobs[:0]
	svc.jobIndex = make(map[string]int)
	svc.mu.Unlock()
}
