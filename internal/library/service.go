package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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

// Manual release statuses exposed by manual search helpers.
const (
	ManualReleaseStatusAvailable = "available"
	ManualReleaseStatusGrabbed   = "grabbed"
	ManualReleaseStatusRejected  = "rejected"
)

// ManualSearchResult captures the outcome of an operator-driven indexer search.
type ManualSearchResult struct {
	ID         string
	EntityID   string
	Query      string
	ExecutedAt time.Time
	Results    []ManualSearchRelease
}

// ManualSearchRelease describes a candidate release surfaced by a manual search.
type ManualSearchRelease struct {
	ID              string
	SearchID        string
	EntityID        string
	Title           string
	Indexer         string
	Protocol        string
	Quality         string
	PublishDate     time.Time
	SizeBytes       int64
	Seeders         int
	Grabs           int
	Score           float64
	Status          string
	RejectionReason string
	DownloadURL     string
	GrabbedAt       time.Time
}

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
	Repository        Repository
	Entities          []Entity
	Jobs              []Job
	Clock             Clock
	IDGenerator       IDGenerator
	RedisClient       *redis.Client
	StreamName        string
	ConsumerGroup     string
	Logger            *slog.Logger
	IdempotencyTTL    time.Duration
	AuditSink         AuditSink
	Tracer            trace.Tracer
	RenameTemplates   RenameTemplates
	LibraryFiles      map[string][]LibraryFile
	MetadataOverrides map[string]MetadataOverrides
}

// Errors returned by the Service APIs.
var (
	ErrInvalidJobType              = errors.New("library: job type is required")
	ErrInvalidTarget               = errors.New("library: target id is required")
	ErrUnknownEntity               = errors.New("library: target entity not found")
	ErrQueueUnavailable            = errors.New("library: orchestration queue unavailable")
	ErrDuplicateJob                = errors.New("library: duplicate job for idempotency key")
	ErrManualSearchEntityRequired  = errors.New("library: manual search entity id is required")
	ErrManualSearchReleaseRequired = errors.New("library: manual search release id is required")
	ErrManualSearchReleaseNotFound = errors.New("library: manual search release not found")
	ErrManualSearchReleaseGrabbed  = errors.New("library: manual search release already grabbed")
)

// Service exposes read/write helpers that back both REST and gRPC front-ends.
type Service struct {
	mu              sync.RWMutex
	repository      Repository
	clock           Clock
	idGen           IDGenerator
	queue           *OrchestrationQueue
	logger          *slog.Logger
	auditSink       AuditSink
	tracer          trace.Tracer
	manualSearches  map[string]ManualSearchResult
	manualReleases  map[string]ManualSearchRelease
	renameTemplates RenameTemplates
	renameHistory   map[string][]RenameJobRecord
	renameState     map[string]map[string]renameState
	libraryFiles    map[string][]LibraryFile
	libraryFileIdx  map[string]map[string]int
}

// NewService constructs a library orchestration service with optional seed data.
func NewService(cfg Config) (*Service, error) {
	svc := &Service{
		clock:          cfg.Clock,
		idGen:          cfg.IDGenerator,
		logger:         cfg.Logger,
		auditSink:      cfg.AuditSink,
		renameHistory:  make(map[string][]RenameJobRecord),
		renameState:    make(map[string]map[string]renameState),
		libraryFiles:   make(map[string][]LibraryFile),
		libraryFileIdx: make(map[string]map[string]int),
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
	svc.tracer = cfg.Tracer
	if svc.tracer == nil {
		svc.tracer = otel.Tracer("github.com/arrgo/arrgo/internal/library")
	}

	svc.manualSearches = make(map[string]ManualSearchResult)
	svc.manualReleases = make(map[string]ManualSearchRelease)
	svc.renameTemplates = cfg.RenameTemplates.WithDefaults()
	if err := svc.renameTemplates.Validate(); err != nil {
		return nil, err
	}
	for entityID, files := range cfg.LibraryFiles {
		copies := make([]LibraryFile, len(files))
		copy(copies, files)
		svc.libraryFiles[entityID] = copies
		idx := make(map[string]int, len(copies))
		for i := range copies {
			idx[copies[i].ID] = i
		}
		svc.libraryFileIdx[entityID] = idx
	}

	repo := cfg.Repository
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	svc.repository = repo

	seedCtx := context.Background()
	for _, entity := range cfg.Entities {
		if err := svc.repository.UpsertEntity(seedCtx, entity); err != nil {
			return nil, fmt.Errorf("library: seed entity %q: %w", entity.ID, err)
		}
	}
	for _, job := range cfg.Jobs {
		if err := svc.repository.SaveJob(seedCtx, job); err != nil {
			return nil, fmt.Errorf("library: seed job %q: %w", job.ID, err)
		}
	}
	for entityID, overrides := range cfg.MetadataOverrides {
		o := cloneMetadataOverrides(overrides)
		if o.UpdatedAt.IsZero() {
			o.UpdatedAt = svc.clock.Now()
		}
		if err := svc.repository.UpsertMetadataOverrides(seedCtx, entityID, o); err != nil {
			return nil, fmt.Errorf("library: seed metadata overrides for %q: %w", entityID, err)
		}
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

func (s *Service) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	options := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindInternal)}
	if len(attrs) > 0 {
		options = append(options, trace.WithAttributes(attrs...))
	}
	return s.tracer.Start(ctx, name, options...)
}

func endSpan(span trace.Span, err *error) {
	if err != nil && *err != nil {
		span.RecordError(*err)
		span.SetStatus(codes.Error, (*err).Error())
	} else {
		span.SetStatus(codes.Ok, "ok")
	}
	span.End()
}

// ListEntities returns entities matching the optional filter.
func (s *Service) ListEntities(ctx context.Context, filter ListEntitiesFilter) (entities []Entity, err error) {
	ctx, span := s.startSpan(ctx, "library.Service.ListEntities",
		attribute.String("library.entity_type", strings.TrimSpace(filter.EntityType)),
	)
	defer endSpan(span, &err)

	records, err := s.repository.ListEntities(ctx)
	if err != nil {
		return nil, fmt.Errorf("library: list entities: %w", err)
	}
	sorted := append([]Entity(nil), records...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Title) < strings.ToLower(sorted[j].Title)
	})

	wantsType := strings.TrimSpace(strings.ToLower(filter.EntityType))
	results := make([]Entity, 0, len(sorted))
	for _, entity := range sorted {
		if wantsType != "" && strings.ToLower(entity.EntityType) != wantsType {
			continue
		}
		results = append(results, entity)
	}
	return append([]Entity(nil), results...), nil
}

// ListJobs returns job summaries ordered by submission time (newest first).
func (s *Service) ListJobs(ctx context.Context) (jobs []Job, err error) {
	ctx, span := s.startSpan(ctx, "library.Service.ListJobs")
	defer endSpan(span, &err)

	records, err := s.repository.ListJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("library: list jobs: %w", err)
	}
	jobs = append([]Job(nil), records...)
	sort.SliceStable(jobs, func(i, j int) bool {
		return jobs[i].SubmittedAt.After(jobs[j].SubmittedAt)
	})

	span.SetAttributes(attribute.Int("library.job_count", len(jobs)))
	return jobs, nil
}

// RunManualSearch synthesizes indexer results for an entity so operators can review candidates.
func (s *Service) RunManualSearch(ctx context.Context, entityID, query string) (result ManualSearchResult, err error) {
	ctx, span := s.startSpan(ctx, "library.Service.RunManualSearch",
		attribute.String("library.entity_id", strings.TrimSpace(entityID)),
		attribute.String("library.manual_query", strings.TrimSpace(query)),
	)
	defer endSpan(span, &err)

	trimmedID := strings.TrimSpace(entityID)
	if trimmedID == "" {
		err = ErrManualSearchEntityRequired
		return
	}

	entity, found, lookupErr := s.lookupEntity(ctx, trimmedID)
	if lookupErr != nil {
		err = lookupErr
		return
	}
	if !found {
		err = ErrUnknownEntity
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	executedAt := s.clock.Now()
	searchID := uuid.NewString()
	trimmedQuery := strings.TrimSpace(query)

	if previous, ok := s.manualSearches[trimmedID]; ok {
		for _, release := range previous.Results {
			delete(s.manualReleases, release.ID)
		}
	}

	releases := s.buildManualSearchReleases(entity, searchID, trimmedQuery, executedAt)
	result = ManualSearchResult{
		ID:         searchID,
		EntityID:   trimmedID,
		Query:      trimmedQuery,
		ExecutedAt: executedAt,
		Results:    releases,
	}
	s.manualSearches[trimmedID] = result
	for _, release := range releases {
		s.manualReleases[release.ID] = release
	}

	span.SetAttributes(
		attribute.String("library.manual_search_id", searchID),
		attribute.Int("library.manual_search_result_count", len(releases)),
	)
	s.logger.Info("library manual search completed", "entityID", trimmedID, "query", trimmedQuery, "results", len(releases))

	return cloneManualSearchResult(result), nil
}

// LatestManualSearch returns the most recent manual search results for an entity.
func (s *Service) LatestManualSearch(ctx context.Context, entityID string) (result ManualSearchResult, found bool, err error) {
	ctx, span := s.startSpan(ctx, "library.Service.LatestManualSearch",
		attribute.String("library.entity_id", strings.TrimSpace(entityID)),
	)
	defer endSpan(span, &err)

	trimmedID := strings.TrimSpace(entityID)
	if trimmedID == "" {
		err = ErrManualSearchEntityRequired
		return
	}

	exists, lookupErr := s.entityExists(ctx, trimmedID)
	if lookupErr != nil {
		err = lookupErr
		return
	}
	if !exists {
		err = ErrUnknownEntity
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	search, ok := s.manualSearches[trimmedID]
	if !ok {
		return ManualSearchResult{}, false, nil
	}

	span.SetAttributes(
		attribute.String("library.manual_search_id", search.ID),
		attribute.Int("library.manual_search_result_count", len(search.Results)),
	)
	return cloneManualSearchResult(search), true, nil
}

// ForceGrabRelease marks a manual search candidate as grabbed so operators can override automation.
func (s *Service) ForceGrabRelease(ctx context.Context, entityID, releaseID string) (release ManualSearchRelease, err error) {
	ctx, span := s.startSpan(ctx, "library.Service.ForceGrabRelease",
		attribute.String("library.entity_id", strings.TrimSpace(entityID)),
		attribute.String("library.release_id", strings.TrimSpace(releaseID)),
	)
	defer endSpan(span, &err)

	trimmedEntityID := strings.TrimSpace(entityID)
	if trimmedEntityID == "" {
		err = ErrManualSearchEntityRequired
		return
	}
	trimmedReleaseID := strings.TrimSpace(releaseID)
	if trimmedReleaseID == "" {
		err = ErrManualSearchReleaseRequired
		return
	}

	exists, lookupErr := s.entityExists(ctx, trimmedEntityID)
	if lookupErr != nil {
		err = lookupErr
		return
	}
	if !exists {
		err = ErrUnknownEntity
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.manualReleases[trimmedReleaseID]
	if !ok || existing.EntityID != trimmedEntityID {
		err = ErrManualSearchReleaseNotFound
		return
	}
	if existing.Status == ManualReleaseStatusGrabbed {
		err = ErrManualSearchReleaseGrabbed
		return
	}

	existing.Status = ManualReleaseStatusGrabbed
	existing.GrabbedAt = s.clock.Now()
	s.manualReleases[existing.ID] = existing
	s.updateManualSearchReleaseLocked(existing)

	span.SetAttributes(
		attribute.String("library.manual_search_id", existing.SearchID),
		attribute.String("library.manual_release_status", existing.Status),
	)
	s.logger.Info("library manual release grabbed", "entityID", existing.EntityID, "releaseID", existing.ID)

	return existing, nil
}

// TriggerJob enqueues a new orchestration job targeting a catalog entity.
func (s *Service) TriggerJob(ctx context.Context, input TriggerJobInput) (job Job, err error) {
	ctx, span := s.startSpan(ctx, "library.Service.TriggerJob",
		attribute.String("library.job_type", strings.TrimSpace(input.JobType)),
		attribute.String("library.target_id", strings.TrimSpace(input.TargetID)),
		attribute.String("library.idempotency_key", strings.TrimSpace(input.IdempotencyKey)),
	)
	defer endSpan(span, &err)

	jobType := strings.TrimSpace(input.JobType)
	if jobType == "" {
		err = ErrInvalidJobType
		return
	}
	targetID := strings.TrimSpace(input.TargetID)
	if targetID == "" {
		err = ErrInvalidTarget
		return
	}

	if s.queue == nil {
		err = ErrQueueUnavailable
		return
	}

	exists, lookupErr := s.entityExists(ctx, targetID)
	if lookupErr != nil {
		err = lookupErr
		return
	}
	if !exists {
		err = ErrUnknownEntity
		return
	}

	job = Job{
		ID:             s.idGen(),
		JobType:        jobType,
		TargetID:       targetID,
		State:          JobStateQueued,
		SubmittedAt:    s.clock.Now(),
		PayloadURI:     strings.TrimSpace(input.PayloadURI),
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
	}
	var streamID string
	streamID, err = s.queue.Enqueue(ctx, job)
	if err != nil {
		job = Job{}
		return
	}
	job.StreamID = streamID
	if repoErr := s.repository.SaveJob(ctx, job); repoErr != nil {
		if delErr := s.queue.Delete(ctx, streamID); delErr != nil {
			s.logger.Warn("library: rollback queued job failed", "jobID", job.ID, "streamID", streamID, "error", delErr)
		}
		err = fmt.Errorf("library: persist job: %w", repoErr)
		job = Job{}
		return
	}

	span.SetAttributes(
		attribute.String("library.job_id", job.ID),
		attribute.String("library.stream_id", streamID),
	)
	s.logger.Info("library job queued", "jobID", job.ID, "jobType", job.JobType, "targetID", job.TargetID, "idempotencyKey", job.IdempotencyKey, "streamID", streamID)

	return job, nil
}

// ClaimJobs reads from the orchestration queue for the provided consumer.
func (s *Service) ClaimJobs(ctx context.Context, consumer string, count int, block time.Duration) (claimed []ClaimedJob, err error) {
	ctx, span := s.startSpan(ctx, "library.Service.ClaimJobs",
		attribute.String("library.consumer", strings.TrimSpace(consumer)),
		attribute.Int("library.requested_count", count),
		attribute.String("library.block", block.String()),
	)
	defer endSpan(span, &err)

	if s.queue == nil {
		err = ErrQueueUnavailable
		return
	}

	claimed, err = s.queue.Claim(ctx, consumer, count, block)
	if err != nil || len(claimed) == 0 {
		return
	}

	updates := make([]Job, 0, len(claimed))
	for i := range claimed {
		attemptTime := s.clock.Now()
		job := claimed[i].Job
		job.State = JobStateRunning
		job.LastAttemptAt = attemptTime
		job.StreamID = claimed[i].StreamID
		claimed[i].Job = job
		updates = append(updates, job)
	}

	for _, updated := range updates {
		if repoErr := s.repository.UpdateJob(ctx, updated); repoErr != nil {
			return nil, fmt.Errorf("library: persist job update: %w", repoErr)
		}
	}

	for _, job := range claimed {
		s.logger.Info("library job claimed", "jobID", job.Job.ID, "streamID", job.StreamID, "consumer", consumer, "attempt", job.Job.Attempt)
	}

	span.SetAttributes(attribute.Int("library.claimed_count", len(claimed)))
	return claimed, nil
}

// AckJob acknowledges job completion to the orchestration queue.
func (s *Service) AckJob(ctx context.Context, job ClaimedJob) (err error) {
	ctx, span := s.startSpan(ctx, "library.Service.AckJob",
		attribute.String("library.job_id", job.Job.ID),
		attribute.String("library.stream_id", job.StreamID),
		attribute.String("library.job_type", job.Job.JobType),
		attribute.String("library.target_id", job.Job.TargetID),
	)
	defer endSpan(span, &err)

	if s.queue == nil {
		err = ErrQueueUnavailable
		return
	}

	if err = s.queue.Ack(ctx, job.StreamID); err != nil {
		return
	}

	completedAt := s.clock.Now()

	updated := job.Job
	updated.State = JobStateDone
	updated.StreamID = ""
	updated.CompletedAt = completedAt
	if repoErr := s.repository.UpdateJob(ctx, updated); repoErr != nil {
		return fmt.Errorf("library: persist job update: %w", repoErr)
	}

	entityType, typeErr := s.entityType(ctx, job.Job.TargetID)
	if typeErr != nil {
		s.logger.Warn("library: failed to load entity for audit", "jobID", job.Job.ID, "targetID", job.Job.TargetID, "error", typeErr)
		entityType = ""
	}

	span.SetAttributes(attribute.String("library.entity_type", entityType))
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
func (s *Service) RetryJob(ctx context.Context, job ClaimedJob) (updated Job, err error) {
	ctx, span := s.startSpan(ctx, "library.Service.RetryJob",
		attribute.String("library.job_id", job.Job.ID),
		attribute.String("library.stream_id", job.StreamID),
		attribute.String("library.job_type", job.Job.JobType),
		attribute.String("library.target_id", job.Job.TargetID),
	)
	defer endSpan(span, &err)

	if s.queue == nil {
		err = ErrQueueUnavailable
		return
	}

	var streamID string
	streamID, err = s.queue.Retry(ctx, job.StreamID, job.Job)
	if err != nil {
		return
	}

	updated = job.Job
	updated.State = JobStateQueued
	updated.StreamID = streamID
	if repoErr := s.repository.UpdateJob(ctx, updated); repoErr != nil {
		return Job{}, fmt.Errorf("library: persist job update: %w", repoErr)
	}

	span.SetAttributes(
		attribute.String("library.stream_id", streamID),
		attribute.Int("library.next_attempt", job.Job.Attempt+1),
	)
	s.logger.Info("library job retried", "jobID", updated.ID, "previousStreamID", job.StreamID, "streamID", streamID, "attempt", job.Job.Attempt+1)

	return updated, nil
}

func (s *Service) buildManualSearchReleases(entity Entity, searchID, query string, now time.Time) []ManualSearchRelease {
	title := entity.Title
	if strings.TrimSpace(title) == "" {
		title = entity.ID
	}
	highlight := title
	if query != "" {
		highlight = fmt.Sprintf("%s · %s", title, query)
	}

	releases := []ManualSearchRelease{
		{
			ID:          uuid.NewString(),
			SearchID:    searchID,
			EntityID:    entity.ID,
			Title:       fmt.Sprintf("%s — Premium 4K WEB-DL", highlight),
			Indexer:     "Velocity Index",
			Protocol:    "torrent",
			Quality:     "2160p WEB-DL",
			PublishDate: now.Add(-6 * time.Hour),
			SizeBytes:   int64(58 << 30),
			Seeders:     142,
			Grabs:       87,
			Score:       92.4,
			Status:      ManualReleaseStatusAvailable,
		},
		{
			ID:          uuid.NewString(),
			SearchID:    searchID,
			EntityID:    entity.ID,
			Title:       fmt.Sprintf("%s — 1080p WEBRip", title),
			Indexer:     "Scene Rush",
			Protocol:    "torrent",
			Quality:     "1080p WEBRip",
			PublishDate: now.Add(-26 * time.Hour),
			SizeBytes:   int64(8 << 30),
			Seeders:     36,
			Grabs:       21,
			Score:       84.1,
			Status:      ManualReleaseStatusAvailable,
		},
		{
			ID:              uuid.NewString(),
			SearchID:        searchID,
			EntityID:        entity.ID,
			Title:           fmt.Sprintf("%s — CAM Leak", title),
			Indexer:         "Sketchy Releases",
			Protocol:        "torrent",
			Quality:         "CAM",
			PublishDate:     now.Add(-2 * time.Hour),
			SizeBytes:       int64(2 << 30),
			Seeders:         5,
			Grabs:           2,
			Score:           9.5,
			Status:          ManualReleaseStatusRejected,
			RejectionReason: "Blocked by quality profile",
		},
	}

	for idx := range releases {
		releases[idx].DownloadURL = fmt.Sprintf("https://example.invalid/releases/%s", releases[idx].ID)
	}

	return releases
}

func (s *Service) updateManualSearchReleaseLocked(release ManualSearchRelease) {
	search, ok := s.manualSearches[release.EntityID]
	if !ok {
		return
	}
	updated := false
	for idx := range search.Results {
		if search.Results[idx].ID == release.ID {
			search.Results[idx] = release
			updated = true
			break
		}
	}
	if updated {
		s.manualSearches[release.EntityID] = search
	}
}

func cloneManualSearchResult(result ManualSearchResult) ManualSearchResult {
	copyResult := ManualSearchResult{
		ID:         result.ID,
		EntityID:   result.EntityID,
		Query:      result.Query,
		ExecutedAt: result.ExecutedAt,
		Results:    make([]ManualSearchRelease, len(result.Results)),
	}
	copy(copyResult.Results, result.Results)
	return copyResult
}

func (s *Service) lookupEntity(ctx context.Context, id string) (Entity, bool, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return Entity{}, false, nil
	}
	entity, found, err := s.repository.GetEntity(ctx, trimmedID)
	if err != nil {
		return Entity{}, false, fmt.Errorf("library: load entity %q: %w", trimmedID, err)
	}
	return entity, found, nil
}

func (s *Service) entityExists(ctx context.Context, id string) (bool, error) {
	_, found, err := s.lookupEntity(ctx, id)
	if err != nil {
		return false, err
	}
	return found, nil
}

func (s *Service) entityType(ctx context.Context, id string) (string, error) {
	entity, found, err := s.lookupEntity(ctx, id)
	if err != nil {
		return "", err
	}
	if !found {
		return "", nil
	}
	return entity.EntityType, nil
}
