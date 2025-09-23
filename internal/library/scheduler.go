package library

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/robfig/cron/v3"
)

// Schedule defines a cron-driven orchestration run for a specific media type.
type Schedule struct {
	EntityType    string
	JobType       string
	Cron          string
	PayloadURI    string
	MaxConcurrent int
}

// SchedulerConfig describes scheduler construction inputs.
type SchedulerConfig struct {
	Service   *Service
	Logger    *slog.Logger
	Schedules []Schedule
}

type jobSchedule struct {
	Schedule
}

// Scheduler coordinates recurring scan/import orchestration.
type Scheduler struct {
	service   *Service
	logger    *slog.Logger
	cron      *cron.Cron
	schedules []jobSchedule

	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
	baseCtx   context.Context
}

// NewScheduler wires a cron-backed scheduler for the library service.
func NewScheduler(cfg SchedulerConfig) (*Scheduler, error) {
	if cfg.Service == nil {
		return nil, errors.New("library: scheduler service is required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	cronScheduler := cron.New(cron.WithSeconds())
	scheduler := &Scheduler{
		service: cfg.Service,
		logger:  logger,
		cron:    cronScheduler,
	}

	for _, schedule := range cfg.Schedules {
		js, err := scheduler.normalizeSchedule(schedule)
		if err != nil {
			return nil, err
		}
		jsCopy := js
		if _, err := cronScheduler.AddFunc(jsCopy.Cron, func() {
			if scheduler.baseCtx == nil {
				return
			}
			if err := scheduler.runSchedule(scheduler.baseCtx, jsCopy); err != nil {
				scheduler.logger.Warn("library: schedule run failed", "error", err, "entityType", jsCopy.EntityType, "jobType", jsCopy.JobType)
			}
		}); err != nil {
			return nil, err
		}
		scheduler.schedules = append(scheduler.schedules, js)
	}

	return scheduler, nil
}

// Start begins executing configured schedules until the context is canceled.
func (s *Scheduler) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		s.baseCtx, s.cancel = context.WithCancel(ctx)
		s.cron.Start()
	})
}

// Stop halts schedule execution and waits for in-flight jobs to finish.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.cron != nil {
			if done := s.cron.Stop(); done != nil {
				<-done.Done()
			}
		}
	})
}

func (s *Scheduler) normalizeSchedule(schedule Schedule) (jobSchedule, error) {
	entityType := strings.TrimSpace(schedule.EntityType)
	if entityType == "" {
		return jobSchedule{}, errors.New("library: schedule entity type is required")
	}
	jobType := strings.TrimSpace(schedule.JobType)
	if jobType == "" {
		return jobSchedule{}, errors.New("library: schedule job type is required")
	}
	cronSpec := strings.TrimSpace(schedule.Cron)
	if cronSpec == "" {
		return jobSchedule{}, errors.New("library: schedule cron expression is required")
	}

	js := jobSchedule{
		Schedule: Schedule{
			EntityType:    entityType,
			JobType:       jobType,
			Cron:          cronSpec,
			PayloadURI:    strings.TrimSpace(schedule.PayloadURI),
			MaxConcurrent: schedule.MaxConcurrent,
		},
	}
	return js, nil
}

func (s *Scheduler) runSchedule(ctx context.Context, schedule jobSchedule) error {
	entities, err := s.service.ListEntities(ctx, ListEntitiesFilter{EntityType: schedule.EntityType})
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		return nil
	}

	entitySet := make(map[string]struct{}, len(entities))
	for _, entity := range entities {
		entitySet[entity.ID] = struct{}{}
	}

	jobs, err := s.service.ListJobs(ctx)
	if err != nil {
		return err
	}

	active := 0
	inFlight := make(map[string]struct{})
	for _, job := range jobs {
		if !strings.EqualFold(job.JobType, schedule.JobType) {
			continue
		}
		if _, ok := entitySet[job.TargetID]; !ok {
			continue
		}
		if job.State != JobStateDone {
			active++
			inFlight[job.TargetID] = struct{}{}
		}
	}

	limit := schedule.MaxConcurrent
	if limit < 0 {
		limit = 0
	}
	if limit > 0 && active >= limit {
		return nil
	}

	remaining := len(entities)
	if limit > 0 {
		remaining = limit - active
	}
	if remaining <= 0 {
		return nil
	}

	queued := 0
	for _, entity := range entities {
		if _, ok := inFlight[entity.ID]; ok {
			continue
		}
		if limit > 0 && queued >= remaining {
			break
		}
		input := TriggerJobInput{
			JobType:    schedule.JobType,
			TargetID:   entity.ID,
			PayloadURI: schedule.PayloadURI,
		}
		if _, err := s.service.TriggerJob(ctx, input); err != nil {
			if errors.Is(err, ErrDuplicateJob) || errors.Is(err, ErrUnknownEntity) {
				s.logger.Warn("library: skipping scheduled job", "targetID", entity.ID, "jobType", schedule.JobType, "error", err)
				continue
			}
			if errors.Is(err, ErrQueueUnavailable) {
				s.logger.Warn("library: queue unavailable for schedule", "jobType", schedule.JobType)
				return nil
			}
			return err
		}
		queued++
		inFlight[entity.ID] = struct{}{}
	}

	return nil
}
