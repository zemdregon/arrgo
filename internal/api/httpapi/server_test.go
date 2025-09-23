package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arrgo/arrgo/internal/library"
)

func TestListEntitiesEndpoint(t *testing.T) {
	svc := library.NewService(library.Config{Entities: []library.Entity{{ID: "1", EntityType: "series", Title: "Series"}}})
	srv := NewServer(svc)

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
	svc := library.NewService(library.Config{
		Entities: []library.Entity{{ID: "entity-1", EntityType: "movie"}},
		Clock:    stubClock{now: time.Unix(1700000000, 0)},
		IDGenerator: func() string {
			return "job-http"
		},
	})
	srv := NewServer(svc)

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

type stubClock struct{ now time.Time }

func (s stubClock) Now() time.Time { return s.now }
