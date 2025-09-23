package library

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Entity represents a top-level media record available to the automation stack.
type Entity struct {
	ID         string
	EntityType string
	Title      string
	Synopsis   string
}

// Job models an orchestration action scheduled against the library.
type Job struct {
	ID             string
	JobType        string
	TargetID       string
	State          string
	SubmittedAt    time.Time
	PayloadURI     string
	IdempotencyKey string
	Attempt        int
	LastAttemptAt  time.Time
	CompletedAt    time.Time

	StreamID string `json:"-"`
}

// Canonical job types emitted by the scheduler.
const (
	JobTypeScan   = "scan"
	JobTypeImport = "import"
)

// Job states returned by the orchestration API.
const (
	JobStateQueued  = "queued"
	JobStateRunning = "running"
	JobStateDone    = "completed"
)

// ListEntitiesFilter constrains a ListEntities call.
type ListEntitiesFilter struct {
	EntityType string
}

// TriggerJobInput describes a new orchestration job request.
type TriggerJobInput struct {
	JobType        string
	TargetID       string
	PayloadURI     string
	IdempotencyKey string
}

// Clock abstracts time retrieval for deterministic testing.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// IDGenerator produces stable identifiers for new jobs.
type IDGenerator func() string

// Config drives Service construction.
type Config struct {
	Entities       []Entity
	Jobs           []Job
	Clock          Clock
	IDGenerator    IDGenerator
	RedisClient    *redis.Client
	StreamName     string
	ConsumerGroup  string
	Logger         *slog.Logger
	IdempotencyTTL time.Duration
	AuditSink      AuditSink
}

// Errors returned by the Service APIs.
var (
	ErrInvalidJobType   = errors.New("library: job type is required")
	ErrInvalidTarget    = errors.New("library: target id is required")
	ErrUnknownEntity    = errors.New("library: target entity not found")
	ErrQueueUnavailable = errors.New("library: orchestration queue unavailable")
	ErrDuplicateJob     = errors.New("library: duplicate job for idempotency key")
)

// Service exposes read/write helpers that back both REST and gRPC front-ends.
type Service struct {
	mu           sync.RWMutex
	entities     []Entity
	entitiesByID map[string]Entity
	jobs         []Job
	jobIndex     map[string]int
	clock        Clock
	idGen        IDGenerator
	queue        *OrchestrationQueue
	logger       *slog.Logger
	auditSink    AuditSink
}

// NewService constructs a library orchestration service with optional seed data.
func NewService(cfg Config) (*Service, error) {
	svc := &Service{
		entities:  make([]Entity, len(cfg.Entities)),
		jobs:      make([]Job, len(cfg.Jobs)),
		jobIndex:  make(map[string]int, len(cfg.Jobs)),
		clock:     cfg.Clock,
		idGen:     cfg.IDGenerator,
		logger:    cfg.Logger,
		auditSink: cfg.AuditSink,
	}

	copy(svc.entities, cfg.Entities)
	copy(svc.jobs, cfg.Jobs)
	for idx := range svc.jobs {
		svc.jobIndex[svc.jobs[idx].ID] = idx
	}

	if svc.clock == nil {
		svc.clock = systemClock{}
	}
	if svc.idGen == nil {
		svc.idGen = func() string { return uuid.NewString() }
	}
	if svc.logger == nil {
		svc.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if svc.auditSink == nil {
		svc.auditSink = noopAuditSink{}
	}

	if len(svc.entities) == 0 {
		svc.entities = append(svc.entities, defaultEntities()...)
	}

	sort.Slice(svc.entities, func(i, j int) bool {
		return svc.entities[i].Title < svc.entities[j].Title
	})

	svc.entitiesByID = make(map[string]Entity, len(svc.entities))
	for _, entity := range svc.entities {
		svc.entitiesByID[entity.ID] = entity
	}

	if cfg.RedisClient != nil {
		queue, err := NewOrchestrationQueue(OrchestrationQueueConfig{
			Client:         cfg.RedisClient,
			Stream:         cfg.StreamName,
			Group:          cfg.ConsumerGroup,
			Logger:         svc.logger,
			IdempotencyTTL: cfg.IdempotencyTTL,
		})
		if err != nil {
			return nil, err
		}
		if err := queue.Initialize(context.Background()); err != nil {
			return nil, err
		}
		svc.queue = queue
	}

	return svc, nil
}

// ListEntities returns entities matching the optional filter.
func (s *Service) ListEntities(ctx context.Context, filter ListEntitiesFilter) ([]Entity, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	wantsType := strings.TrimSpace(strings.ToLower(filter.EntityType))

	results := make([]Entity, 0, len(s.entities))
	for _, entity := range s.entities {
		if wantsType != "" && strings.ToLower(entity.EntityType) != wantsType {
			continue
		}
		results = append(results, entity)
	}

	return append([]Entity(nil), results...), nil
}

// ListJobs returns job summaries ordered by submission time (newest first).
func (s *Service) ListJobs(ctx context.Context) ([]Job, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := append([]Job(nil), s.jobs...)
	sort.SliceStable(jobs, func(i, j int) bool {
		return jobs[i].SubmittedAt.After(jobs[j].SubmittedAt)
	})

	return jobs, nil
}

// TriggerJob enqueues a new orchestration job targeting a catalog entity.
func (s *Service) TriggerJob(ctx context.Context, input TriggerJobInput) (Job, error) {
	_ = ctx

	jobType := strings.TrimSpace(input.JobType)
	if jobType == "" {
		return Job{}, ErrInvalidJobType
	}
	targetID := strings.TrimSpace(input.TargetID)
	if targetID == "" {
		return Job{}, ErrInvalidTarget
	}

	if s.queue == nil {
		return Job{}, ErrQueueUnavailable
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.entityExistsLocked(targetID) {
		return Job{}, ErrUnknownEntity
	}

	job := Job{
		ID:             s.idGen(),
		JobType:        jobType,
		TargetID:       targetID,
		State:          JobStateQueued,
		SubmittedAt:    s.clock.Now(),
		PayloadURI:     strings.TrimSpace(input.PayloadURI),
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
	}
	streamID, err := s.queue.Enqueue(ctx, job)
	if err != nil {
		if errors.Is(err, ErrDuplicateJob) {
			return Job{}, err
		}
		return Job{}, err
	}
	job.StreamID = streamID
	s.jobs = append(s.jobs, job)
	s.jobIndex[job.ID] = len(s.jobs) - 1
	s.logger.Info("library job queued", "jobID", job.ID, "jobType", job.JobType, "targetID", job.TargetID, "idempotencyKey", job.IdempotencyKey, "streamID", streamID)

	return job, nil
}

// ClaimJobs reads from the orchestration queue for the provided consumer.
func (s *Service) ClaimJobs(ctx context.Context, consumer string, count int, block time.Duration) ([]ClaimedJob, error) {
	if s.queue == nil {
		return nil, ErrQueueUnavailable
	}

	claimed, err := s.queue.Claim(ctx, consumer, count, block)
	if err != nil {
		return nil, err
	}
	if len(claimed) == 0 {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range claimed {
		attemptTime := s.clock.Now()
		job := claimed[i].Job
		job.State = JobStateRunning
		job.LastAttemptAt = attemptTime
		claimed[i].Job = job

		s.updateJobLocked(job.ID, func(j *Job) {
			j.State = JobStateRunning
			j.Attempt = job.Attempt
			j.StreamID = claimed[i].StreamID
			j.LastAttemptAt = attemptTime
		})

		s.logger.Info("library job claimed", "jobID", job.ID, "streamID", claimed[i].StreamID, "consumer", consumer, "attempt", job.Attempt)
	}

	return claimed, nil
}

// AckJob acknowledges job completion to the orchestration queue.
func (s *Service) AckJob(ctx context.Context, job ClaimedJob) error {
	if s.queue == nil {
		return ErrQueueUnavailable
	}

	if err := s.queue.Ack(ctx, job.StreamID); err != nil {
		return err
	}

	completedAt := s.clock.Now()

	s.mu.Lock()
	s.updateJobLocked(job.Job.ID, func(j *Job) {
		j.State = JobStateDone
		j.StreamID = ""
		j.CompletedAt = completedAt
	})
	entityType := s.entityTypeLocked(job.Job.TargetID)
	s.mu.Unlock()

	s.logger.Info("library job acknowledged", "jobID", job.Job.ID, "streamID", job.StreamID, "attempt", job.Job.Attempt)

	if strings.EqualFold(job.Job.JobType, JobTypeImport) {
		event := AuditEvent{
			EventType:   AuditEventImportCompleted,
			JobID:       job.Job.ID,
			JobType:     job.Job.JobType,
			TargetID:    job.Job.TargetID,
			EntityType:  entityType,
			CompletedAt: completedAt,
		}
		if err := s.auditSink.Publish(ctx, event); err != nil {
			s.logger.Warn("library: audit sink publish failed", "error", err)
		}
	}

	return nil
}

// RetryJob re-queues a claimed job for another attempt.
func (s *Service) RetryJob(ctx context.Context, job ClaimedJob) (Job, error) {
	if s.queue == nil {
		return Job{}, ErrQueueUnavailable
	}

	streamID, err := s.queue.Retry(ctx, job.StreamID, job.Job)
	if err != nil {
		return Job{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var updated Job
	s.updateJobLocked(job.Job.ID, func(j *Job) {
		j.State = JobStateQueued
		j.StreamID = streamID
		updated = *j
	})

	if updated.ID == "" {
		updated = job.Job
		updated.State = JobStateQueued
	}

	s.logger.Info("library job retried", "jobID", updated.ID, "previousStreamID", job.StreamID, "streamID", streamID, "attempt", job.Job.Attempt+1)

	return updated, nil
}

func (s *Service) entityExistsLocked(id string) bool {
	_, ok := s.entitiesByID[id]
	return ok
}

func (s *Service) updateJobLocked(id string, fn func(*Job)) {
	idx, ok := s.jobIndex[id]
	if !ok {
		return
	}
	job := s.jobs[idx]
	fn(&job)
	s.jobs[idx] = job
}

func (s *Service) entityTypeLocked(id string) string {
	if entity, ok := s.entitiesByID[id]; ok {
		return entity.EntityType
	}
	return ""
}

func defaultEntities() []Entity {
	return []Entity{
		{
			ID:         "series-1",
			EntityType: "series",
			Title:      "The Expanse",
			Synopsis:   "A space opera following the crew of the Rocinante as they unravel a solar system conspiracy.",
		},
		{
			ID:         "movie-1",
			EntityType: "movie",
			Title:      "Blade Runner 2049",
			Synopsis:   "A young blade runner unearths a long-buried secret that leads him to former blade runner Rick Deckard.",
		},
		{
			ID:         "artist-1",
			EntityType: "artist",
			Title:      "Daft Punk",
			Synopsis:   "The French electronic music duo celebrated for their pioneering house and synth-pop records.",
		},
	}
}
