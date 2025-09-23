package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/arrgo/arrgo/internal/library"
)

// Server exposes REST handlers for interacting with the library orchestration service.
type Server struct {
	service *library.Service
}

// NewServer wires a REST surface on top of the shared library service.
func NewServer(service *library.Service) *Server {
	return &Server{service: service}
}

// Routes returns the HTTP multiplexer hosting the API endpoints.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/library/entities", s.handleListEntities)
	mux.HandleFunc("/api/v1/jobs", s.handleJobs)
	return mux
}

func (s *Server) handleListEntities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	filter := library.ListEntitiesFilter{EntityType: r.URL.Query().Get("type")}
	entities, err := s.service.ListEntities(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list entities")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"entities": entities})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jobs, err := s.service.ListJobs(r.Context())
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to list jobs")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
	case http.MethodPost:
		var payload struct {
			JobType  string `json:"job_type"`
			TargetID string `json:"target_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		job, err := s.service.TriggerJob(r.Context(), library.TriggerJobInput{
			JobType:  payload.JobType,
			TargetID: payload.TargetID,
		})
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, library.ErrInvalidJobType), errors.Is(err, library.ErrInvalidTarget):
				status = http.StatusBadRequest
			case errors.Is(err, library.ErrUnknownEntity):
				status = http.StatusNotFound
			}
			respondError(w, status, err.Error())
			return
		}
		respondJSON(w, http.StatusCreated, map[string]any{"job": job})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	respondError(w, http.StatusMethodNotAllowed, "method not allowed")
}
