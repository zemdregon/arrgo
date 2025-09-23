package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/worker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

func TestListEntitiesEndpoint(t *testing.T) {
	svc := mustLibraryService(t, library.Config{Entities: []library.Entity{{ID: "1", EntityType: "series", Title: "Series"}}})
	srv := NewServer(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/library/entities", nil)
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

func TestTriggerJobEndpoint(t *testing.T) {
	svc := mustRedisBackedService(t, library.Config{
		Entities: []library.Entity{{ID: "entity-1", EntityType: "movie"}},
		Clock:    stubClock{now: time.Unix(1700000000, 0)},
		IDGenerator: func() string {
			return "job-http"
		},
	})
	srv := NewServer(svc, nil)

	body, _ := json.Marshal(map[string]string{"job_type": "refresh", "target_id": "entity-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewReader(body))
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

func TestWorkerRegistrationEndpoint(t *testing.T) {
	libSvc := mustLibraryService(t, library.Config{})
	workerSvc := mustWorkerService(t)
	srv := NewServer(libSvc, workerSvc)

	payload, _ := json.Marshal(map[string]any{
		"pool":         "transcode",
		"capabilities": []string{"gpu"},
		"endpoint":     "http://worker:9000",
		"version":      "1.2.3",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workers/register", bytes.NewReader(payload))
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
	srv := NewServer(libSvc, workerSvc)

	registerPayload, _ := json.Marshal(map[string]any{"pool": "default"})
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/workers/register", bytes.NewReader(registerPayload))
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
