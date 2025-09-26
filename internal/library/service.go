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

	"github.com/arrgo/arrgo/internal/observability/logging"
	"github.com/arrgo/arrgo/internal/timeutil"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Entity represents a top-level media record available to the automation stack.
type Entity struct {
	ID              string
	EntityType      string
	Title           string
	Synopsis        string
	PendingImports  int
	QualityUpgrades int
	Failures        int
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
	ErrorMessage   string

	History []JobHistoryRecord `json:"history,omitempty"`

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

const (
	defaultJobPageSize     = 25
	maxJobPageSize         = 100
	defaultJobHistoryLimit = 10
)

var jobHistoryRetentionPolicy = JobHistoryRetention{
	MaxEntries: 200,
	MaxAge:     30 * 24 * time.Hour,
}

// ListJobsPageInput captures pagination tuning for job listings.
type ListJobsPageInput struct {
	PageToken    string
	PageSize     int
	HistoryLimit int
}

// ListJobsPageResult wraps a page of jobs alongside the continuation token.
type ListJobsPageResult struct {
	Jobs          []Job
	NextPageToken string
}

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

// IDGenerator produces stable identifiers for new jobs.
type IDGenerator func() string

// SubmitRequestInput captures the metadata required to persist a new request.
type SubmitRequestInput struct {
	Kind           string
	Title          string
	RequestedBy    string
	Notes          string
	Policy         string
	ExternalIDs    map[string]string
	QuotaRemaining int
	SLADeadline    time.Time
}

// RecordRequestApprovalInput captures approval decisions applied to a request.
type RecordRequestApprovalInput struct {
	Decision  RequestDecision
	Actor     string
	Notes     string
	DecidedAt time.Time
}

// RecordRequestNotificationInput captures notification delivery attempts for a request.
type RecordRequestNotificationInput struct {
	Channel      string
	Target       string
	Status       RequestNotificationStatus
	AttemptedAt  time.Time
	ErrorMessage string
}

// Config drives Service construction.
type Config struct {
	Repository           Repository
	Entities             []Entity
	Jobs                 []Job
	Requests             []Request
	RequestApprovals     []RequestApprovalEvent
	RequestNotifications []RequestNotification
	Clock                timeutil.Clock
	IDGenerator          IDGenerator
	RedisClient          *redis.Client
	StreamName           string
	ConsumerGroup        string
	Logger               *slog.Logger
	IdempotencyTTL       time.Duration
	AuditSink            AuditSink
	Tracer               trace.Tracer
	RenameTemplates      RenameTemplates
	LibraryFiles         map[string][]LibraryFile
	MetadataOverrides    map[string]MetadataOverrides
}

// Errors returned by the Service APIs.
var (
	ErrInvalidJobType                     = errors.New("library: job type is required")
	ErrInvalidTarget                      = errors.New("library: target id is required")
	ErrUnknownEntity                      = errors.New("library: target entity not found")
	ErrQueueUnavailable                   = errors.New("library: orchestration queue unavailable")
	ErrDuplicateJob                       = errors.New("library: duplicate job for idempotency key")
	ErrManualSearchEntityRequired         = errors.New("library: manual search entity id is required")
	ErrManualSearchReleaseRequired        = errors.New("library: manual search release id is required")
	ErrManualSearchReleaseNotFound        = errors.New("library: manual search release not found")
	ErrManualSearchReleaseGrabbed         = errors.New("library: manual search release already grabbed")
	ErrInvalidJobPageToken                = errors.New("library: invalid job page token")
	ErrRequestKindRequired                = errors.New("library: request kind is required")
	ErrRequestRequesterRequired           = errors.New("library: requested by is required")
	ErrRequestIDRequired                  = errors.New("library: request id is required")
	ErrRequestNotFound                    = errors.New("library: request not found")
	ErrRequestFinalized                   = errors.New("library: request has already been finalized")
	ErrRequestDecisionRequired            = errors.New("library: request decision is required")
	ErrRequestActorRequired               = errors.New("library: approval actor is required")
	ErrRequestNotificationChannelRequired = errors.New("library: notification channel is required")
	ErrRequestNotificationTargetRequired  = errors.New("library: notification target is required")
)

// Service exposes read/write helpers that back both REST and gRPC front-ends.
type Service struct {
	mu              sync.RWMutex
	repository      Repository
	clock           timeutil.Clock
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
		svc.clock = timeutil.SystemClock{}
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
	for _, request := range cfg.Requests {
		seeded := cloneRequest(request)
		if strings.TrimSpace(seeded.ID) == "" {
			seeded.ID = svc.idGen()
		}
		if seeded.SubmittedAt.IsZero() {
			seeded.SubmittedAt = svc.clock.Now()
		}
		if seeded.UpdatedAt.IsZero() || seeded.UpdatedAt.Before(seeded.SubmittedAt) {
			seeded.UpdatedAt = seeded.SubmittedAt
		}
		if seeded.Status == "" {
			seeded.Status = RequestStatusPending
		}
		if err := svc.repository.SaveRequest(seedCtx, seeded); err != nil {
			return nil, fmt.Errorf("library: seed request %q: %w", seeded.ID, err)
		}
	}
	for _, approval := range cfg.RequestApprovals {
		seeded := cloneRequestApproval(approval)
		if strings.TrimSpace(seeded.ID) == "" {
			seeded.ID = svc.idGen()
		}
		if seeded.DecidedAt.IsZero() {
			seeded.DecidedAt = svc.clock.Now()
		}
		if err := svc.repository.AppendRequestApproval(seedCtx, seeded); err != nil {
			return nil, fmt.Errorf("library: seed request approval %q: %w", seeded.ID, err)
		}
	}
	for _, notification := range cfg.RequestNotifications {
		seeded := cloneRequestNotification(notification)
		if strings.TrimSpace(seeded.ID) == "" {
			seeded.ID = svc.idGen()
		}
		if seeded.AttemptedAt.IsZero() {
			seeded.AttemptedAt = svc.clock.Now()
		}
		if err := svc.repository.AppendRequestNotification(seedCtx, seeded); err != nil {
			return nil, fmt.Errorf("library: seed request notification %q: %w", seeded.ID, err)
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

	jobs, err := s.repository.ListJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("library: list jobs: %w", err)
	}
	statusByEntity := summarizeEntityStatuses(jobs)
	enriched := applyEntityStatuses(sorted, statusByEntity)

	wantsType := strings.TrimSpace(strings.ToLower(filter.EntityType))
	results := make([]Entity, 0, len(enriched))
	for _, entity := range enriched {
		if wantsType != "" && strings.ToLower(entity.EntityType) != wantsType {
			continue
		}
		results = append(results, entity)
	}
	return append([]Entity(nil), results...), nil
}

type entityStatusTotals struct {
	PendingImports  int
	QualityUpgrades int
	Failures        int
}

var canonicalCompletedState = strings.ToLower(JobStateDone)

func summarizeEntityStatuses(jobs []Job) map[string]entityStatusTotals {
	summary := make(map[string]entityStatusTotals)
	for _, job := range jobs {
		entityID := strings.TrimSpace(job.TargetID)
		if entityID == "" {
			continue
		}

		totals := summary[entityID]
		state := normalizeJobState(job.State)
		if isFailureState(state) {
			totals.Failures++
			summary[entityID] = totals
			continue
		}

		if state == canonicalCompletedState {
			summary[entityID] = totals
			continue
		}

		jobType := normalizeJobType(job.JobType)
		switch {
		case jobType == "import":
			totals.PendingImports++
		case jobType == "refresh", jobType == "scan", strings.Contains(jobType, "upgrade"), strings.Contains(jobType, "quality"):
			totals.QualityUpgrades++
		}
		summary[entityID] = totals
	}
	return summary
}

func applyEntityStatuses(entities []Entity, status map[string]entityStatusTotals) []Entity {
	enriched := make([]Entity, len(entities))
	for i, entity := range entities {
		totals := status[strings.TrimSpace(entity.ID)]
		entity.PendingImports = totals.PendingImports
		entity.QualityUpgrades = totals.QualityUpgrades
		entity.Failures = totals.Failures
		enriched[i] = entity
	}
	return enriched
}

func normalizeJobType(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}

func normalizeJobState(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}

func isFailureState(state string) bool {
	if state == "" {
		return false
	}
	switch state {
	case "failed", "failure", "errored", "error", "cancelled", "canceled", "aborted", "deadlettered", "timeout", "timed_out", "timed-out":
		return true
	}
	if strings.Contains(state, "fail") || strings.Contains(state, "error") || strings.Contains(state, "cancel") {
		return true
	}
	return false
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

	if len(jobs) > 0 {
		jobIDs := make([]string, 0, len(jobs))
		for _, job := range jobs {
			jobIDs = append(jobIDs, job.ID)
		}
		historyMap, histErr := s.repository.ListJobHistories(ctx, jobIDs, defaultJobHistoryLimit)
		if histErr != nil {
			return nil, fmt.Errorf("library: list job histories: %w", histErr)
		}
		for i := range jobs {
			if history := historyMap[jobs[i].ID]; len(history) > 0 {
				jobs[i].History = append([]JobHistoryRecord(nil), history...)
			} else {
				jobs[i].History = nil
			}
		}
	}

	span.SetAttributes(attribute.Int("library.job_count", len(jobs)))
	return jobs, nil
}

// ListJobsPage returns a paginated slice of jobs ordered by submission time (newest first).
func (s *Service) ListJobsPage(ctx context.Context, input ListJobsPageInput) (result ListJobsPageResult, err error) {
	pageSize := input.PageSize
	if pageSize <= 0 {
		pageSize = defaultJobPageSize
	}
	if pageSize > maxJobPageSize {
		pageSize = maxJobPageSize
	}

	historyLimit := input.HistoryLimit
	if historyLimit <= 0 {
		historyLimit = defaultJobHistoryLimit
	}
	if historyLimit > jobHistoryRetentionPolicy.MaxEntries {
		historyLimit = jobHistoryRetentionPolicy.MaxEntries
	}

	cursor, err := decodeJobCursor(input.PageToken)
	if err != nil {
		return ListJobsPageResult{}, fmt.Errorf("%w: %v", ErrInvalidJobPageToken, err)
	}

	ctx, span := s.startSpan(ctx, "library.Service.ListJobsPage",
		attribute.Int("library.page_size", pageSize),
		attribute.Int("library.history_limit", historyLimit),
	)
	defer endSpan(span, &err)

	repoResult, err := s.repository.ListJobsPage(ctx, ListJobsOptions{Limit: pageSize, Cursor: cursor})
	if err != nil {
		return ListJobsPageResult{}, fmt.Errorf("library: list jobs page: %w", err)
	}

	jobs := append([]Job(nil), repoResult.Jobs...)

	if len(jobs) > 0 && historyLimit > 0 {
		jobIDs := make([]string, 0, len(jobs))
		for _, job := range jobs {
			jobIDs = append(jobIDs, job.ID)
		}
		historyMap, histErr := s.repository.ListJobHistories(ctx, jobIDs, historyLimit)
		if histErr != nil {
			return ListJobsPageResult{}, fmt.Errorf("library: list job histories: %w", histErr)
		}
		for i := range jobs {
			if history := historyMap[jobs[i].ID]; len(history) > 0 {
				jobs[i].History = append([]JobHistoryRecord(nil), history...)
			} else {
				jobs[i].History = nil
			}
		}
	}

	result = ListJobsPageResult{
		Jobs:          jobs,
		NextPageToken: encodeJobCursor(repoResult.NextCursor),
	}
	span.SetAttributes(attribute.Int("library.job_count", len(jobs)))
	if result.NextPageToken != "" {
		span.SetAttributes(attribute.String("library.next_page_token", result.NextPageToken))
	}
	return result, nil
}

func (s *Service) recordJobHistory(ctx context.Context, jobID string, record JobHistoryRecord) {
	if strings.TrimSpace(jobID) == "" {
		return
	}
	if record.OccurredAt.IsZero() {
		record.OccurredAt = s.clock.Now()
	} else {
		record.OccurredAt = record.OccurredAt.UTC()
	}
	record.State = strings.TrimSpace(record.State)
	record.Message = strings.TrimSpace(record.Message)
	if err := s.repository.AppendJobHistory(ctx, jobID, record, jobHistoryRetentionPolicy); err != nil {
		logging.WithContext(ctx, s.logger).Warn("library: append job history failed",
			"job_id", jobID,
			"error", err,
		)
	}
}

// SubmitRequest persists a new intake request and returns the stored record.
func (s *Service) SubmitRequest(ctx context.Context, input SubmitRequestInput) (request Request, err error) {
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	if kind == "" {
		err = ErrRequestKindRequired
		return
	}
	requestedBy := strings.TrimSpace(input.RequestedBy)
	if requestedBy == "" {
		err = ErrRequestRequesterRequired
		return
	}

	ctx, span := s.startSpan(ctx, "library.Service.SubmitRequest",
		attribute.String("library.request_kind", kind),
		attribute.String("library.requested_by", requestedBy),
	)
	defer endSpan(span, &err)

	now := s.clock.Now()
	externalIDs := make(map[string]string, len(input.ExternalIDs))
	for key, value := range input.ExternalIDs {
		externalIDs[key] = value
	}

	request = Request{
		ID:             s.idGen(),
		Kind:           kind,
		Title:          strings.TrimSpace(input.Title),
		RequestedBy:    requestedBy,
		Notes:          strings.TrimSpace(input.Notes),
		Policy:         strings.TrimSpace(input.Policy),
		ExternalIDs:    externalIDs,
		QuotaRemaining: input.QuotaRemaining,
		SubmittedAt:    now,
		UpdatedAt:      now,
		SLADeadline:    input.SLADeadline,
		Status:         RequestStatusPending,
	}

	if err = s.repository.SaveRequest(ctx, request); err != nil {
		err = fmt.Errorf("library: submit request: %w", err)
		return
	}

	stored, found, loadErr := s.loadRequest(ctx, request.ID)
	if loadErr != nil {
		err = loadErr
		return
	}
	if !found {
		err = fmt.Errorf("library: request %q not found after creation", request.ID)
		return
	}
	request = stored
	span.SetAttributes(
		attribute.String("library.request_id", request.ID),
		attribute.String("library.request_status", string(request.Status)),
	)
	return
}

// ListRequests returns persisted requests ordered by submission time.
func (s *Service) ListRequests(ctx context.Context) (requests []Request, err error) {
	ctx, span := s.startSpan(ctx, "library.Service.ListRequests")
	defer endSpan(span, &err)

	records, err := s.repository.ListRequests(ctx)
	if err != nil {
		return nil, fmt.Errorf("library: list requests: %w", err)
	}

	requests = make([]Request, len(records))
	for i := range records {
		cloned := cloneRequest(records[i])
		cloned.Approvals = nil
		cloned.Notifications = nil
		requests[i] = cloned
	}
	span.SetAttributes(attribute.Int("library.request_count", len(requests)))
	return requests, nil
}

// GetRequest returns a single request along with approval and notification timeline.
func (s *Service) GetRequest(ctx context.Context, id string) (request Request, found bool, err error) {
	trimmedID := strings.TrimSpace(id)
	ctx, span := s.startSpan(ctx, "library.Service.GetRequest",
		attribute.String("library.request_id", trimmedID),
	)
	defer endSpan(span, &err)

	if trimmedID == "" {
		return Request{}, false, nil
	}

	request, found, err = s.loadRequest(ctx, trimmedID)
	if err != nil {
		return Request{}, false, err
	}
	if !found {
		return Request{}, false, nil
	}
	span.SetAttributes(attribute.String("library.request_status", string(request.Status)))
	return request, true, nil
}

// RecordRequestApproval applies an approval decision to a request and returns the updated record.
func (s *Service) RecordRequestApproval(ctx context.Context, requestID string, input RecordRequestApprovalInput) (request Request, err error) {
	trimmedID := strings.TrimSpace(requestID)
	if trimmedID == "" {
		err = ErrRequestIDRequired
		return
	}
	decision := normalizeRequestDecision(input.Decision)
	if decision == "" {
		err = ErrRequestDecisionRequired
		return
	}
	actor := strings.TrimSpace(input.Actor)
	if actor == "" {
		err = ErrRequestActorRequired
		return
	}

	ctx, span := s.startSpan(ctx, "library.Service.RecordRequestApproval",
		attribute.String("library.request_id", trimmedID),
		attribute.String("library.request_decision", string(decision)),
	)
	defer endSpan(span, &err)

	base, found, lookupErr := s.repository.GetRequest(ctx, trimmedID)
	if lookupErr != nil {
		err = fmt.Errorf("library: get request %q: %w", trimmedID, lookupErr)
		return
	}
	if !found {
		err = ErrRequestNotFound
		return
	}
	if isTerminalRequestStatus(normalizeRequestStatus(base.Status)) {
		err = ErrRequestFinalized
		return
	}

	decidedAt := input.DecidedAt
	if decidedAt.IsZero() {
		decidedAt = s.clock.Now()
	} else {
		decidedAt = decidedAt.UTC()
	}

	event := RequestApprovalEvent{
		ID:        s.idGen(),
		RequestID: trimmedID,
		Decision:  decision,
		Actor:     actor,
		Notes:     strings.TrimSpace(input.Notes),
		DecidedAt: decidedAt,
	}

	if appendErr := s.repository.AppendRequestApproval(ctx, event); appendErr != nil {
		err = fmt.Errorf("library: append request approval: %w", appendErr)
		return
	}

	base.Status = requestStatusFromDecision(decision)
	base.UpdatedAt = decidedAt
	if updateErr := s.repository.UpdateRequest(ctx, base); updateErr != nil {
		err = fmt.Errorf("library: update request %q: %w", trimmedID, updateErr)
		return
	}

	request, found, err = s.loadRequest(ctx, trimmedID)
	if err != nil {
		return Request{}, err
	}
	if !found {
		err = ErrRequestNotFound
		return
	}
	span.SetAttributes(attribute.String("library.request_status", string(request.Status)))
	return request, nil
}

// RecordRequestNotification stores a notification delivery attempt for a request.
func (s *Service) RecordRequestNotification(ctx context.Context, requestID string, input RecordRequestNotificationInput) (notification RequestNotification, err error) {
	trimmedID := strings.TrimSpace(requestID)
	if trimmedID == "" {
		err = ErrRequestIDRequired
		return
	}
	channel := strings.TrimSpace(input.Channel)
	if channel == "" {
		err = ErrRequestNotificationChannelRequired
		return
	}
	target := strings.TrimSpace(input.Target)
	if target == "" {
		err = ErrRequestNotificationTargetRequired
		return
	}

	ctx, span := s.startSpan(ctx, "library.Service.RecordRequestNotification",
		attribute.String("library.request_id", trimmedID),
		attribute.String("library.notification_channel", channel),
	)
	defer endSpan(span, &err)

	base, found, lookupErr := s.repository.GetRequest(ctx, trimmedID)
	if lookupErr != nil {
		err = fmt.Errorf("library: get request %q: %w", trimmedID, lookupErr)
		return
	}
	if !found {
		err = ErrRequestNotFound
		return
	}

	attemptedAt := input.AttemptedAt
	if attemptedAt.IsZero() {
		attemptedAt = s.clock.Now()
	} else {
		attemptedAt = attemptedAt.UTC()
	}

	notification = RequestNotification{
		ID:           s.idGen(),
		RequestID:    trimmedID,
		Channel:      strings.ToLower(channel),
		Target:       target,
		Status:       normalizeNotificationStatus(input.Status),
		AttemptedAt:  attemptedAt,
		ErrorMessage: strings.TrimSpace(input.ErrorMessage),
	}

	if appendErr := s.repository.AppendRequestNotification(ctx, notification); appendErr != nil {
		err = fmt.Errorf("library: append request notification: %w", appendErr)
		return
	}

	base.UpdatedAt = attemptedAt
	if updateErr := s.repository.UpdateRequest(ctx, base); updateErr != nil {
		err = fmt.Errorf("library: update request %q: %w", trimmedID, updateErr)
		return
	}

	span.SetAttributes(attribute.String("library.notification_status", string(notification.Status)))
	return notification, nil
}

func (s *Service) loadRequest(ctx context.Context, id string) (Request, bool, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return Request{}, false, nil
	}

	base, found, err := s.repository.GetRequest(ctx, trimmedID)
	if err != nil {
		return Request{}, false, fmt.Errorf("library: get request %q: %w", trimmedID, err)
	}
	if !found {
		return Request{}, false, nil
	}

	approvals, err := s.repository.ListRequestApprovals(ctx, trimmedID)
	if err != nil {
		return Request{}, false, fmt.Errorf("library: list request approvals: %w", err)
	}
	notifications, err := s.repository.ListRequestNotifications(ctx, trimmedID)
	if err != nil {
		return Request{}, false, fmt.Errorf("library: list request notifications: %w", err)
	}

	cloned := cloneRequest(base)
	cloned.Approvals = sortApprovals(approvals)
	cloned.Notifications = sortNotifications(notifications)
	return cloned, true, nil
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
	logging.WithContext(ctx, s.logger).Info("library manual search completed",
		"entity_id", trimmedID,
		"query", trimmedQuery,
		"results", len(releases),
	)

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
	logging.WithContext(ctx, s.logger).Info("library manual release grabbed",
		"entity_id", existing.EntityID,
		"release_id", existing.ID,
	)

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
	ctxWithJob := logging.ContextWithJobID(ctx, job.ID)
	var streamID string
	streamID, err = s.queue.Enqueue(ctxWithJob, job)
	if err != nil {
		job = Job{}
		return
	}
	job.StreamID = streamID
	if repoErr := s.repository.SaveJob(ctx, job); repoErr != nil {
		if delErr := s.queue.Delete(ctxWithJob, streamID); delErr != nil {
			logging.WithContext(ctxWithJob, s.logger).Warn("library: rollback queued job failed", "stream_id", streamID, "error", delErr)
		}
		err = fmt.Errorf("library: persist job: %w", repoErr)
		job = Job{}
		return
	}

	s.recordJobHistory(ctxWithJob, job.ID, JobHistoryRecord{
		Attempt:    job.Attempt + 1,
		State:      JobStateQueued,
		Message:    "queued",
		OccurredAt: job.SubmittedAt,
	})

	span.SetAttributes(
		attribute.String("library.job_id", job.ID),
		attribute.String("library.stream_id", streamID),
	)
	logging.WithContext(ctxWithJob, s.logger).Info("library job queued",
		"job_type", job.JobType,
		"target_id", job.TargetID,
		"idempotency_key", job.IdempotencyKey,
		"stream_id", streamID,
	)

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
		jobCtx := logging.ContextWithJobID(ctx, job.Job.ID)
		logging.WithContext(jobCtx, s.logger).Info("library job claimed",
			"stream_id", job.StreamID,
			"consumer", consumer,
			"attempt", job.Job.Attempt,
		)
		s.recordJobHistory(jobCtx, job.Job.ID, JobHistoryRecord{
			Attempt:    job.Job.Attempt,
			State:      JobStateRunning,
			Message:    fmt.Sprintf("claimed by %s", strings.TrimSpace(consumer)),
			OccurredAt: job.Job.LastAttemptAt,
		})
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

	jobCtx := logging.ContextWithJobID(ctx, job.Job.ID)
	if err = s.queue.Ack(jobCtx, job.StreamID); err != nil {
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
		logging.WithContext(jobCtx, s.logger).Warn("library: failed to load entity for audit",
			"target_id", job.Job.TargetID,
			"error", typeErr,
		)
		entityType = ""
	}

	span.SetAttributes(attribute.String("library.entity_type", entityType))
	logging.WithContext(jobCtx, s.logger).Info("library job acknowledged",
		"stream_id", job.StreamID,
		"attempt", job.Job.Attempt,
	)
	s.recordJobHistory(jobCtx, job.Job.ID, JobHistoryRecord{
		Attempt:    job.Job.Attempt,
		State:      JobStateDone,
		Message:    "completed",
		OccurredAt: completedAt,
	})

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
			logging.WithContext(jobCtx, s.logger).Warn("library: audit sink publish failed", "error", err)
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

	jobCtx := logging.ContextWithJobID(ctx, job.Job.ID)
	var streamID string
	streamID, err = s.queue.Retry(jobCtx, job.StreamID, job.Job)
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
	logging.WithContext(jobCtx, s.logger).Info("library job retried",
		"previous_stream_id", job.StreamID,
		"stream_id", streamID,
		"attempt", job.Job.Attempt+1,
	)

	s.recordJobHistory(jobCtx, updated.ID, JobHistoryRecord{
		Attempt: job.Job.Attempt + 1,
		State:   JobStateQueued,
		Message: "retried",
	})

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
