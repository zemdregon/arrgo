package library

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type stubClock struct{ now time.Time }

func (s stubClock) Now() time.Time { return s.now }

func newRedisService(t *testing.T, cfg Config) *Service {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cfg.RedisClient = client

	svc, err := NewService(cfg)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
	})

	return svc
}

func TestListEntitiesFiltersByType(t *testing.T) {
	svc, err := NewService(Config{
		Entities: []Entity{{ID: "1", EntityType: "series", Title: "Series"}, {ID: "2", EntityType: "movie", Title: "Movie"}},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	entities, err := svc.ListEntities(context.Background(), ListEntitiesFilter{EntityType: "MOVIE"})
	if err != nil {
		t.Fatalf("ListEntities returned error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}
	if entities[0].EntityType != "movie" {
		t.Fatalf("expected movie type, got %s", entities[0].EntityType)
	}
}

func TestTriggerJobValidatesAndPersists(t *testing.T) {
	now := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	svc := newRedisService(t, Config{
		Entities: []Entity{{ID: "entity-1", EntityType: "series"}},
		Clock:    stubClock{now: now},
		IDGenerator: func() string {
			return "job-123"
		},
	})

	job, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "scan", TargetID: "entity-1"})
	if err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}
	if job.ID != "job-123" {
		t.Fatalf("unexpected job id %s", job.ID)
	}
	if job.State != JobStateQueued {
		t.Fatalf("unexpected job state %s", job.State)
	}
	if !job.SubmittedAt.Equal(now) {
		t.Fatalf("unexpected submission time %v", job.SubmittedAt)
	}
	if job.Attempt != 0 {
		t.Fatalf("expected initial attempt count 0, got %d", job.Attempt)
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
}

func TestTriggerJobRejectsUnknownEntities(t *testing.T) {
	svc := newRedisService(t, Config{Entities: []Entity{{ID: "entity-1", EntityType: "movie"}}})

	if _, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "refresh", TargetID: "missing"}); !errors.Is(err, ErrUnknownEntity) {
		t.Fatalf("expected ErrUnknownEntity, got %v", err)
	}
}

func TestTriggerJobValidatesInput(t *testing.T) {
	svc := newRedisService(t, Config{Entities: []Entity{{ID: "entity-1", EntityType: "movie"}}})

	if _, err := svc.TriggerJob(context.Background(), TriggerJobInput{TargetID: "entity-1"}); !errors.Is(err, ErrInvalidJobType) {
		t.Fatalf("expected ErrInvalidJobType, got %v", err)
	}
	if _, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "refresh"}); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("expected ErrInvalidTarget, got %v", err)
	}
}

func TestTriggerJobEnforcesIdempotency(t *testing.T) {
	svc := newRedisService(t, Config{Entities: []Entity{{ID: "entity-1", EntityType: "series"}}})

	input := TriggerJobInput{JobType: "refresh", TargetID: "entity-1", IdempotencyKey: "refresh-entity-1"}
	if _, err := svc.TriggerJob(context.Background(), input); err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}
	if _, err := svc.TriggerJob(context.Background(), input); !errors.Is(err, ErrDuplicateJob) {
		t.Fatalf("expected ErrDuplicateJob, got %v", err)
	}
}

func TestClaimAckFlowUpdatesState(t *testing.T) {
	svc := newRedisService(t, Config{Entities: []Entity{{ID: "entity-1", EntityType: "series"}}})

	job, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "scan", TargetID: "entity-1"})
	if err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}

	claimed, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs returned error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}
	if claimed[0].Job.ID != job.ID {
		t.Fatalf("unexpected job id %s", claimed[0].Job.ID)
	}
	if claimed[0].Job.State != JobStateRunning {
		t.Fatalf("expected running state, got %s", claimed[0].Job.State)
	}
	if claimed[0].Job.Attempt != 1 {
		t.Fatalf("expected attempt 1, got %d", claimed[0].Job.Attempt)
	}
	if claimed[0].Job.LastAttemptAt.IsZero() {
		t.Fatalf("expected last attempt timestamp to be set")
	}

	if err := svc.AckJob(context.Background(), claimed[0]); err != nil {
		t.Fatalf("AckJob returned error: %v", err)
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if jobs[0].State != JobStateDone {
		t.Fatalf("expected job to be completed, got %s", jobs[0].State)
	}
}

func TestRetryJobRequeuesWithIncrementedAttempt(t *testing.T) {
	svc := newRedisService(t, Config{Entities: []Entity{{ID: "entity-1", EntityType: "movie"}}})

	_, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "refresh", TargetID: "entity-1"})
	if err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}

	claimed, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs returned error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}

	retried, err := svc.RetryJob(context.Background(), claimed[0])
	if err != nil {
		t.Fatalf("RetryJob returned error: %v", err)
	}
	if retried.State != JobStateQueued {
		t.Fatalf("expected job to return to queued state, got %s", retried.State)
	}

	claimedAgain, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs (retry) returned error: %v", err)
	}
	if len(claimedAgain) != 1 {
		t.Fatalf("expected 1 claimed job after retry, got %d", len(claimedAgain))
	}
	if claimedAgain[0].Job.Attempt != 2 {
		t.Fatalf("expected attempt 2 after retry, got %d", claimedAgain[0].Job.Attempt)
	}
}
