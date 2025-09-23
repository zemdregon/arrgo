package library

import (
	"context"
	"testing"
)

func TestNewSchedulerValidatesConfig(t *testing.T) {
	if _, err := NewScheduler(SchedulerConfig{}); err == nil {
		t.Fatalf("expected error when service is nil")
	}

	svc := newRedisService(t, Config{})
	_, err := NewScheduler(SchedulerConfig{
		Service:   svc,
		Schedules: []Schedule{{EntityType: "", JobType: JobTypeScan, Cron: "@every 1m"}},
	})
	if err == nil {
		t.Fatalf("expected error for invalid schedule")
	}
}

func TestSchedulerRunRespectsConcurrency(t *testing.T) {
	svc := newRedisService(t, Config{
		Entities: []Entity{
			{ID: "series-1", EntityType: "series", Title: "Series A"},
			{ID: "series-2", EntityType: "series", Title: "Series B"},
			{ID: "series-3", EntityType: "series", Title: "Series C"},
		},
	})

	scheduler, err := NewScheduler(SchedulerConfig{
		Service: svc,
		Schedules: []Schedule{{
			EntityType:    "series",
			JobType:       JobTypeScan,
			Cron:          "@every 1m",
			MaxConcurrent: 2,
		}},
	})
	if err != nil {
		t.Fatalf("NewScheduler returned error: %v", err)
	}
	schedule := scheduler.schedules[0]

	if err := scheduler.runSchedule(context.Background(), schedule); err != nil {
		t.Fatalf("runSchedule returned error: %v", err)
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs queued, got %d", len(jobs))
	}

	claimed, err := svc.ClaimJobs(context.Background(), "worker-1", 2, 0)
	if err != nil {
		t.Fatalf("ClaimJobs returned error: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("expected 2 claimed jobs, got %d", len(claimed))
	}

	if err := scheduler.runSchedule(context.Background(), schedule); err != nil {
		t.Fatalf("runSchedule with active jobs returned error: %v", err)
	}
	jobs, err = svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected scheduler to honor concurrency limit, got %d jobs", len(jobs))
	}

	if err := svc.AckJob(context.Background(), claimed[0]); err != nil {
		t.Fatalf("AckJob returned error: %v", err)
	}

	if err := scheduler.runSchedule(context.Background(), schedule); err != nil {
		t.Fatalf("runSchedule after completion returned error: %v", err)
	}

	jobs, err = svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected scheduler to queue another job, got %d", len(jobs))
	}

	active := 0
	for _, job := range jobs {
		if job.State != JobStateDone {
			active++
		}
	}
	if active > 2 {
		t.Fatalf("expected at most 2 active jobs, got %d", active)
	}
}
