package library

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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
	ID          string
	JobType     string
	TargetID    string
	State       string
	SubmittedAt time.Time
}

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
	JobType  string
	TargetID string
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
	Entities    []Entity
	Jobs        []Job
	Clock       Clock
	IDGenerator IDGenerator
}

// Errors returned by the Service APIs.
var (
	ErrInvalidJobType = errors.New("library: job type is required")
	ErrInvalidTarget  = errors.New("library: target id is required")
	ErrUnknownEntity  = errors.New("library: target entity not found")
)

// Service exposes read/write helpers that back both REST and gRPC front-ends.
type Service struct {
	mu       sync.RWMutex
	entities []Entity
	jobs     []Job
	clock    Clock
	idGen    IDGenerator
}

// NewService constructs a library orchestration service with optional seed data.
func NewService(cfg Config) *Service {
	svc := &Service{
		entities: make([]Entity, len(cfg.Entities)),
		jobs:     make([]Job, len(cfg.Jobs)),
		clock:    cfg.Clock,
		idGen:    cfg.IDGenerator,
	}

	copy(svc.entities, cfg.Entities)
	copy(svc.jobs, cfg.Jobs)

	if svc.clock == nil {
		svc.clock = systemClock{}
	}
	if svc.idGen == nil {
		svc.idGen = func() string { return uuid.NewString() }
	}

	if len(svc.entities) == 0 {
		svc.entities = append(svc.entities, defaultEntities()...)
	}

	sort.Slice(svc.entities, func(i, j int) bool {
		return svc.entities[i].Title < svc.entities[j].Title
	})

	return svc
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.entityExistsLocked(targetID) {
		return Job{}, ErrUnknownEntity
	}

	job := Job{
		ID:          s.idGen(),
		JobType:     jobType,
		TargetID:    targetID,
		State:       JobStateQueued,
		SubmittedAt: s.clock.Now(),
	}

	s.jobs = append(s.jobs, job)

	return job, nil
}

func (s *Service) entityExistsLocked(id string) bool {
	for _, entity := range s.entities {
		if entity.ID == id {
			return true
		}
	}
	return false
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
