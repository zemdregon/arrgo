package library

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Repository defines persistence operations for the library service.
type Repository interface {
	EnsureSchema(ctx context.Context) error
	UpsertEntity(ctx context.Context, entity Entity) error
	GetEntity(ctx context.Context, id string) (Entity, bool, error)
	ListEntities(ctx context.Context) ([]Entity, error)
	SaveJob(ctx context.Context, job Job) error
	UpdateJob(ctx context.Context, job Job) error
	ListJobs(ctx context.Context) ([]Job, error)
	ListJobsPage(ctx context.Context, opts ListJobsOptions) (ListJobsResult, error)
	AppendJobHistory(ctx context.Context, jobID string, record JobHistoryRecord, retention JobHistoryRetention) error
	ListJobHistories(ctx context.Context, jobIDs []string, limit int) (map[string][]JobHistoryRecord, error)
	UpsertMetadataOverrides(ctx context.Context, entityID string, overrides MetadataOverrides) error
	DeleteMetadataOverrides(ctx context.Context, entityID string) error
	GetMetadataOverrides(ctx context.Context, entityID string) (MetadataOverrides, bool, error)
	ListMetadataOverrides(ctx context.Context) (map[string]MetadataOverrides, error)
}

// InMemoryRepository provides an in-memory implementation of Repository for tests.
type InMemoryRepository struct {
	mu         sync.RWMutex
	entities   map[string]Entity
	jobs       map[string]Job
	jobHistory map[string][]JobHistoryRecord
	overrides  map[string]MetadataOverrides
}

// NewInMemoryRepository constructs an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		entities:   make(map[string]Entity),
		jobs:       make(map[string]Job),
		jobHistory: make(map[string][]JobHistoryRecord),
		overrides:  make(map[string]MetadataOverrides),
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

// GetEntity returns a single entity by id.
func (r *InMemoryRepository) GetEntity(_ context.Context, id string) (Entity, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return Entity{}, false, nil
	}
	entity, ok := r.entities[trimmedID]
	if !ok {
		return Entity{}, false, nil
	}
	return entity, true, nil
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
	canonical.ErrorMessage = strings.TrimSpace(canonical.ErrorMessage)
	canonical.History = nil

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
	canonical.ErrorMessage = strings.TrimSpace(canonical.ErrorMessage)
	canonical.History = nil

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

// ListJobsPage returns a single page of jobs ordered by submission time (newest first).
func (r *InMemoryRepository) ListJobsPage(_ context.Context, opts ListJobsOptions) (ListJobsResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultJobPageSize
	}

	jobs := make([]Job, 0, len(r.jobs))
	for _, job := range r.jobs {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].SubmittedAt.Equal(jobs[j].SubmittedAt) {
			return jobs[i].ID > jobs[j].ID
		}
		return jobs[i].SubmittedAt.After(jobs[j].SubmittedAt)
	})

	page := make([]Job, 0, limit+1)
	for _, job := range jobs {
		if opts.Cursor != nil {
			if job.SubmittedAt.After(opts.Cursor.SubmittedAt) {
				continue
			}
			if job.SubmittedAt.Equal(opts.Cursor.SubmittedAt) && job.ID >= opts.Cursor.JobID {
				continue
			}
		}
		page = append(page, job)
		if len(page) == limit+1 {
			break
		}
	}

	var next *JobListCursor
	if len(page) > limit {
		last := page[limit-1]
		next = &JobListCursor{SubmittedAt: last.SubmittedAt, JobID: last.ID}
		page = page[:limit]
	}

	return ListJobsResult{Jobs: append([]Job(nil), page...), NextCursor: next}, nil
}

// AppendJobHistory stores a history record while enforcing retention policies.
func (r *InMemoryRepository) AppendJobHistory(_ context.Context, jobID string, record JobHistoryRecord, retention JobHistoryRetention) error {
	trimmedID := strings.TrimSpace(jobID)
	if trimmedID == "" {
		return fmt.Errorf("library: job history job id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	record.State = strings.TrimSpace(record.State)
	record.Message = strings.TrimSpace(record.Message)
	if record.OccurredAt.IsZero() {
		record.OccurredAt = time.Now().UTC()
	} else {
		record.OccurredAt = record.OccurredAt.UTC()
	}
	if record.Attempt < 0 {
		record.Attempt = 0
	}

	history := append([]JobHistoryRecord(nil), r.jobHistory[trimmedID]...)
	history = append(history, record)

	sort.Slice(history, func(i, j int) bool {
		if history[i].OccurredAt.Equal(history[j].OccurredAt) {
			return history[i].Attempt > history[j].Attempt
		}
		return history[i].OccurredAt.After(history[j].OccurredAt)
	})

	if retention.MaxAge > 0 {
		cutoff := record.OccurredAt.Add(-retention.MaxAge)
		filtered := history[:0]
		for _, entry := range history {
			if entry.OccurredAt.Before(cutoff) {
				continue
			}
			filtered = append(filtered, entry)
		}
		history = filtered
	}

	if retention.MaxEntries > 0 && len(history) > retention.MaxEntries {
		history = history[:retention.MaxEntries]
	}

	r.jobHistory[trimmedID] = history
	return nil
}

// ListJobHistories returns the latest history entries for the requested jobs.
func (r *InMemoryRepository) ListJobHistories(_ context.Context, jobIDs []string, limit int) (map[string][]JobHistoryRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = defaultJobHistoryLimit
	}

	results := make(map[string][]JobHistoryRecord, len(jobIDs))
	for _, id := range jobIDs {
		trimmedID := strings.TrimSpace(id)
		if trimmedID == "" {
			continue
		}
		history := r.jobHistory[trimmedID]
		if len(history) == 0 {
			continue
		}
		take := limit
		if take > len(history) {
			take = len(history)
		}
		slice := make([]JobHistoryRecord, take)
		copy(slice, history[:take])
		results[trimmedID] = slice
	}
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

// GetMetadataOverrides returns overrides for a single entity when present.
func (r *InMemoryRepository) GetMetadataOverrides(_ context.Context, entityID string) (MetadataOverrides, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	trimmedID := strings.TrimSpace(entityID)
	if trimmedID == "" {
		return MetadataOverrides{}, false, nil
	}
	overrides, ok := r.overrides[trimmedID]
	if !ok {
		return MetadataOverrides{}, false, nil
	}
	return cloneMetadataOverrides(overrides), true, nil
}

func cloneJob(src Job) Job {
	dst := src
	if len(src.History) > 0 {
		dst.History = append([]JobHistoryRecord(nil), src.History...)
	}
	return dst
}
