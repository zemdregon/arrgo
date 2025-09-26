package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/arrgo/arrgo/internal/auth"
	"github.com/arrgo/arrgo/internal/library"
	observabilitymetrics "github.com/arrgo/arrgo/internal/observability/metrics"
	"github.com/arrgo/arrgo/internal/worker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestListEntitiesEndpoint(t *testing.T) {
	svc := mustLibraryService(t, library.Config{Entities: []library.Entity{{ID: "1", EntityType: "series", Title: "Series"}}})
	authSvc := mustAuthService(t)
	token := mustAPIKey(t, authSvc, "observer")
	srv := NewServer(svc, nil, authSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/library/entities", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()

	srv.handleListEntities(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}

	var payload struct {
		Entities []library.Entity `json:"entities"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if len(payload.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(payload.Entities))
	}
}

func TestAuthorizationFailures(t *testing.T) {
	svc := mustLibraryService(t, library.Config{Entities: []library.Entity{{ID: "1", EntityType: "series", Title: "Series"}}})
	authSvc := mustAuthService(t)
	srv := NewServer(svc, nil, authSvc)

	t.Run("missing token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/library/entities", nil)
		resp := httptest.NewRecorder()

		srv.handleListEntities(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for missing token, got %d", resp.Code)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		token := mustAPIKey(t, authSvc, "observer")
		body, _ := json.Marshal(map[string]string{"job_type": "refresh", "target_id": "1"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		resp := httptest.NewRecorder()

		srv.handleJobs(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for insufficient permission, got %d", resp.Code)
		}
	})
}

func TestTriggerJobEndpoint(t *testing.T) {
	svc := mustRedisBackedService(t, library.Config{
		Entities: []library.Entity{{ID: "entity-1", EntityType: "movie"}},
		Clock:    stubClock{now: time.Unix(1700000000, 0)},
		IDGenerator: func() string {
			return "job-http"
		},
	})
	authSvc := mustAuthService(t)
	token := mustAPIKey(t, authSvc, "admin")
	srv := NewServer(svc, nil, authSvc)

	body, _ := json.Marshal(map[string]string{"job_type": "refresh", "target_id": "entity-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()

	srv.handleJobs(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.Code)
	}

	var payload struct {
		Job library.Job `json:"job"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if payload.Job.ID != "job-http" {
		t.Fatalf("expected job-http id, got %s", payload.Job.ID)
	}
}

func TestListJobsEndpointPagination(t *testing.T) {
	repo := library.NewInMemoryRepository()
	now := time.Unix(1_700_000_500, 0)
	for i := 0; i < 3; i++ {
		jobID := fmt.Sprintf("job-%d", i)
		submitted := now.Add(-time.Duration(i) * time.Minute)
		if err := repo.SaveJob(context.Background(), library.Job{
			ID:          jobID,
			JobType:     "scan",
			TargetID:    "entity-1",
			State:       library.JobStateQueued,
			SubmittedAt: submitted,
		}); err != nil {
			t.Fatalf("SaveJob returned error: %v", err)
		}
		if err := repo.AppendJobHistory(context.Background(), jobID, library.JobHistoryRecord{
			Attempt:    1,
			State:      library.JobStateQueued,
			OccurredAt: submitted,
		}, library.JobHistoryRetention{MaxEntries: 5, MaxAge: time.Hour}); err != nil {
			t.Fatalf("AppendJobHistory returned error: %v", err)
		}
	}

	svc := mustLibraryService(t, library.Config{
		Repository: repo,
		Entities:   []library.Entity{{ID: "entity-1", EntityType: "movie"}},
	})
	authSvc := mustAuthService(t)
	token := mustAPIKey(t, authSvc, "observer")
	srv := NewServer(svc, nil, authSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?page_size=2&history_limit=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()

	srv.handleJobs(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}

	var payload struct {
		Jobs          []library.Job `json:"jobs"`
		NextPageToken string        `json:"next_page_token"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if len(payload.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(payload.Jobs))
	}
	if payload.NextPageToken == "" {
		t.Fatalf("expected next page token")
	}
	for _, job := range payload.Jobs {
		if len(job.History) == 0 || len(job.History) > 1 {
			t.Fatalf("expected history length 1, got %d", len(job.History))
		}
	}

	nextURL := "/api/v1/jobs?page_token=" + url.QueryEscape(payload.NextPageToken)
	req2 := httptest.NewRequest(http.MethodGet, nextURL, nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	resp2 := httptest.NewRecorder()
	srv.handleJobs(resp2, req2)
	if resp2.Code != http.StatusOK {
		t.Fatalf("expected status 200 on second page, got %d", resp2.Code)
	}

	var payload2 struct {
		Jobs          []library.Job `json:"jobs"`
		NextPageToken string        `json:"next_page_token"`
	}
	if err := json.Unmarshal(resp2.Body.Bytes(), &payload2); err != nil {
		t.Fatalf("failed to decode second payload: %v", err)
	}
	if len(payload2.Jobs) != 1 {
		t.Fatalf("expected 1 job on second page, got %d", len(payload2.Jobs))
	}
	if payload2.NextPageToken != "" {
		t.Fatalf("expected empty next page token on last page")
	}

	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?page_token=!!bad!!", nil)
	badReq.Header.Set("Authorization", "Bearer "+token)
	badResp := httptest.NewRecorder()
	srv.handleJobs(badResp, badReq)
	if badResp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid token, got %d", badResp.Code)
	}
}

func TestRequestEndpoints(t *testing.T) {
	var idSeq int
	svc := mustLibraryService(t, library.Config{
		Clock: stubClock{now: time.Unix(1_700_100_000, 0)},
		IDGenerator: func() string {
			idSeq++
			return fmt.Sprintf("req-%d", idSeq)
		},
	})
	authSvc := mustAuthService(t)
	adminToken := mustAPIKey(t, authSvc, "admin")
	observerToken := mustAPIKey(t, authSvc, "observer")
	srv := NewServer(svc, nil, authSvc)

	submitBody, _ := json.Marshal(map[string]any{
		"kind":            "series",
		"title":           "The Expanse",
		"requested_by":    "user@example.com",
		"notes":           "Please monitor 1080p releases",
		"quota_remaining": 5,
	})
	submitReq := httptest.NewRequest(http.MethodPost, "/api/v1/requests", bytes.NewReader(submitBody))
	submitReq.Header.Set("Authorization", "Bearer "+adminToken)
	submitResp := httptest.NewRecorder()

	srv.handleRequests(submitResp, submitReq)

	if submitResp.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", submitResp.Code, submitResp.Body.String())
	}

	var submitPayload struct {
		Request library.Request `json:"request"`
	}
	if err := json.Unmarshal(submitResp.Body.Bytes(), &submitPayload); err != nil {
		t.Fatalf("failed to decode submit payload: %v", err)
	}
	if submitPayload.Request.ID == "" {
		t.Fatalf("expected request id to be populated")
	}
	if submitPayload.Request.Status != library.RequestStatusPending {
		t.Fatalf("expected pending status, got %s", submitPayload.Request.Status)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/requests", nil)
	listReq.Header.Set("Authorization", "Bearer "+observerToken)
	listResp := httptest.NewRecorder()

	srv.handleRequests(listResp, listReq)

	if listResp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", listResp.Code, listResp.Body.String())
	}

	var listPayload struct {
		Requests   []library.Request `json:"requests"`
		NextCursor any               `json:"next_cursor"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("failed to decode list payload: %v", err)
	}
	if len(listPayload.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(listPayload.Requests))
	}

	approveBody, _ := json.Marshal(map[string]string{
		"approved_by": "ops@arrgo.dev",
		"notes":       "Fits within quota",
	})
	approveURL := fmt.Sprintf("/api/v1/requests/%s/approve", submitPayload.Request.ID)
	approveReq := httptest.NewRequest(http.MethodPost, approveURL, bytes.NewReader(approveBody))
	approveReq.Header.Set("Authorization", "Bearer "+adminToken)
	approveResp := httptest.NewRecorder()

	srv.handleRequestRoutes(approveResp, approveReq)

	if approveResp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", approveResp.Code, approveResp.Body.String())
	}

	var approvePayload struct {
		Request library.Request `json:"request"`
	}
	if err := json.Unmarshal(approveResp.Body.Bytes(), &approvePayload); err != nil {
		t.Fatalf("failed to decode approval payload: %v", err)
	}
	if approvePayload.Request.Status != library.RequestStatusApproved {
		t.Fatalf("expected approved status, got %s", approvePayload.Request.Status)
	}

	timelineURL := fmt.Sprintf("/api/v1/requests/%s/timeline", submitPayload.Request.ID)
	timelineReq := httptest.NewRequest(http.MethodGet, timelineURL, nil)
	timelineReq.Header.Set("Authorization", "Bearer "+observerToken)
	timelineResp := httptest.NewRecorder()

	srv.handleRequestRoutes(timelineResp, timelineReq)

	if timelineResp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", timelineResp.Code, timelineResp.Body.String())
	}

	var timelinePayload struct {
		Request       library.Request                `json:"request"`
		Approvals     []library.RequestApprovalEvent `json:"approvals"`
		Notifications []library.RequestNotification  `json:"notifications"`
	}
	if err := json.Unmarshal(timelineResp.Body.Bytes(), &timelinePayload); err != nil {
		t.Fatalf("failed to decode timeline payload: %v", err)
	}
	if len(timelinePayload.Approvals) != 1 {
		t.Fatalf("expected 1 approval event, got %d", len(timelinePayload.Approvals))
	}
	if timelinePayload.Approvals[0].Decision != library.RequestDecisionApproved {
		t.Fatalf("expected approval decision, got %s", timelinePayload.Approvals[0].Decision)
	}
	if timelinePayload.Request.Status != library.RequestStatusApproved {
		t.Fatalf("expected approved request status, got %s", timelinePayload.Request.Status)
	}
}

func TestRequestDecisionEndpointsCoverFlows(t *testing.T) {
	preview := &automationPreviewStub{jobs: []library.AutomationPreviewJob{{JobType: "import", TargetID: "entity-1"}}}
	var mu sync.Mutex
	idSeq := 0
	svc := mustRedisBackedService(t, library.Config{
		Entities:            []library.Entity{{ID: "entity-1", EntityType: "series"}},
		AutomationPreviewer: preview,
		IDGenerator: func() string {
			mu.Lock()
			defer mu.Unlock()
			idSeq++
			return fmt.Sprintf("id-%d", idSeq)
		},
	})
	authSvc := mustAuthService(t)
	adminToken := mustAPIKey(t, authSvc, "admin")
	srv := NewServer(svc, nil, authSvc)

	submitRequest := func(title string) string {
		body, _ := json.Marshal(map[string]any{
			"kind":         "series",
			"title":        title,
			"requested_by": "user@example.com",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp := httptest.NewRecorder()
		srv.handleRequests(resp, req)
		if resp.Code != http.StatusAccepted {
			t.Fatalf("expected submit status 202, got %d: %s", resp.Code, resp.Body.String())
		}
		var payload struct {
			Request library.Request `json:"request"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode submit payload: %v", err)
		}
		return payload.Request.ID
	}

	approvedID := submitRequest("Approve flow")

	approveBody, _ := json.Marshal(map[string]string{"approved_by": "ops@arrgo.dev"})
	approveReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/requests/%s/approve", approvedID), bytes.NewReader(approveBody))
	approveReq.Header.Set("Authorization", "Bearer "+adminToken)
	approveResp := httptest.NewRecorder()
	srv.handleRequestRoutes(approveResp, approveReq)
	if approveResp.Code != http.StatusOK {
		t.Fatalf("expected approve status 200, got %d: %s", approveResp.Code, approveResp.Body.String())
	}
	var approvePayload struct {
		Request library.Request `json:"request"`
	}
	if err := json.Unmarshal(approveResp.Body.Bytes(), &approvePayload); err != nil {
		t.Fatalf("failed to decode approve payload: %v", err)
	}
	if approvePayload.Request.Status != library.RequestStatusApproved {
		t.Fatalf("expected approved status, got %s", approvePayload.Request.Status)
	}

	jobs, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 automation job after approval, got %d", len(jobs))
	}
	job := jobs[0]
	if job.JobType != "import" {
		t.Fatalf("expected automation job type import, got %s", job.JobType)
	}
	if job.TargetID != "entity-1" {
		t.Fatalf("expected automation job target entity-1, got %s", job.TargetID)
	}
	expectedKey := fmt.Sprintf("request:%s:%s:%s", strings.ToLower(approvedID), "import", "entity-1")
	if job.IdempotencyKey != expectedKey {
		t.Fatalf("expected idempotency key %q, got %q", expectedKey, job.IdempotencyKey)
	}

	deniedID := submitRequest("Deny flow")
	denyBody, _ := json.Marshal(map[string]string{"denied_by": "ops@arrgo.dev"})
	denyReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/requests/%s/deny", deniedID), bytes.NewReader(denyBody))
	denyReq.Header.Set("Authorization", "Bearer "+adminToken)
	denyResp := httptest.NewRecorder()
	srv.handleRequestRoutes(denyResp, denyReq)
	if denyResp.Code != http.StatusOK {
		t.Fatalf("expected deny status 200, got %d: %s", denyResp.Code, denyResp.Body.String())
	}
	var denyPayload struct {
		Request library.Request `json:"request"`
	}
	if err := json.Unmarshal(denyResp.Body.Bytes(), &denyPayload); err != nil {
		t.Fatalf("failed to decode deny payload: %v", err)
	}
	if denyPayload.Request.Status != library.RequestStatusDenied {
		t.Fatalf("expected denied status, got %s", denyPayload.Request.Status)
	}

	jobsAfterDeny, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobsAfterDeny) != 1 {
		t.Fatalf("expected automation jobs to remain at 1 after denial, got %d", len(jobsAfterDeny))
	}

	cancelledID := submitRequest("Cancel flow")
	cancelBody, _ := json.Marshal(map[string]string{"cancelled_by": "user@example.com"})
	cancelReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/requests/%s/cancel", cancelledID), bytes.NewReader(cancelBody))
	cancelReq.Header.Set("Authorization", "Bearer "+adminToken)
	cancelResp := httptest.NewRecorder()
	srv.handleRequestRoutes(cancelResp, cancelReq)
	if cancelResp.Code != http.StatusOK {
		t.Fatalf("expected cancel status 200, got %d: %s", cancelResp.Code, cancelResp.Body.String())
	}
	var cancelPayload struct {
		Request library.Request `json:"request"`
	}
	if err := json.Unmarshal(cancelResp.Body.Bytes(), &cancelPayload); err != nil {
		t.Fatalf("failed to decode cancel payload: %v", err)
	}
	if cancelPayload.Request.Status != library.RequestStatusCancelled {
		t.Fatalf("expected cancelled status, got %s", cancelPayload.Request.Status)
	}

	jobsAfterCancel, err := svc.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(jobsAfterCancel) != 1 {
		t.Fatalf("expected automation jobs to remain at 1 after cancellation, got %d", len(jobsAfterCancel))
	}

	if preview.Calls() != 1 {
		t.Fatalf("expected automation preview to run once, got %d", preview.Calls())
	}
}

func TestManualSearchEndpoints(t *testing.T) {
	svc := mustLibraryService(t, library.Config{Entities: []library.Entity{{ID: "entity-1", EntityType: "series"}}, Clock: stubClock{now: time.Unix(1700001000, 0)}})
	authSvc := mustAuthService(t)
	readToken := mustAPIKey(t, authSvc, "observer")
	writeToken := mustAPIKey(t, authSvc, "admin")
	srv := NewServer(svc, nil, authSvc)

	// Initial fetch should return no cached search yet.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/indexer/search?entity_id=entity-1", nil)
	getReq.Header.Set("Authorization", "Bearer "+readToken)
	getResp := httptest.NewRecorder()
	srv.handleIndexerSearch(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected status 200 for initial fetch, got %d", getResp.Code)
	}
	var emptyPayload struct {
		Search *library.ManualSearchResult `json:"search"`
	}
	if err := json.Unmarshal(getResp.Body.Bytes(), &emptyPayload); err != nil {
		t.Fatalf("failed to decode empty payload: %v", err)
	}
	if emptyPayload.Search != nil {
		t.Fatalf("expected no cached search, got %+v", emptyPayload.Search)
	}

	// Run a manual search.
	searchBody, _ := json.Marshal(map[string]string{"entity_id": "entity-1", "query": "remux"})
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/indexer/search", bytes.NewReader(searchBody))
	postReq.Header.Set("Authorization", "Bearer "+writeToken)
	postResp := httptest.NewRecorder()
	srv.handleIndexerSearch(postResp, postReq)
	if postResp.Code != http.StatusOK {
		t.Fatalf("expected search status 200, got %d", postResp.Code)
	}
	var searchPayload struct {
		Search library.ManualSearchResult `json:"search"`
	}
	if err := json.Unmarshal(postResp.Body.Bytes(), &searchPayload); err != nil {
		t.Fatalf("failed to decode search payload: %v", err)
	}
	if searchPayload.Search.ID == "" {
		t.Fatalf("expected search id to be populated")
	}
	if len(searchPayload.Search.Results) == 0 {
		t.Fatalf("expected manual search results")
	}

	// Grab the first release.
	releaseID := searchPayload.Search.Results[0].ID
	grabBody, _ := json.Marshal(map[string]string{"entity_id": "entity-1", "release_id": releaseID})
	grabReq := httptest.NewRequest(http.MethodPost, "/api/v1/indexer/releases/grab", bytes.NewReader(grabBody))
	grabReq.Header.Set("Authorization", "Bearer "+writeToken)
	grabResp := httptest.NewRecorder()
	srv.handleIndexerGrab(grabResp, grabReq)
	if grabResp.Code != http.StatusOK {
		t.Fatalf("expected grab status 200, got %d", grabResp.Code)
	}
	var grabPayload struct {
		Release library.ManualSearchRelease `json:"release"`
	}
	if err := json.Unmarshal(grabResp.Body.Bytes(), &grabPayload); err != nil {
		t.Fatalf("failed to decode grab payload: %v", err)
	}
	if grabPayload.Release.Status != library.ManualReleaseStatusGrabbed {
		t.Fatalf("expected grabbed status, got %s", grabPayload.Release.Status)
	}

	// Subsequent grab should conflict.
	conflictReq := httptest.NewRequest(http.MethodPost, "/api/v1/indexer/releases/grab", bytes.NewReader(grabBody))
	conflictReq.Header.Set("Authorization", "Bearer "+writeToken)
	conflictResp := httptest.NewRecorder()
	srv.handleIndexerGrab(conflictResp, conflictReq)
	if conflictResp.Code != http.StatusConflict {
		t.Fatalf("expected conflict status, got %d", conflictResp.Code)
	}
}

func TestMetadataOverrideEndpoints(t *testing.T) {
	now := time.Unix(1700002000, 0)
	svc := mustLibraryService(t, library.Config{Entities: []library.Entity{{ID: "series-1", EntityType: "series"}}, Clock: stubClock{now: now}})
	authSvc := mustAuthService(t)
	readToken := mustAPIKey(t, authSvc, "observer")
	writeToken := mustAPIKey(t, authSvc, "admin")
	srv := NewServer(svc, nil, authSvc)

	basePath := "/api/v1/library/overrides/series-1"

	t.Run("initial fetch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, basePath, nil)
		req.Header.Set("Authorization", "Bearer "+readToken)
		resp := httptest.NewRecorder()

		srv.handleMetadataOverrides(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.Code)
		}

		var payload struct {
			Overrides *library.MetadataOverrides `json:"overrides"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if payload.Overrides != nil {
			t.Fatalf("expected no overrides yet, got %+v", payload.Overrides)
		}
	})

	requestBody := struct {
		EpisodeCutovers      []library.EpisodeCutoverOverride `json:"episode_cutovers"`
		AlbumEditions        []library.AlbumEditionOverride   `json:"album_editions"`
		TrackNumberOverrides []library.TrackNumberOverride    `json:"track_number_overrides"`
	}{
		EpisodeCutovers:      []library.EpisodeCutoverOverride{{EpisodeID: "ep-special", SeasonNumber: 0, EpisodeNumber: 1, Title: "Special"}},
		AlbumEditions:        []library.AlbumEditionOverride{{EditionID: "deluxe", Name: "Deluxe"}},
		TrackNumberOverrides: []library.TrackNumberOverride{{TrackID: "track-a", DiscNumber: 1, TrackNumber: 1}},
	}
	rawBody, _ := json.Marshal(requestBody)

	t.Run("update overrides", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, basePath, bytes.NewReader(rawBody))
		req.Header.Set("Authorization", "Bearer "+writeToken)
		resp := httptest.NewRecorder()

		srv.handleMetadataOverrides(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.Code)
		}

		var payload struct {
			Overrides library.MetadataOverrides `json:"overrides"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
		if len(payload.Overrides.EpisodeCutovers) != 1 {
			t.Fatalf("expected one episode override, got %+v", payload.Overrides.EpisodeCutovers)
		}
		if !payload.Overrides.UpdatedAt.Equal(now) {
			t.Fatalf("expected UpdatedAt %v, got %v", now, payload.Overrides.UpdatedAt)
		}
	})

	t.Run("fetch after update", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, basePath, nil)
		req.Header.Set("Authorization", "Bearer "+readToken)
		resp := httptest.NewRecorder()

		srv.handleMetadataOverrides(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.Code)
		}

		var payload struct {
			Overrides library.MetadataOverrides `json:"overrides"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
		if len(payload.Overrides.TrackNumberOverrides) != 1 {
			t.Fatalf("expected one track override, got %+v", payload.Overrides.TrackNumberOverrides)
		}
	})

	t.Run("delete overrides", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, basePath, nil)
		req.Header.Set("Authorization", "Bearer "+writeToken)
		resp := httptest.NewRecorder()

		srv.handleMetadataOverrides(resp, req)

		if resp.Code != http.StatusNoContent {
			t.Fatalf("expected status 204, got %d", resp.Code)
		}
	})

	t.Run("fetch after delete", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, basePath, nil)
		req.Header.Set("Authorization", "Bearer "+readToken)
		resp := httptest.NewRecorder()

		srv.handleMetadataOverrides(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.Code)
		}

		var payload struct {
			Overrides *library.MetadataOverrides `json:"overrides"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
		if payload.Overrides != nil {
			t.Fatalf("expected overrides to be cleared, got %+v", payload.Overrides)
		}
	})
}

func TestRenameHistoryEndpoint(t *testing.T) {
	now := time.Unix(1700003000, 0)
	svc := mustRedisBackedService(t, library.Config{
		Entities: []library.Entity{{ID: "series-1", EntityType: "series", Title: "History Show"}},
		Clock:    stubClock{now: now},
		LibraryFiles: map[string][]library.LibraryFile{
			"series-1": {{
				ID:            "file-1",
				EntityID:      "series-1",
				WorkType:      "episode",
				RelativePath:  "History Show/Season 01/History Show - S01E01 - Pilot.mkv",
				Extension:     "mkv",
				Quality:       "WEBDL-1080p",
				SeasonNumber:  1,
				EpisodeNumber: 1,
				EpisodeTitle:  "Pilot",
				SeriesTitle:   "History Show",
				ReleaseYear:   2023,
			}},
		},
	})
	if _, err := svc.StartRenameJob(context.Background(), "series-1", library.RenameOptions{}); err != nil {
		t.Fatalf("StartRenameJob returned error: %v", err)
	}
	if _, err := svc.StartRenameJob(context.Background(), "series-1", library.RenameOptions{}); err != nil {
		t.Fatalf("StartRenameJob second run returned error: %v", err)
	}

	authSvc := mustAuthService(t)
	token := mustAPIKey(t, authSvc, "observer")
	srv := NewServer(svc, nil, authSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/library/entities/series-1/rename-history?limit=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()

	srv.handleLibraryEntityRoutes(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}

	var payload struct {
		History []library.RenameJobRecord `json:"history"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode rename history payload: %v", err)
	}
	if len(payload.History) != 1 {
		t.Fatalf("expected history limit to apply, got %d entries", len(payload.History))
	}

	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/library/entities/series-1/rename-history?limit=oops", nil)
	badReq.Header.Set("Authorization", "Bearer "+token)
	badResp := httptest.NewRecorder()
	srv.handleLibraryEntityRoutes(badResp, badReq)
	if badResp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid limit, got %d", badResp.Code)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/library/entities/missing/rename-history", nil)
	missingReq.Header.Set("Authorization", "Bearer "+token)
	missingResp := httptest.NewRecorder()
	srv.handleLibraryEntityRoutes(missingResp, missingReq)
	if missingResp.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for missing entity, got %d", missingResp.Code)
	}
}

func TestWorkerRegistrationEndpoint(t *testing.T) {
	libSvc := mustLibraryService(t, library.Config{})
	workerSvc := mustWorkerService(t)
	authSvc := mustAuthService(t)
	token := mustAPIKey(t, authSvc, "admin")
	srv := NewServer(libSvc, workerSvc, authSvc)

	payload, _ := json.Marshal(map[string]any{
		"pool":         "transcode",
		"capabilities": []string{"gpu"},
		"endpoint":     "http://worker:9000",
		"version":      "1.2.3",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workers/register", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()

	srv.handleRegisterWorker(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.Code)
	}

	var body struct {
		Worker struct {
			ID   string `json:"id"`
			Pool string `json:"pool"`
		} `json:"worker"`
		Assignment struct {
			Stream string `json:"stream"`
		} `json:"assignment"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if body.Worker.ID == "" {
		t.Fatalf("expected worker id in response")
	}
	if body.Assignment.Stream == "" {
		t.Fatalf("expected assignment stream")
	}
}

func TestWorkerHeartbeatEndpoint(t *testing.T) {
	libSvc := mustLibraryService(t, library.Config{})
	workerSvc := mustWorkerService(t)
	authSvc := mustAuthService(t)
	token := mustAPIKey(t, authSvc, "admin")
	srv := NewServer(libSvc, workerSvc, authSvc)

	registerPayload, _ := json.Marshal(map[string]any{"pool": "default"})
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/workers/register", bytes.NewReader(registerPayload))
	registerReq.Header.Set("Authorization", "Bearer "+token)
	registerResp := httptest.NewRecorder()
	srv.handleRegisterWorker(registerResp, registerReq)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("expected registration 201, got %d", registerResp.Code)
	}
	var regBody struct {
		Worker struct {
			ID string `json:"id"`
		} `json:"worker"`
	}
	if err := json.Unmarshal(registerResp.Body.Bytes(), &regBody); err != nil {
		t.Fatalf("failed to decode registration payload: %v", err)
	}

	heartbeatPayload, _ := json.Marshal(map[string]any{"active_jobs": 2})
	heartbeatURL := "/api/v1/workers/" + regBody.Worker.ID + "/heartbeat"
	heartbeatReq := httptest.NewRequest(http.MethodPost, heartbeatURL, bytes.NewReader(heartbeatPayload))
	heartbeatReq.Header.Set("Authorization", "Bearer "+token)
	heartbeatResp := httptest.NewRecorder()

	srv.handleWorkerRoutes(heartbeatResp, heartbeatReq)

	if heartbeatResp.Code != http.StatusOK {
		t.Fatalf("expected heartbeat 200, got %d", heartbeatResp.Code)
	}

	var hbBody struct {
		Stale bool `json:"stale"`
	}
	if err := json.Unmarshal(heartbeatResp.Body.Bytes(), &hbBody); err != nil {
		t.Fatalf("failed to decode heartbeat payload: %v", err)
	}
	if hbBody.Stale {
		t.Fatalf("expected heartbeat to mark worker healthy")
	}
}

func TestListWorkersEndpoint(t *testing.T) {
	libSvc := mustLibraryService(t, library.Config{})
	workerSvc := mustWorkerService(t)
	authSvc := mustAuthService(t)
	token := mustAPIKey(t, authSvc, "observer")
	srv := NewServer(libSvc, workerSvc, authSvc)

	if _, err := workerSvc.RegisterWorker(context.Background(), worker.RegistrationRequest{Pool: "default"}); err != nil {
		t.Fatalf("RegisterWorker returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()

	srv.handleListWorkers(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}

	var payload struct {
		Workers []worker.Worker `json:"workers"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if len(payload.Workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(payload.Workers))
	}
}

func TestWorkerPoolEndpoints(t *testing.T) {
	libSvc := mustLibraryService(t, library.Config{})
	workerSvc := mustWorkerService(t)
	authSvc := mustAuthService(t)
	readToken := mustAPIKey(t, authSvc, "observer")
	manageToken := mustAPIKey(t, authSvc, "admin")
	srv := NewServer(libSvc, workerSvc, authSvc)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/worker-pools", nil)
	listReq.Header.Set("Authorization", "Bearer "+readToken)
	listResp := httptest.NewRecorder()

	srv.handleWorkerPools(listResp, listReq)

	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listResp.Code)
	}

	var listPayload struct {
		Pools []map[string]any `json:"pools"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("failed to decode list payload: %v", err)
	}
	if len(listPayload.Pools) == 0 {
		t.Fatalf("expected at least one pool configuration")
	}

	updateBody := map[string]any{
		"description":                "Transcode pool",
		"stream":                     "arrgo:library:transcode",
		"consumer_group":             "arrgo-transcode",
		"heartbeat_interval_seconds": 25,
		"stale_after_seconds":        70,
		"max_concurrent_jobs":        6,
	}
	rawUpdate, _ := json.Marshal(updateBody)
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/worker-pools/transcode", bytes.NewReader(rawUpdate))
	updateReq.Header.Set("Authorization", "Bearer "+manageToken)
	updateResp := httptest.NewRecorder()

	srv.handleWorkerPoolConfig(updateResp, updateReq)

	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update status 200, got %d", updateResp.Code)
	}

	var updatePayload struct {
		Pool struct {
			Pool                     string `json:"pool"`
			ConsumerGroup            string `json:"consumer_group"`
			HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds"`
			StaleAfterSeconds        int    `json:"stale_after_seconds"`
			MaxConcurrentJobs        int    `json:"max_concurrent_jobs"`
		} `json:"pool"`
	}
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updatePayload); err != nil {
		t.Fatalf("failed to decode update payload: %v", err)
	}
	if updatePayload.Pool.Pool != "transcode" {
		t.Fatalf("expected pool name transcode, got %s", updatePayload.Pool.Pool)
	}
	if updatePayload.Pool.ConsumerGroup != "arrgo-transcode" {
		t.Fatalf("expected consumer group override to persist")
	}
	if updatePayload.Pool.HeartbeatIntervalSeconds != 25 {
		t.Fatalf("expected heartbeat interval seconds to match override")
	}

	configs, err := workerSvc.ListPoolConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListPoolConfigs returned error: %v", err)
	}
	found := false
	for _, cfg := range configs {
		if cfg.Pool == "transcode" {
			found = true
			if cfg.ConsumerGroup != "arrgo-transcode" || cfg.HeartbeatInterval != 25*time.Second {
				t.Fatalf("unexpected persisted pool configuration: %#v", cfg)
			}
		}
	}
	if !found {
		t.Fatalf("expected transcode pool configuration to be stored")
	}
}

func TestObservabilityEndpoints(t *testing.T) {
	svc := mustLibraryService(t, library.Config{})
	srv := NewServer(svc, nil, nil)
	handler := srv.Routes()

	t.Run("metrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		resp := httptest.NewRecorder()

		handler.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected metrics status 200, got %d", resp.Code)
		}
		if resp.Body.Len() == 0 {
			t.Fatalf("expected metrics payload")
		}
	})

	t.Run("pprof", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
		resp := httptest.NewRecorder()

		handler.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected pprof status 200, got %d", resp.Code)
		}
		if !strings.Contains(resp.Body.String(), "profile") {
			t.Fatalf("expected pprof index output")
		}
	})
}

func TestHTTPMetricsRecorded(t *testing.T) {
	metricsOnce = sync.Once{}
	globalHTTPMetrics = nil

	originalProvider := otel.GetMeterProvider()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	otel.SetMeterProvider(provider)

	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetMeterProvider(originalProvider)
		metricsOnce = sync.Once{}
		globalHTTPMetrics = nil
	})

	srv := &Server{httpMetrics: newHTTPMetrics()}
	handler := srv.instrument("/metrics-test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics-test", nil)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected handler to preserve status code, got %d", resp.Code)
	}

	rm, err := observabilitymetrics.CollectOnce(context.Background(), reader)
	if err != nil {
		t.Fatalf("CollectOnce returned error: %v", err)
	}

	var requestMetricFound, durationMetricFound bool

	for _, scope := range rm.ScopeMetrics {
		for _, metricData := range scope.Metrics {
			switch data := metricData.Data.(type) {
			case metricdata.Sum[int64]:
				if metricData.Name != "arrgo_http_requests_total" {
					continue
				}
				requestMetricFound = true
				if len(data.DataPoints) != 1 {
					t.Fatalf("expected 1 datapoint for requests, got %d", len(data.DataPoints))
				}
				dp := data.DataPoints[0]
				if dp.Value != 1 {
					t.Fatalf("expected request counter to increment, got %d", dp.Value)
				}
				assertHTTPMetricAttributes(t, dp.Attributes)
			case metricdata.Histogram[float64]:
				if metricData.Name != "arrgo_http_request_duration_seconds" {
					continue
				}
				durationMetricFound = true
				if len(data.DataPoints) != 1 {
					t.Fatalf("expected 1 datapoint for duration, got %d", len(data.DataPoints))
				}
				dp := data.DataPoints[0]
				if dp.Count != 1 {
					t.Fatalf("expected histogram count to equal 1, got %d", dp.Count)
				}
				if dp.Sum <= 0 {
					t.Fatalf("expected positive duration sum, got %f", dp.Sum)
				}
				assertHTTPMetricAttributes(t, dp.Attributes)
			}
		}
	}

	if !requestMetricFound {
		t.Fatalf("arrgo_http_requests_total metric was not recorded")
	}
	if !durationMetricFound {
		t.Fatalf("arrgo_http_request_duration_seconds metric was not recorded")
	}
}

func assertHTTPMetricAttributes(t *testing.T, attrs attribute.Set) {
	t.Helper()

	values := map[string]string{}
	for _, kv := range attrs.ToSlice() {
		values[string(kv.Key)] = kv.Value.AsString()
	}

	if values["route"] != "/metrics-test" {
		t.Fatalf("expected route attribute to be /metrics-test, got %q", values["route"])
	}
	if values["method"] != http.MethodGet {
		t.Fatalf("expected method attribute to be GET, got %q", values["method"])
	}
	if values["status"] != "202" {
		t.Fatalf("expected status attribute to be 202, got %q", values["status"])
	}
}

type stubClock struct{ now time.Time }

func (s stubClock) Now() time.Time { return s.now }

func mustLibraryService(t *testing.T, cfg library.Config) *library.Service {
	t.Helper()

	svc, err := library.NewService(cfg)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	return svc
}

type automationPreviewStub struct {
	mu    sync.Mutex
	jobs  []library.AutomationPreviewJob
	count int
}

func (s *automationPreviewStub) PreviewAutomation(context.Context, library.Request) (library.AutomationPreviewResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := make([]library.AutomationPreviewJob, len(s.jobs))
	copy(jobs, s.jobs)
	s.count++
	return library.AutomationPreviewResult{Jobs: jobs}, nil
}

func (s *automationPreviewStub) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

func mustRedisBackedService(t *testing.T, cfg library.Config) *library.Service {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cfg.RedisClient = client

	svc, err := library.NewService(cfg)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
	})

	return svc
}

func mustWorkerService(t *testing.T) *worker.Service {
	t.Helper()

	repo := worker.NewInMemoryRepository()
	metrics := worker.NewMetrics(prometheus.NewRegistry())
	svc, err := worker.NewService(worker.Config{
		Repository: repo,
		Metrics:    metrics,
	})
	if err != nil {
		t.Fatalf("worker.NewService returned error: %v", err)
	}
	return svc
}

func mustAuthService(t *testing.T) *auth.Service {
	t.Helper()

	svc, err := auth.NewService(auth.Config{})
	if err != nil {
		t.Fatalf("auth.NewService returned error: %v", err)
	}
	return svc
}

func mustAPIKey(t *testing.T, svc *auth.Service, role string) string {
	t.Helper()

	material, err := svc.CreateAPIKey(context.Background(), "test-suite", role)
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	return material.Token
}
