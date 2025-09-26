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
	SaveRequest(ctx context.Context, request Request) error
	UpdateRequest(ctx context.Context, request Request) error
	GetRequest(ctx context.Context, id string) (Request, bool, error)
	ListRequests(ctx context.Context) ([]Request, error)
	AppendRequestApproval(ctx context.Context, event RequestApprovalEvent) error
	ListRequestApprovals(ctx context.Context, requestID string) ([]RequestApprovalEvent, error)
	AppendRequestNotification(ctx context.Context, notification RequestNotification) error
	ListRequestNotifications(ctx context.Context, requestID string) ([]RequestNotification, error)
}

// InMemoryRepository provides an in-memory implementation of Repository for tests.
type InMemoryRepository struct {
	mu         sync.RWMutex
	entities   map[string]Entity
	jobs       map[string]Job
	jobHistory map[string][]JobHistoryRecord
	overrides  map[string]MetadataOverrides
	requests   map[string]Request
	approvals  map[string][]RequestApprovalEvent
	notify     map[string][]RequestNotification
}

// NewInMemoryRepository constructs an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		entities:   make(map[string]Entity),
		jobs:       make(map[string]Job),
		jobHistory: make(map[string][]JobHistoryRecord),
		overrides:  make(map[string]MetadataOverrides),
		requests:   make(map[string]Request),
		approvals:  make(map[string][]RequestApprovalEvent),
		notify:     make(map[string][]RequestNotification),
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

// SaveRequest inserts a new request record.
func (r *InMemoryRepository) SaveRequest(_ context.Context, request Request) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	canonical := canonicalizeRequest(request)
	if canonical.ID == "" {
		return fmt.Errorf("library: request id is required")
	}
	if canonical.Kind == "" {
		return fmt.Errorf("library: request kind is required")
	}
	if canonical.RequestedBy == "" {
		return fmt.Errorf("library: request requester is required")
	}
	if canonical.SubmittedAt.IsZero() {
		canonical.SubmittedAt = time.Now().UTC()
	}
	if canonical.UpdatedAt.IsZero() || canonical.UpdatedAt.Before(canonical.SubmittedAt) {
		canonical.UpdatedAt = canonical.SubmittedAt
	}

	if _, exists := r.requests[canonical.ID]; exists {
		return fmt.Errorf("library: request %q already exists", canonical.ID)
	}
	r.requests[canonical.ID] = canonical
	r.approvals[canonical.ID] = nil
	r.notify[canonical.ID] = nil
	return nil
}

// UpdateRequest replaces an existing request record.
func (r *InMemoryRepository) UpdateRequest(_ context.Context, request Request) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	canonical := canonicalizeRequest(request)
	if canonical.ID == "" {
		return fmt.Errorf("library: request id is required")
	}
	if canonical.SubmittedAt.IsZero() {
		canonical.SubmittedAt = time.Now().UTC()
	}
	if canonical.UpdatedAt.IsZero() || canonical.UpdatedAt.Before(canonical.SubmittedAt) {
		canonical.UpdatedAt = canonical.SubmittedAt
	}

	if _, exists := r.requests[canonical.ID]; !exists {
		return fmt.Errorf("library: request %q not found", canonical.ID)
	}
	r.requests[canonical.ID] = canonical
	return nil
}

// GetRequest returns a single request with associated events.
func (r *InMemoryRepository) GetRequest(_ context.Context, id string) (Request, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return Request{}, false, nil
	}
	request, ok := r.requests[trimmed]
	if !ok {
		return Request{}, false, nil
	}
	cloned := cloneRequest(request)
	cloned.Approvals = cloneApprovalsLocked(r.approvals[trimmed])
	cloned.Notifications = cloneNotificationsLocked(r.notify[trimmed])
	return cloned, true, nil
}

// ListRequests returns stored requests ordered by submission time.
func (r *InMemoryRepository) ListRequests(_ context.Context) ([]Request, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]Request, 0, len(r.requests))
	for _, request := range r.requests {
		cloned := cloneRequest(request)
		cloned.Approvals = nil
		cloned.Notifications = nil
		results = append(results, cloned)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].SubmittedAt.Equal(results[j].SubmittedAt) {
			return results[i].ID > results[j].ID
		}
		return results[i].SubmittedAt.After(results[j].SubmittedAt)
	})
	return results, nil
}

// AppendRequestApproval records an approval event.
func (r *InMemoryRepository) AppendRequestApproval(_ context.Context, event RequestApprovalEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	canonical := canonicalizeRequestApproval(event)
	if canonical.ID == "" {
		return fmt.Errorf("library: approval id is required")
	}
	if canonical.RequestID == "" {
		return fmt.Errorf("library: approval request id is required")
	}
	if canonical.Decision == "" {
		return fmt.Errorf("library: approval decision is required")
	}
	if canonical.Actor == "" {
		return fmt.Errorf("library: approval actor is required")
	}
	if canonical.DecidedAt.IsZero() {
		canonical.DecidedAt = time.Now().UTC()
	}

	if _, exists := r.requests[canonical.RequestID]; !exists {
		return fmt.Errorf("library: request %q not found", canonical.RequestID)
	}
	events := r.approvals[canonical.RequestID]
	for _, existing := range events {
		if existing.ID == canonical.ID {
			return fmt.Errorf("library: approval %q already exists", canonical.ID)
		}
	}
	events = append(events, canonical)
	r.approvals[canonical.RequestID] = sortApprovals(events)
	return nil
}

// ListRequestApprovals returns approval events for a request.
func (r *InMemoryRepository) ListRequestApprovals(_ context.Context, requestID string) ([]RequestApprovalEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	trimmed := strings.TrimSpace(requestID)
	if trimmed == "" {
		return nil, nil
	}
	return cloneApprovalsLocked(r.approvals[trimmed]), nil
}

// AppendRequestNotification stores a notification event.
func (r *InMemoryRepository) AppendRequestNotification(_ context.Context, notification RequestNotification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	canonical := canonicalizeRequestNotification(notification)
	if canonical.ID == "" {
		return fmt.Errorf("library: notification id is required")
	}
	if canonical.RequestID == "" {
		return fmt.Errorf("library: notification request id is required")
	}
	if canonical.Channel == "" {
		return fmt.Errorf("library: notification channel is required")
	}
	if canonical.Target == "" {
		return fmt.Errorf("library: notification target is required")
	}
	if canonical.AttemptedAt.IsZero() {
		canonical.AttemptedAt = time.Now().UTC()
	}

	if _, exists := r.requests[canonical.RequestID]; !exists {
		return fmt.Errorf("library: request %q not found", canonical.RequestID)
	}
	events := r.notify[canonical.RequestID]
	for _, existing := range events {
		if existing.ID == canonical.ID {
			return fmt.Errorf("library: notification %q already exists", canonical.ID)
		}
	}
	events = append(events, canonical)
	r.notify[canonical.RequestID] = sortNotifications(events)
	return nil
}

// ListRequestNotifications returns notification events for a request.
func (r *InMemoryRepository) ListRequestNotifications(_ context.Context, requestID string) ([]RequestNotification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	trimmed := strings.TrimSpace(requestID)
	if trimmed == "" {
		return nil, nil
	}
	return cloneNotificationsLocked(r.notify[trimmed]), nil
}

func cloneApprovalsLocked(events []RequestApprovalEvent) []RequestApprovalEvent {
	if len(events) == 0 {
		return nil
	}
	cloned := make([]RequestApprovalEvent, len(events))
	for i := range events {
		cloned[i] = cloneRequestApproval(events[i])
	}
	return cloned
}

func cloneNotificationsLocked(events []RequestNotification) []RequestNotification {
	if len(events) == 0 {
		return nil
	}
	cloned := make([]RequestNotification, len(events))
	for i := range events {
		cloned[i] = cloneRequestNotification(events[i])
	}
	return cloned
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
