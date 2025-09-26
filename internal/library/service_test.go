package library

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type stubClock struct{ now time.Time }

func (s stubClock) Now() time.Time { return s.now }

type capturingAuditSink struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (c *capturingAuditSink) Publish(_ context.Context, event AuditEvent) error {
	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
	return nil
}

func (c *capturingAuditSink) Events() []AuditEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]AuditEvent, len(c.events))
	copy(out, c.events)
	return out
}

type stubQuotaEvaluator struct {
	mu        sync.Mutex
	count     int
	result    QuotaEvaluationResult
	err       error
	requestID []string
}

func (s *stubQuotaEvaluator) EvaluateQuota(_ context.Context, request Request) (QuotaEvaluationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	s.requestID = append(s.requestID, request.ID)
	if s.err != nil {
		return QuotaEvaluationResult{}, s.err
	}
	return s.result, nil
}

func (s *stubQuotaEvaluator) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

type stubAutomationPreviewer struct {
	mu        sync.Mutex
	count     int
	result    AutomationPreviewResult
	err       error
	requestID []string
}

func (s *stubAutomationPreviewer) PreviewAutomation(_ context.Context, request Request) (AutomationPreviewResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	s.requestID = append(s.requestID, request.ID)
	if s.err != nil {
		return AutomationPreviewResult{}, s.err
	}
	return s.result, nil
}

func (s *stubAutomationPreviewer) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

func newRedisService(t testing.TB, cfg Config) *Service {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cfg.RedisClient = client

	svc, err := NewService(cfg)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
	})

	return svc
}

func TestListEntitiesFiltersByType(t *testing.T) {
	svc, err := NewService(Config{
		Entities: []Entity{{ID: "1", EntityType: "series", Title: "Series"}, {ID: "2", EntityType: "movie", Title: "Movie"}},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

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
	svc := newRedisService(t, Config{
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
	if job.Attempt != 0 {
		t.Fatalf("expected initial attempt count 0, got %d", job.Attempt)
	}
	if !job.CompletedAt.IsZero() {
		t.Fatalf("expected zero completion timestamp for new job")
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if !jobs[0].CompletedAt.IsZero() {
		t.Fatalf("expected stored job completion timestamp to be zero")
	}
}

func TestTriggerJobRejectsUnknownEntities(t *testing.T) {
	svc := newRedisService(t, Config{Entities: []Entity{{ID: "entity-1", EntityType: "movie"}}})

	if _, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "refresh", TargetID: "missing"}); !errors.Is(err, ErrUnknownEntity) {
		t.Fatalf("expected ErrUnknownEntity, got %v", err)
	}
}

func TestTriggerJobValidatesInput(t *testing.T) {
	svc := newRedisService(t, Config{Entities: []Entity{{ID: "entity-1", EntityType: "movie"}}})

	if _, err := svc.TriggerJob(context.Background(), TriggerJobInput{TargetID: "entity-1"}); !errors.Is(err, ErrInvalidJobType) {
		t.Fatalf("expected ErrInvalidJobType, got %v", err)
	}
	if _, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "refresh"}); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("expected ErrInvalidTarget, got %v", err)
	}
}

func TestTriggerJobEnforcesIdempotency(t *testing.T) {
	svc := newRedisService(t, Config{Entities: []Entity{{ID: "entity-1", EntityType: "series"}}})

	input := TriggerJobInput{JobType: "refresh", TargetID: "entity-1", IdempotencyKey: "refresh-entity-1"}
	if _, err := svc.TriggerJob(context.Background(), input); err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}
	if _, err := svc.TriggerJob(context.Background(), input); !errors.Is(err, ErrDuplicateJob) {
		t.Fatalf("expected ErrDuplicateJob, got %v", err)
	}
}

func TestSubmitRequestPersistsAndReturnsRecord(t *testing.T) {
	now := time.Date(2025, 1, 12, 9, 45, 0, 0, time.UTC)
	idSeq := 0
	svc, err := NewService(Config{
		Clock: stubClock{now: now},
		IDGenerator: func() string {
			idSeq++
			return fmt.Sprintf("req-%d", idSeq)
		},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	request, err := svc.SubmitRequest(context.Background(), SubmitRequestInput{
		Kind:        "Series",
		Title:       "The Expanse",
		RequestedBy: "user@example.com",
		Notes:       "Please monitor 1080p releases",
		Policy:      "public",
		ExternalIDs: map[string]string{"TMDB": "63639"},
	})
	if err != nil {
		t.Fatalf("SubmitRequest returned error: %v", err)
	}
	if request.ID != "req-1" {
		t.Fatalf("unexpected request id %q", request.ID)
	}
	if request.Kind != "series" {
		t.Fatalf("expected normalized kind, got %q", request.Kind)
	}
	if request.Status != RequestStatusPending {
		t.Fatalf("expected pending status, got %q", request.Status)
	}
	if !request.SubmittedAt.Equal(now) {
		t.Fatalf("expected submission timestamp %v, got %v", now, request.SubmittedAt)
	}
	if request.ExternalIDs["tmdb"] != "63639" {
		t.Fatalf("expected normalized external id, got %v", request.ExternalIDs)
	}

	list, err := svc.ListRequests(context.Background())
	if err != nil {
		t.Fatalf("ListRequests returned error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 request, got %d", len(list))
	}
	if list[0].ID != request.ID {
		t.Fatalf("expected request id %q in list, got %q", request.ID, list[0].ID)
	}
	if len(list[0].Approvals) != 0 || len(list[0].Notifications) != 0 {
		t.Fatalf("expected list entry without timeline data")
	}
}

func TestListRequestsPageFiltersAndPagination(t *testing.T) {
	base := time.Date(2025, 2, 5, 12, 0, 0, 0, time.UTC)
	idSeq := 0
	svc, err := NewService(Config{
		Clock: stubClock{now: base},
		IDGenerator: func() string {
			idSeq++
			return fmt.Sprintf("req-%d", idSeq)
		},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	req1, err := svc.SubmitRequest(context.Background(), SubmitRequestInput{
		Kind:        "series",
		Title:       "The Expanse",
		RequestedBy: "user1@example.com",
		Policy:      "Public",
	})
	if err != nil {
		t.Fatalf("SubmitRequest returned error: %v", err)
	}

	svc.clock = stubClock{now: base.Add(1 * time.Minute)}
	req2, err := svc.SubmitRequest(context.Background(), SubmitRequestInput{
		Kind:        "movie",
		Title:       "Dune",
		RequestedBy: "user2@example.com",
		Policy:      "VIP",
	})
	if err != nil {
		t.Fatalf("SubmitRequest returned error: %v", err)
	}

	svc.clock = stubClock{now: base.Add(2 * time.Minute)}
	req3, err := svc.SubmitRequest(context.Background(), SubmitRequestInput{
		Kind:        "series",
		Title:       "Foundation",
		RequestedBy: "user3@example.com",
		Policy:      "Public",
	})
	if err != nil {
		t.Fatalf("SubmitRequest returned error: %v", err)
	}

	svc.clock = stubClock{now: base.Add(3 * time.Minute)}
	if _, err := svc.RecordRequestApproval(context.Background(), req2.ID, RecordRequestApprovalInput{
		Decision: RequestDecisionApproved,
		Actor:    "approver@example.com",
	}); err != nil {
		t.Fatalf("RecordRequestApproval returned error: %v", err)
	}

	result, err := svc.ListRequestsPage(context.Background(), ListRequestsPageInput{Status: "pending", Limit: 1})
	if err != nil {
		t.Fatalf("ListRequestsPage returned error: %v", err)
	}
	if len(result.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(result.Requests))
	}
	if result.Requests[0].ID != req3.ID {
		t.Fatalf("expected newest pending request %q, got %q", req3.ID, result.Requests[0].ID)
	}
	if result.NextCursor == "" {
		t.Fatalf("expected next cursor for additional results")
	}

	nextPage, err := svc.ListRequestsPage(context.Background(), ListRequestsPageInput{Status: "pending", Cursor: result.NextCursor, Limit: 5})
	if err != nil {
		t.Fatalf("ListRequestsPage returned error: %v", err)
	}
	if len(nextPage.Requests) != 1 {
		t.Fatalf("expected second pending request, got %d entries", len(nextPage.Requests))
	}
	if nextPage.Requests[0].ID != req1.ID {
		t.Fatalf("expected pending request %q, got %q", req1.ID, nextPage.Requests[0].ID)
	}
	if nextPage.NextCursor != "" {
		t.Fatalf("expected no further pages, got cursor %q", nextPage.NextCursor)
	}

	policyFiltered, err := svc.ListRequestsPage(context.Background(), ListRequestsPageInput{Policy: "PUBLIC", Limit: 10})
	if err != nil {
		t.Fatalf("ListRequestsPage returned error: %v", err)
	}
	if len(policyFiltered.Requests) != 2 {
		t.Fatalf("expected 2 requests for policy filter, got %d", len(policyFiltered.Requests))
	}
	if policyFiltered.Requests[0].ID != req3.ID || policyFiltered.Requests[1].ID != req1.ID {
		t.Fatalf("unexpected policy ordering: %#v", policyFiltered.Requests)
	}

	kindFiltered, err := svc.ListRequestsPage(context.Background(), ListRequestsPageInput{Kind: "MOVIE", Limit: 5})
	if err != nil {
		t.Fatalf("ListRequestsPage returned error: %v", err)
	}
	if len(kindFiltered.Requests) != 1 || kindFiltered.Requests[0].ID != req2.ID {
		t.Fatalf("expected only movie request %q, got %#v", req2.ID, kindFiltered.Requests)
	}

	if _, err := svc.ListRequestsPage(context.Background(), ListRequestsPageInput{Status: "unknown"}); !errors.Is(err, ErrInvalidRequestStatusFilter) {
		t.Fatalf("expected ErrInvalidRequestStatusFilter, got %v", err)
	}
	if _, err := svc.ListRequestsPage(context.Background(), ListRequestsPageInput{Cursor: "!!!"}); !errors.Is(err, ErrInvalidRequestCursor) {
		t.Fatalf("expected ErrInvalidRequestCursor, got %v", err)
	}
	if _, err := svc.ListRequestsPage(context.Background(), ListRequestsPageInput{Limit: -1}); !errors.Is(err, ErrInvalidRequestPageLimit) {
		t.Fatalf("expected ErrInvalidRequestPageLimit, got %v", err)
	}
}

func TestRecordRequestApprovalTransitionsStatus(t *testing.T) {
	now := time.Date(2025, 3, 2, 14, 0, 0, 0, time.UTC)
	idSeq := 0
	svc, err := NewService(Config{
		Clock: stubClock{now: now},
		IDGenerator: func() string {
			idSeq++
			return fmt.Sprintf("req-%d", idSeq)
		},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	request, err := svc.SubmitRequest(context.Background(), SubmitRequestInput{
		Kind:        "movie",
		Title:       "Dune",
		RequestedBy: "ops@example.com",
	})
	if err != nil {
		t.Fatalf("SubmitRequest returned error: %v", err)
	}

	approvalTime := now.Add(2 * time.Hour)
	svc.clock = stubClock{now: approvalTime}

	updated, err := svc.RecordRequestApproval(context.Background(), request.ID, RecordRequestApprovalInput{
		Decision: RequestDecisionApproved,
		Actor:    "approver@example.com",
		Notes:    "Within quota",
	})
	if err != nil {
		t.Fatalf("RecordRequestApproval returned error: %v", err)
	}
	if updated.Status != RequestStatusApproved {
		t.Fatalf("expected approved status, got %q", updated.Status)
	}
	if len(updated.Approvals) != 1 {
		t.Fatalf("expected 1 approval event, got %d", len(updated.Approvals))
	}
	event := updated.Approvals[0]
	if event.Actor != "approver@example.com" {
		t.Fatalf("unexpected actor %q", event.Actor)
	}
	if event.Decision != RequestDecisionApproved {
		t.Fatalf("unexpected decision %q", event.Decision)
	}
	if !event.DecidedAt.Equal(approvalTime) {
		t.Fatalf("expected decided at %v, got %v", approvalTime, event.DecidedAt)
	}
	if !updated.UpdatedAt.Equal(approvalTime) {
		t.Fatalf("expected updated timestamp %v, got %v", approvalTime, updated.UpdatedAt)
	}

	if _, err := svc.RecordRequestApproval(context.Background(), request.ID, RecordRequestApprovalInput{Decision: RequestDecisionApproved, Actor: "ops"}); !errors.Is(err, ErrRequestFinalized) {
		t.Fatalf("expected ErrRequestFinalized, got %v", err)
	}
}

func TestRecordRequestApprovalRunsPolicyHooks(t *testing.T) {
	remaining := 3
	quota := &stubQuotaEvaluator{result: QuotaEvaluationResult{Remaining: &remaining}}
	preview := &stubAutomationPreviewer{result: AutomationPreviewResult{Jobs: []AutomationPreviewJob{{JobType: "import", TargetID: "entity-42", Summary: "Import requested entity"}}}}

	svc := newRedisService(t, Config{
		Entities:            []Entity{{ID: "entity-42", EntityType: "series"}},
		QuotaEvaluator:      quota,
		AutomationPreviewer: preview,
	})

	request, err := svc.SubmitRequest(context.Background(), SubmitRequestInput{
		Kind:        "movie",
		Title:       "Dune",
		RequestedBy: "ops@example.com",
	})
	if err != nil {
		t.Fatalf("SubmitRequest returned error: %v", err)
	}

	updated, err := svc.RecordRequestApproval(context.Background(), request.ID, RecordRequestApprovalInput{
		Decision: RequestDecisionApproved,
		Actor:    "approver@example.com",
	})
	if err != nil {
		t.Fatalf("RecordRequestApproval returned error: %v", err)
	}

	if quota.calls() != 1 {
		t.Fatalf("expected quota evaluator to be invoked once, got %d", quota.calls())
	}
	if preview.calls() != 1 {
		t.Fatalf("expected automation previewer to be invoked once, got %d", preview.calls())
	}
	if updated.QuotaRemaining != remaining {
		t.Fatalf("expected quota remaining %d, got %d", remaining, updated.QuotaRemaining)
	}

	stored, found, err := svc.GetRequest(context.Background(), request.ID)
	if err != nil {
		t.Fatalf("GetRequest returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected request to be found")
	}
	if stored.QuotaRemaining != remaining {
		t.Fatalf("expected persisted quota remaining %d, got %d", remaining, stored.QuotaRemaining)
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 automation job, got %d", len(jobs))
	}
	job := jobs[0]
	if job.TargetID != "entity-42" {
		t.Fatalf("expected automation job target entity-42, got %s", job.TargetID)
	}
	if job.JobType != "import" {
		t.Fatalf("expected automation job type import, got %s", job.JobType)
	}
}

func TestRecordRequestApprovalFansOutAutomationJobs(t *testing.T) {
	preview := &stubAutomationPreviewer{result: AutomationPreviewResult{Jobs: []AutomationPreviewJob{{JobType: "import", TargetID: "entity-42"}, {JobType: "scan", TargetID: "entity-42"}}}}
	var mu sync.Mutex
	idSeq := 0
	svc := newRedisService(t, Config{
		Entities:            []Entity{{ID: "entity-42", EntityType: "series"}},
		AutomationPreviewer: preview,
		IDGenerator: func() string {
			mu.Lock()
			defer mu.Unlock()
			idSeq++
			return fmt.Sprintf("id-%d", idSeq)
		},
	})

	request, err := svc.SubmitRequest(context.Background(), SubmitRequestInput{
		Kind:        "series",
		Title:       "The Expanse",
		RequestedBy: "ops@example.com",
	})
	if err != nil {
		t.Fatalf("SubmitRequest returned error: %v", err)
	}

	updated, err := svc.RecordRequestApproval(context.Background(), request.ID, RecordRequestApprovalInput{
		Decision: RequestDecisionApproved,
		Actor:    "approver@example.com",
	})
	if err != nil {
		t.Fatalf("RecordRequestApproval returned error: %v", err)
	}
	if updated.Status != RequestStatusApproved {
		t.Fatalf("expected approved status, got %s", updated.Status)
	}
	if preview.calls() != 1 {
		t.Fatalf("expected automation previewer to be invoked once, got %d", preview.calls())
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 automation jobs, got %d", len(jobs))
	}

	jobsByType := make(map[string]Job)
	for _, job := range jobs {
		jobsByType[job.JobType] = job
	}

	importJob, ok := jobsByType["import"]
	if !ok {
		t.Fatalf("expected import job to be enqueued")
	}
	if importJob.TargetID != "entity-42" {
		t.Fatalf("expected import job target entity-42, got %s", importJob.TargetID)
	}
	expectedImportKey := fmt.Sprintf("request:%s:%s:%s", strings.ToLower(updated.ID), "import", "entity-42")
	if importJob.IdempotencyKey != expectedImportKey {
		t.Fatalf("expected import job idempotency key %q, got %q", expectedImportKey, importJob.IdempotencyKey)
	}

	scanJob, ok := jobsByType["scan"]
	if !ok {
		t.Fatalf("expected scan job to be enqueued")
	}
	if scanJob.TargetID != "entity-42" {
		t.Fatalf("expected scan job target entity-42, got %s", scanJob.TargetID)
	}
	expectedScanKey := fmt.Sprintf("request:%s:%s:%s", strings.ToLower(updated.ID), "scan", "entity-42")
	if scanJob.IdempotencyKey != expectedScanKey {
		t.Fatalf("expected scan job idempotency key %q, got %q", expectedScanKey, scanJob.IdempotencyKey)
	}
}

func TestRecordRequestApprovalSkipsPolicyHooksForNonApprovals(t *testing.T) {
	quota := &stubQuotaEvaluator{}
	preview := &stubAutomationPreviewer{}

	svc, err := NewService(Config{
		QuotaEvaluator:      quota,
		AutomationPreviewer: preview,
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	request, err := svc.SubmitRequest(context.Background(), SubmitRequestInput{
		Kind:        "series",
		RequestedBy: "ops@example.com",
	})
	if err != nil {
		t.Fatalf("SubmitRequest returned error: %v", err)
	}

	updated, err := svc.RecordRequestApproval(context.Background(), request.ID, RecordRequestApprovalInput{
		Decision: RequestDecisionDenied,
		Actor:    "approver@example.com",
	})
	if err != nil {
		t.Fatalf("RecordRequestApproval returned error: %v", err)
	}
	if updated.Status != RequestStatusDenied {
		t.Fatalf("expected denied status, got %s", updated.Status)
	}
	if quota.calls() != 0 {
		t.Fatalf("expected quota evaluator not to be called, got %d", quota.calls())
	}
	if preview.calls() != 0 {
		t.Fatalf("expected automation previewer not to be called, got %d", preview.calls())
	}
}

func TestRecordRequestApprovalFailsWhenQuotaHookErrors(t *testing.T) {
	evalErr := errors.New("quota exhausted")
	quota := &stubQuotaEvaluator{err: evalErr}

	svc, err := NewService(Config{QuotaEvaluator: quota})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	request, err := svc.SubmitRequest(context.Background(), SubmitRequestInput{
		Kind:        "movie",
		RequestedBy: "ops@example.com",
	})
	if err != nil {
		t.Fatalf("SubmitRequest returned error: %v", err)
	}

	if _, err := svc.RecordRequestApproval(context.Background(), request.ID, RecordRequestApprovalInput{Decision: RequestDecisionApproved, Actor: "approver@example.com"}); !errors.Is(err, evalErr) {
		t.Fatalf("expected quota error to propagate, got %v", err)
	}

	stored, found, err := svc.GetRequest(context.Background(), request.ID)
	if err != nil {
		t.Fatalf("GetRequest returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected request to be found")
	}
	if stored.Status != RequestStatusPending {
		t.Fatalf("expected request to remain pending, got %s", stored.Status)
	}
	if len(stored.Approvals) != 0 {
		t.Fatalf("expected no approval events to be recorded")
	}
}

func TestRecordRequestNotificationStoresTimeline(t *testing.T) {
	now := time.Date(2025, 4, 18, 10, 30, 0, 0, time.UTC)
	idSeq := 0
	svc, err := NewService(Config{
		Clock: stubClock{now: now},
		IDGenerator: func() string {
			idSeq++
			return fmt.Sprintf("req-%d", idSeq)
		},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	request, err := svc.SubmitRequest(context.Background(), SubmitRequestInput{
		Kind:        "series",
		Title:       "Foundation",
		RequestedBy: "admin@example.com",
	})
	if err != nil {
		t.Fatalf("SubmitRequest returned error: %v", err)
	}

	notificationTime := now.Add(30 * time.Minute)
	svc.clock = stubClock{now: notificationTime}

	notification, err := svc.RecordRequestNotification(context.Background(), request.ID, RecordRequestNotificationInput{
		Channel: "Email",
		Target:  "admin@example.com",
		Status:  RequestNotificationStatusQueued,
	})
	if err != nil {
		t.Fatalf("RecordRequestNotification returned error: %v", err)
	}
	if notification.ID != "req-2" {
		t.Fatalf("expected notification id req-2, got %q", notification.ID)
	}
	if notification.Channel != "email" {
		t.Fatalf("expected normalized channel, got %q", notification.Channel)
	}
	if !notification.AttemptedAt.Equal(notificationTime) {
		t.Fatalf("expected attempted at %v, got %v", notificationTime, notification.AttemptedAt)
	}

	stored, found, err := svc.GetRequest(context.Background(), request.ID)
	if err != nil {
		t.Fatalf("GetRequest returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected request to be found")
	}
	if len(stored.Notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(stored.Notifications))
	}
	if stored.UpdatedAt != notification.AttemptedAt {
		t.Fatalf("expected updated timestamp %v, got %v", notification.AttemptedAt, stored.UpdatedAt)
	}
}

func TestRunManualSearchAndLatest(t *testing.T) {
	now := time.Date(2024, 7, 10, 15, 30, 0, 0, time.UTC)
	svc, err := NewService(Config{
		Entities: []Entity{{ID: "entity-1", EntityType: "series", Title: "Example Series"}},
		Clock:    stubClock{now: now},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	result, err := svc.RunManualSearch(context.Background(), "entity-1", "remux")
	if err != nil {
		t.Fatalf("RunManualSearch returned error: %v", err)
	}
	if result.ID == "" {
		t.Fatalf("expected search id to be set")
	}
	if result.EntityID != "entity-1" {
		t.Fatalf("unexpected entity id %s", result.EntityID)
	}
	if result.Query != "remux" {
		t.Fatalf("expected trimmed query, got %q", result.Query)
	}
	if !result.ExecutedAt.Equal(now) {
		t.Fatalf("expected executed timestamp to use clock, got %v", result.ExecutedAt)
	}
	if len(result.Results) < 2 {
		t.Fatalf("expected multiple releases, got %d", len(result.Results))
	}
	for _, release := range result.Results {
		if release.SearchID != result.ID {
			t.Fatalf("expected release %s to reference search %s", release.ID, result.ID)
		}
	}

	latest, found, err := svc.LatestManualSearch(context.Background(), "entity-1")
	if err != nil {
		t.Fatalf("LatestManualSearch returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected manual search to be present")
	}
	if latest.ID != result.ID {
		t.Fatalf("expected same search id, got %s", latest.ID)
	}

	// Running another search should replace previous release ids.
	next, err := svc.RunManualSearch(context.Background(), "entity-1", "")
	if err != nil {
		t.Fatalf("RunManualSearch second call error: %v", err)
	}
	if next.ID == result.ID {
		t.Fatalf("expected new search id to differ")
	}
	for _, release := range result.Results {
		if _, err := svc.ForceGrabRelease(context.Background(), "entity-1", release.ID); !errors.Is(err, ErrManualSearchReleaseNotFound) {
			t.Fatalf("expected prior release %s to be removed, got %v", release.ID, err)
		}
	}
}

func TestLatestManualSearchValidation(t *testing.T) {
	svc, err := NewService(Config{Entities: []Entity{{ID: "entity-1", EntityType: "movie"}}})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	if _, _, err := svc.LatestManualSearch(context.Background(), ""); !errors.Is(err, ErrManualSearchEntityRequired) {
		t.Fatalf("expected ErrManualSearchEntityRequired, got %v", err)
	}
	if _, _, err := svc.LatestManualSearch(context.Background(), "missing"); !errors.Is(err, ErrUnknownEntity) {
		t.Fatalf("expected ErrUnknownEntity, got %v", err)
	}

	result, found, err := svc.LatestManualSearch(context.Background(), "entity-1")
	if err != nil {
		t.Fatalf("LatestManualSearch returned error: %v", err)
	}
	if found {
		t.Fatalf("expected no manual search, got %+v", result)
	}
}

func TestForceGrabRelease(t *testing.T) {
	now := time.Date(2024, 7, 11, 9, 0, 0, 0, time.UTC)
	svc, err := NewService(Config{
		Entities: []Entity{{ID: "entity-1", EntityType: "series"}},
		Clock:    stubClock{now: now},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	search, err := svc.RunManualSearch(context.Background(), "entity-1", "4k")
	if err != nil {
		t.Fatalf("RunManualSearch returned error: %v", err)
	}
	release := search.Results[0]

	grabbed, err := svc.ForceGrabRelease(context.Background(), "entity-1", release.ID)
	if err != nil {
		t.Fatalf("ForceGrabRelease returned error: %v", err)
	}
	if grabbed.Status != ManualReleaseStatusGrabbed {
		t.Fatalf("expected status grabbed, got %s", grabbed.Status)
	}
	if grabbed.GrabbedAt.IsZero() {
		t.Fatalf("expected grabbed timestamp to be populated")
	}

	// Confirm the cached search reflects the updated release state.
	latest, found, err := svc.LatestManualSearch(context.Background(), "entity-1")
	if err != nil {
		t.Fatalf("LatestManualSearch returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected manual search to exist")
	}
	matched := false
	for _, candidate := range latest.Results {
		if candidate.ID == release.ID {
			matched = true
			if candidate.Status != ManualReleaseStatusGrabbed {
				t.Fatalf("expected cached status grabbed, got %s", candidate.Status)
			}
			break
		}
	}
	if !matched {
		t.Fatalf("expected release %s to remain in cache", release.ID)
	}

	if _, err := svc.ForceGrabRelease(context.Background(), "entity-1", release.ID); !errors.Is(err, ErrManualSearchReleaseGrabbed) {
		t.Fatalf("expected ErrManualSearchReleaseGrabbed, got %v", err)
	}
	if _, err := svc.ForceGrabRelease(context.Background(), "entity-1", "missing-release"); !errors.Is(err, ErrManualSearchReleaseNotFound) {
		t.Fatalf("expected ErrManualSearchReleaseNotFound, got %v", err)
	}
	if _, err := svc.ForceGrabRelease(context.Background(), "missing", release.ID); !errors.Is(err, ErrUnknownEntity) {
		t.Fatalf("expected ErrUnknownEntity for missing entity, got %v", err)
	}
	if _, err := svc.ForceGrabRelease(context.Background(), "entity-1", ""); !errors.Is(err, ErrManualSearchReleaseRequired) {
		t.Fatalf("expected ErrManualSearchReleaseRequired, got %v", err)
	}
}

func TestClaimAckFlowUpdatesState(t *testing.T) {
	svc := newRedisService(t, Config{Entities: []Entity{{ID: "entity-1", EntityType: "series"}}})

	job, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "scan", TargetID: "entity-1"})
	if err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}

	claimed, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs returned error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}
	if claimed[0].Job.ID != job.ID {
		t.Fatalf("unexpected job id %s", claimed[0].Job.ID)
	}
	if claimed[0].Job.State != JobStateRunning {
		t.Fatalf("expected running state, got %s", claimed[0].Job.State)
	}
	if claimed[0].Job.Attempt != 1 {
		t.Fatalf("expected attempt 1, got %d", claimed[0].Job.Attempt)
	}
	if claimed[0].Job.LastAttemptAt.IsZero() {
		t.Fatalf("expected last attempt timestamp to be set")
	}

	if err := svc.AckJob(context.Background(), claimed[0]); err != nil {
		t.Fatalf("AckJob returned error: %v", err)
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if jobs[0].State != JobStateDone {
		t.Fatalf("expected job to be completed, got %s", jobs[0].State)
	}
	if jobs[0].CompletedAt.IsZero() {
		t.Fatalf("expected job completion timestamp to be set")
	}
}

func TestRetryJobRequeuesWithIncrementedAttempt(t *testing.T) {
	svc := newRedisService(t, Config{Entities: []Entity{{ID: "entity-1", EntityType: "movie"}}})

	_, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "refresh", TargetID: "entity-1"})
	if err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}

	claimed, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs returned error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}

	retried, err := svc.RetryJob(context.Background(), claimed[0])
	if err != nil {
		t.Fatalf("RetryJob returned error: %v", err)
	}
	if retried.State != JobStateQueued {
		t.Fatalf("expected job to return to queued state, got %s", retried.State)
	}

	claimedAgain, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs (retry) returned error: %v", err)
	}
	if len(claimedAgain) != 1 {
		t.Fatalf("expected 1 claimed job after retry, got %d", len(claimedAgain))
	}
	if claimedAgain[0].Job.Attempt != 2 {
		t.Fatalf("expected attempt 2 after retry, got %d", claimedAgain[0].Job.Attempt)
	}
}

func TestRetryJobDeadLettersAfterMaxAttempts(t *testing.T) {
	svc := newRedisService(t, Config{
		Entities:            []Entity{{ID: "entity-1", EntityType: "movie"}},
		MaxDeliveryAttempts: 2,
	})

	job, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "refresh", TargetID: "entity-1"})
	if err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}

	claimed, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs returned error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}

	claimed[0].Job.ErrorMessage = "transient failure"
	retried, err := svc.RetryJob(context.Background(), claimed[0])
	if err != nil {
		t.Fatalf("RetryJob returned error on first retry: %v", err)
	}
	if retried.State != JobStateQueued {
		t.Fatalf("expected job to be queued after first retry, got %s", retried.State)
	}

	claimedAgain, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs after retry returned error: %v", err)
	}
	if len(claimedAgain) != 1 {
		t.Fatalf("expected 1 claimed job after retry, got %d", len(claimedAgain))
	}
	if claimedAgain[0].Job.Attempt != 2 {
		t.Fatalf("expected attempt 2 before dead-letter, got %d", claimedAgain[0].Job.Attempt)
	}

	claimedAgain[0].Job.ErrorMessage = "processing failed"
	dead, err := svc.RetryJob(context.Background(), claimedAgain[0])
	if err != nil {
		t.Fatalf("RetryJob returned error on dead-letter: %v", err)
	}
	if dead.State != JobStateFailed {
		t.Fatalf("expected failed state after dead-letter, got %s", dead.State)
	}
	if dead.CompletedAt.IsZero() {
		t.Fatalf("expected completion timestamp after dead-letter")
	}
	if dead.StreamID != "" {
		t.Fatalf("expected empty stream id after dead-letter, got %s", dead.StreamID)
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected single job, got %d", len(jobs))
	}
	if jobs[0].State != JobStateFailed {
		t.Fatalf("expected persisted job state failed, got %s", jobs[0].State)
	}
	if len(jobs[0].History) == 0 {
		t.Fatalf("expected job history entries, got none")
	}
	seenDeadLetter := false
	for _, entry := range jobs[0].History {
		if entry.State == JobStateFailed {
			seenDeadLetter = true
		}
	}
	if !seenDeadLetter {
		t.Fatalf("expected dead-letter history entry")
	}

	entries, err := svc.queue.client.XRange(context.Background(), svc.queue.deadLetter, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRANGE dead-letter returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 dead-letter entry, got %d", len(entries))
	}
	if reason, ok := entries[0].Values["failure_reason"].(string); !ok || reason != "processing failed" {
		t.Fatalf("expected failure reason 'processing failed', got %#v", entries[0].Values["failure_reason"])
	}
	if jobID, ok := entries[0].Values["job_id"].(string); !ok || jobID != job.ID {
		t.Fatalf("expected dead-letter job id %s, got %#v", job.ID, entries[0].Values["job_id"])
	}
}

func TestRotateQueueKeyspaceUpdatesStreamAndIdempotency(t *testing.T) {
	svc := newRedisService(t, Config{Entities: []Entity{{ID: "entity-1", EntityType: "movie"}}})

	if err := svc.RotateQueueKeyspace(context.Background(), "2025/04/01"); err != nil {
		t.Fatalf("RotateQueueKeyspace returned error: %v", err)
	}

	stream, group := svc.queue.currentStreamAndGroup()
	if stream != "arrgo:library:jobs:2025-04-01" {
		t.Fatalf("expected rotated stream name, got %s", stream)
	}
	if group != "arrgo-library-workers" {
		t.Fatalf("unexpected consumer group %s", group)
	}

	prefix, _ := svc.queue.currentIdempotency()
	if prefix != "arrgo:library:idemp:2025-04-01:" {
		t.Fatalf("expected rotated idempotency prefix, got %s", prefix)
	}

	_, err := svc.TriggerJob(context.Background(), TriggerJobInput{
		JobType:        "refresh",
		TargetID:       "entity-1",
		IdempotencyKey: "refresh-entity-1",
	})
	if err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}

	exists, err := svc.queue.client.Exists(context.Background(), "arrgo:library:idemp:2025-04-01:refresh-entity-1").Result()
	if err != nil {
		t.Fatalf("EXISTS returned error: %v", err)
	}
	if exists != 1 {
		t.Fatalf("expected idempotency key in rotated namespace, got %d", exists)
	}

	svc2 := newRedisService(t, Config{
		Entities:         []Entity{{ID: "entity-1", EntityType: "movie"}},
		StreamKeyVersion: "2025/04/01",
	})
	prefix2, _ := svc2.queue.currentIdempotency()
	if prefix2 != "arrgo:library:idemp:2025-04-01:" {
		t.Fatalf("expected configured idempotency prefix, got %s", prefix2)
	}
}

func TestJobHistoryLifecycle(t *testing.T) {
	now := time.Unix(1_700_000_100, 0)
	svc := newRedisService(t, Config{
		Entities: []Entity{{ID: "entity-1", EntityType: "movie"}},
		Clock:    stubClock{now: now},
		IDGenerator: func() string {
			return "job-history"
		},
	})

	_, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "scan", TargetID: "entity-1"})
	if err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}

	claimed, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs returned error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}

	if err := svc.AckJob(context.Background(), claimed[0]); err != nil {
		t.Fatalf("AckJob returned error: %v", err)
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected single job, got %d", len(jobs))
	}

	history := jobs[0].History
	if len(history) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(history))
	}

	states := make(map[string]int)
	for _, entry := range history {
		states[entry.State]++
		if entry.Attempt != 1 {
			t.Fatalf("expected attempt 1, got %d", entry.Attempt)
		}
	}
	for _, state := range []string{JobStateQueued, JobStateRunning, JobStateDone} {
		if states[state] == 0 {
			t.Fatalf("expected history entry for state %s", state)
		}
	}
}

func TestJobHistoryRecordsRetry(t *testing.T) {
	now := time.Unix(1_700_000_200, 0)
	svc := newRedisService(t, Config{
		Entities: []Entity{{ID: "entity-1", EntityType: "movie"}},
		Clock:    stubClock{now: now},
		IDGenerator: func() string {
			return "job-retry"
		},
	})

	_, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: "scan", TargetID: "entity-1"})
	if err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}

	claimed, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs returned error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}

	retried, err := svc.RetryJob(context.Background(), claimed[0])
	if err != nil {
		t.Fatalf("RetryJob returned error: %v", err)
	}
	if retried.State != JobStateQueued {
		t.Fatalf("expected queued state after retry, got %s", retried.State)
	}

	claimedAgain, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs after retry returned error: %v", err)
	}
	if len(claimedAgain) != 1 {
		t.Fatalf("expected 1 claimed job after retry, got %d", len(claimedAgain))
	}

	if err := svc.AckJob(context.Background(), claimedAgain[0]); err != nil {
		t.Fatalf("AckJob after retry returned error: %v", err)
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected single job, got %d", len(jobs))
	}

	history := jobs[0].History
	if len(history) < 5 {
		t.Fatalf("expected at least 5 history entries, got %d", len(history))
	}

	seen := make(map[string]bool)
	for _, entry := range history {
		key := fmt.Sprintf("%s-%d", entry.State, entry.Attempt)
		seen[key] = true
	}
	required := []string{
		fmt.Sprintf("%s-1", JobStateQueued),
		fmt.Sprintf("%s-1", JobStateRunning),
		fmt.Sprintf("%s-2", JobStateQueued),
		fmt.Sprintf("%s-2", JobStateRunning),
		fmt.Sprintf("%s-2", JobStateDone),
	}
	for _, key := range required {
		if !seen[key] {
			t.Fatalf("expected history entry %s", key)
		}
	}
}

func TestListJobsPagePaginates(t *testing.T) {
	repo := NewInMemoryRepository()
	now := time.Unix(1_700_000_300, 0)
	for i := 0; i < 3; i++ {
		jobID := fmt.Sprintf("job-%d", i)
		submitted := now.Add(-time.Duration(i) * time.Minute)
		if err := repo.SaveJob(context.Background(), Job{
			ID:          jobID,
			JobType:     "scan",
			TargetID:    "entity-1",
			State:       JobStateQueued,
			SubmittedAt: submitted,
		}); err != nil {
			t.Fatalf("SaveJob returned error: %v", err)
		}
		if err := repo.AppendJobHistory(context.Background(), jobID, JobHistoryRecord{
			Attempt:    1,
			State:      JobStateQueued,
			OccurredAt: submitted,
		}, JobHistoryRetention{MaxEntries: 10, MaxAge: time.Hour}); err != nil {
			t.Fatalf("AppendJobHistory returned error: %v", err)
		}
	}

	svc, err := NewService(Config{
		Repository: repo,
		Entities:   []Entity{{ID: "entity-1", EntityType: "movie"}},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	page1, err := svc.ListJobsPage(context.Background(), ListJobsPageInput{PageSize: 2, HistoryLimit: 1})
	if err != nil {
		t.Fatalf("ListJobsPage returned error: %v", err)
	}
	if len(page1.Jobs) != 2 {
		t.Fatalf("expected 2 jobs on first page, got %d", len(page1.Jobs))
	}
	if page1.NextPageToken == "" {
		t.Fatalf("expected next page token for remaining jobs")
	}
	for _, job := range page1.Jobs {
		if len(job.History) == 0 || len(job.History) > 1 {
			t.Fatalf("expected history limit 1, got %d", len(job.History))
		}
	}

	page2, err := svc.ListJobsPage(context.Background(), ListJobsPageInput{PageToken: page1.NextPageToken})
	if err != nil {
		t.Fatalf("ListJobsPage (second page) returned error: %v", err)
	}
	if len(page2.Jobs) != 1 {
		t.Fatalf("expected 1 job on second page, got %d", len(page2.Jobs))
	}
	if page2.NextPageToken != "" {
		t.Fatalf("expected empty token after final page")
	}

	if _, err := svc.ListJobsPage(context.Background(), ListJobsPageInput{PageToken: "invalid@@@"}); !errors.Is(err, ErrInvalidJobPageToken) {
		t.Fatalf("expected ErrInvalidJobPageToken, got %v", err)
	}
}

func TestAckJobPublishesAuditEventForImports(t *testing.T) {
	now := time.Date(2024, 7, 1, 15, 30, 0, 0, time.UTC)
	sink := &capturingAuditSink{}
	svc := newRedisService(t, Config{
		Entities:  []Entity{{ID: "entity-1", EntityType: "movie"}},
		Clock:     stubClock{now: now},
		AuditSink: sink,
	})

	_, err := svc.TriggerJob(context.Background(), TriggerJobInput{JobType: JobTypeImport, TargetID: "entity-1"})
	if err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}

	claimed, err := svc.ClaimJobs(context.Background(), "worker-1", 1, 0)
	if err != nil {
		t.Fatalf("ClaimJobs returned error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}

	if err := svc.AckJob(context.Background(), claimed[0]); err != nil {
		t.Fatalf("AckJob returned error: %v", err)
	}

	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	event := events[0]
	if event.EventType != AuditEventImportCompleted {
		t.Fatalf("unexpected event type %s", event.EventType)
	}
	if event.JobID != claimed[0].Job.ID {
		t.Fatalf("unexpected job id %s", event.JobID)
	}
	if event.TargetID != "entity-1" {
		t.Fatalf("unexpected target id %s", event.TargetID)
	}
	if event.EntityType != "movie" {
		t.Fatalf("unexpected entity type %s", event.EntityType)
	}
	if !event.CompletedAt.Equal(now) {
		t.Fatalf("expected completion timestamp %v, got %v", now, event.CompletedAt)
	}
}

func TestMetadataOverridesLifecycle(t *testing.T) {
	airDate := time.Date(2024, 9, 5, 12, 0, 0, 0, time.UTC)
	now := time.Date(2024, 9, 6, 8, 30, 0, 0, time.UTC)
	svc, err := NewService(Config{
		Entities: []Entity{{ID: "series-1", EntityType: "series"}},
		Clock:    stubClock{now: now},
	})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	input := MetadataOverrides{
		EpisodeCutovers: []EpisodeCutoverOverride{
			{EpisodeID: "ep-premiere", SeasonNumber: 2, EpisodeNumber: 1, AbsoluteNumber: 26, Title: "Premiere"},
			{EpisodeID: "ep-special", SeasonNumber: 0, EpisodeNumber: 1, Title: "Special", AirDate: &airDate},
		},
		AlbumEditions: []AlbumEditionOverride{
			{EditionID: "deluxe", Name: "Deluxe", Notes: "Bonus tracks"},
			{EditionID: "anniversary", Name: "Anniversary", IsDefault: true},
		},
		TrackNumberOverrides: []TrackNumberOverride{
			{TrackID: "track-b", DiscNumber: 2, TrackNumber: 2, TotalTracks: 12},
			{TrackID: "track-a", DiscNumber: 2, TrackNumber: 1, TotalTracks: 12, Notes: "Opener"},
		},
	}

	stored, err := svc.SetMetadataOverrides(context.Background(), "series-1", input)
	if err != nil {
		t.Fatalf("SetMetadataOverrides returned error: %v", err)
	}
	if !stored.UpdatedAt.Equal(now) {
		t.Fatalf("expected UpdatedAt to use clock, got %v", stored.UpdatedAt)
	}
	if len(stored.EpisodeCutovers) != 2 || stored.EpisodeCutovers[0].EpisodeID != "ep-special" {
		t.Fatalf("expected episode cutovers to be sorted, got %+v", stored.EpisodeCutovers)
	}
	if len(stored.TrackNumberOverrides) != 2 || stored.TrackNumberOverrides[0].TrackID != "track-a" {
		t.Fatalf("expected track overrides sorted, got %+v", stored.TrackNumberOverrides)
	}

	retrieved, found, err := svc.GetMetadataOverrides(context.Background(), "series-1")
	if err != nil {
		t.Fatalf("GetMetadataOverrides returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected overrides to be present")
	}
	if retrieved.EpisodeCutovers[0].Title != "Special" {
		t.Fatalf("expected retrieved cutover title to match input")
	}
	if retrieved.AlbumEditions[0].EditionID != "anniversary" {
		t.Fatalf("expected album editions to be sorted, got %+v", retrieved.AlbumEditions)
	}

	retrieved.EpisodeCutovers[0].Title = "Changed"
	again, ok, err := svc.GetMetadataOverrides(context.Background(), "series-1")
	if err != nil {
		t.Fatalf("GetMetadataOverrides second call error: %v", err)
	}
	if !ok {
		t.Fatalf("expected overrides to remain present")
	}
	if again.EpisodeCutovers[0].Title != "Special" {
		t.Fatalf("expected stored overrides to remain immutable, got %s", again.EpisodeCutovers[0].Title)
	}

	if err := svc.ClearMetadataOverrides(context.Background(), "series-1"); err != nil {
		t.Fatalf("ClearMetadataOverrides returned error: %v", err)
	}

	cleared, ok, err := svc.GetMetadataOverrides(context.Background(), "series-1")
	if err != nil {
		t.Fatalf("GetMetadataOverrides after clear error: %v", err)
	}
	if ok {
		t.Fatalf("expected overrides to be cleared, got %+v", cleared)
	}
}

func TestSetMetadataOverridesValidates(t *testing.T) {
	svc, err := NewService(Config{Entities: []Entity{{ID: "series-1", EntityType: "series"}}})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	cases := []struct {
		name     string
		entityID string
		payload  MetadataOverrides
		wantErr  error
	}{
		{
			name:    "empty entity",
			payload: MetadataOverrides{},
			wantErr: ErrMetadataOverrideEntityRequired,
		},
		{
			name:     "unknown entity",
			entityID: "missing",
			payload:  MetadataOverrides{},
			wantErr:  ErrUnknownEntity,
		},
		{
			name:     "missing episode id",
			entityID: "series-1",
			payload:  MetadataOverrides{EpisodeCutovers: []EpisodeCutoverOverride{{}}},
			wantErr:  ErrMetadataOverrideInvalidEpisode,
		},
		{
			name:     "missing edition id",
			entityID: "series-1",
			payload:  MetadataOverrides{AlbumEditions: []AlbumEditionOverride{{Name: "Deluxe"}}},
			wantErr:  ErrMetadataOverrideInvalidEdition,
		},
		{
			name:     "missing track id",
			entityID: "series-1",
			payload:  MetadataOverrides{TrackNumberOverrides: []TrackNumberOverride{{TrackNumber: 1}}},
			wantErr:  ErrMetadataOverrideInvalidTrack,
		},
		{
			name:     "invalid track number",
			entityID: "series-1",
			payload:  MetadataOverrides{TrackNumberOverrides: []TrackNumberOverride{{TrackID: "track-1", TrackNumber: 0}}},
			wantErr:  ErrMetadataOverrideInvalidTrackNum,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.SetMetadataOverrides(context.Background(), tc.entityID, tc.payload)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}
