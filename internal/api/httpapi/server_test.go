package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/arrgo/arrgo/internal/auth"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/worker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
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
