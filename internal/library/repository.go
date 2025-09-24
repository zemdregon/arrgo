package library

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Repository defines persistence operations for the library service.
type Repository interface {
	EnsureSchema(ctx context.Context) error
	UpsertEntity(ctx context.Context, entity Entity) error
	ListEntities(ctx context.Context) ([]Entity, error)
	SaveJob(ctx context.Context, job Job) error
	UpdateJob(ctx context.Context, job Job) error
	ListJobs(ctx context.Context) ([]Job, error)
	UpsertMetadataOverrides(ctx context.Context, entityID string, overrides MetadataOverrides) error
	DeleteMetadataOverrides(ctx context.Context, entityID string) error
	ListMetadataOverrides(ctx context.Context) (map[string]MetadataOverrides, error)
}

// InMemoryRepository provides an in-memory implementation of Repository for tests.
type InMemoryRepository struct {
	mu        sync.RWMutex
	entities  map[string]Entity
	jobs      map[string]Job
	overrides map[string]MetadataOverrides
}

// NewInMemoryRepository constructs an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		entities:  make(map[string]Entity),
		jobs:      make(map[string]Job),
		overrides: make(map[string]MetadataOverrides),
	}
}

// EnsureSchema fulfills the Repository contract; no-op for in-memory repo.
func (r *InMemoryRepository) EnsureSchema(context.Context) error {
	return nil
}

// UpsertEntity creates or replaces an entity record.
func (r *InMemoryRepository) UpsertEntity(_ context.Context, entity Entity) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	trimmed := Entity{
		ID:         strings.TrimSpace(entity.ID),
		EntityType: strings.TrimSpace(entity.EntityType),
		Title:      strings.TrimSpace(entity.Title),
		Synopsis:   strings.TrimSpace(entity.Synopsis),
	}
	if trimmed.ID == "" {
		return fmt.Errorf("library: entity id is required")
	}
	r.entities[trimmed.ID] = trimmed
	return nil
}

// ListEntities returns all entities sorted by title.
func (r *InMemoryRepository) ListEntities(_ context.Context) ([]Entity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]Entity, 0, len(r.entities))
	for _, entity := range r.entities {
		results = append(results, entity)
	}
	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Title) < strings.ToLower(results[j].Title)
	})
	return results, nil
}

// SaveJob stores a new job record.
func (r *InMemoryRepository) SaveJob(_ context.Context, job Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if strings.TrimSpace(job.ID) == "" {
		return fmt.Errorf("library: job id is required")
	}

	canonical := cloneJob(job)
	canonical.SubmittedAt = canonical.SubmittedAt.UTC()
	canonical.LastAttemptAt = canonical.LastAttemptAt.UTC()
	canonical.CompletedAt = canonical.CompletedAt.UTC()

	r.jobs[canonical.ID] = canonical
	return nil
}

// UpdateJob replaces an existing job record.
func (r *InMemoryRepository) UpdateJob(_ context.Context, job Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	canonical := cloneJob(job)
	canonical.SubmittedAt = canonical.SubmittedAt.UTC()
	canonical.LastAttemptAt = canonical.LastAttemptAt.UTC()
	canonical.CompletedAt = canonical.CompletedAt.UTC()

	if _, exists := r.jobs[canonical.ID]; !exists {
		return fmt.Errorf("library: job %q not found", canonical.ID)
	}
	r.jobs[canonical.ID] = canonical
	return nil
}

// ListJobs returns the stored jobs ordered by submission time (newest first).
func (r *InMemoryRepository) ListJobs(_ context.Context) ([]Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]Job, 0, len(r.jobs))
	for _, job := range r.jobs {
		results = append(results, job)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].SubmittedAt.After(results[j].SubmittedAt)
	})
	return results, nil
}

// UpsertMetadataOverrides stores metadata overrides for an entity.
func (r *InMemoryRepository) UpsertMetadataOverrides(_ context.Context, entityID string, overrides MetadataOverrides) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	trimmedID := strings.TrimSpace(entityID)
	if trimmedID == "" {
		return fmt.Errorf("library: metadata override entity id is required")
	}

	normalized := cloneMetadataOverrides(overrides)
	normalized.UpdatedAt = normalized.UpdatedAt.UTC()
	r.overrides[trimmedID] = normalized
	return nil
}

// DeleteMetadataOverrides removes overrides for the provided entity.
func (r *InMemoryRepository) DeleteMetadataOverrides(_ context.Context, entityID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	trimmedID := strings.TrimSpace(entityID)
	if trimmedID == "" {
		return fmt.Errorf("library: metadata override entity id is required")
	}

	delete(r.overrides, trimmedID)
	return nil
}

// ListMetadataOverrides returns all stored overrides keyed by entity id.
func (r *InMemoryRepository) ListMetadataOverrides(_ context.Context) (map[string]MetadataOverrides, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make(map[string]MetadataOverrides, len(r.overrides))
	for entityID, overrides := range r.overrides {
		results[entityID] = cloneMetadataOverrides(overrides)
	}
	return results, nil
}

func cloneJob(src Job) Job {
	dst := src
	return dst
}
