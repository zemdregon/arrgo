package library

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubClock struct{ now time.Time }

func (s stubClock) Now() time.Time { return s.now }

func TestListEntitiesFiltersByType(t *testing.T) {
	svc := NewService(Config{
		Entities: []Entity{{ID: "1", EntityType: "series", Title: "Series"}, {ID: "2", EntityType: "movie", Title: "Movie"}},
	})

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
	svc := NewService(Config{
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

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
}

func TestTriggerJobRejectsUnknownEntities(t *testing.T) {
	svc := NewService(Config{Entities: []Entity{{ID: "entity-1", EntityType: "movie"}}})

	if _, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "refresh", TargetID: "missing"}); !errors.Is(err, ErrUnknownEntity) {
		t.Fatalf("expected ErrUnknownEntity, got %v", err)
	}
}

func TestTriggerJobValidatesInput(t *testing.T) {
	svc := NewService(Config{Entities: []Entity{{ID: "entity-1", EntityType: "movie"}}})

	if _, err := svc.TriggerJob(context.Background(), TriggerJobInput{TargetID: "entity-1"}); !errors.Is(err, ErrInvalidJobType) {
		t.Fatalf("expected ErrInvalidJobType, got %v", err)
	}
	if _, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "refresh"}); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("expected ErrInvalidTarget, got %v", err)
	}
}
