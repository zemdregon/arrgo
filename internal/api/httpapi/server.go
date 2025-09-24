package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/arrgo/arrgo/internal/auth"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/worker"
)

// Server exposes REST handlers for interacting with the library and worker coordination services.
type Server struct {
	service       *library.Service
	workerService *worker.Service
	authorizer    Authorizer
}

// NewServer wires a REST surface on top of the shared library service.
func NewServer(service *library.Service, workerService *worker.Service, authorizer Authorizer) *Server {
	return &Server{service: service, workerService: workerService, authorizer: authorizer}
}

// Routes returns the HTTP multiplexer hosting the API endpoints.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/v1/library/entities", s.instrument("/api/v1/library/entities", http.HandlerFunc(s.handleListEntities)))
	mux.Handle("/api/v1/library/entities/", s.instrument("/api/v1/library/entities/{entity}", http.HandlerFunc(s.handleLibraryEntityRoutes)))
	mux.Handle("/api/v1/jobs", s.instrument("/api/v1/jobs", http.HandlerFunc(s.handleJobs)))
	mux.Handle("/api/v1/indexer/search", s.instrument("/api/v1/indexer/search", http.HandlerFunc(s.handleIndexerSearch)))
	mux.Handle("/api/v1/indexer/releases/grab", s.instrument("/api/v1/indexer/releases/grab", http.HandlerFunc(s.handleIndexerGrab)))
	mux.Handle("/api/v1/library/overrides/", s.instrument("/api/v1/library/overrides/{entity}", http.HandlerFunc(s.handleMetadataOverrides)))
	if s.workerService != nil {
		mux.Handle("/api/v1/workers", s.instrument("/api/v1/workers", http.HandlerFunc(s.handleListWorkers)))
		mux.Handle("/api/v1/workers/register", s.instrument("/api/v1/workers/register", http.HandlerFunc(s.handleRegisterWorker)))
		mux.Handle("/api/v1/workers/", s.instrument("/api/v1/workers/{id}/heartbeat", http.HandlerFunc(s.handleWorkerRoutes)))
		mux.Handle("/api/v1/worker-pools", s.instrument("/api/v1/worker-pools", http.HandlerFunc(s.handleWorkerPools)))
		mux.Handle("/api/v1/worker-pools/", s.instrument("/api/v1/worker-pools/{pool}", http.HandlerFunc(s.handleWorkerPoolConfig)))
	}
	mux.Handle("/metrics", promhttp.Handler())
	registerPprofHandlers(mux)
	return mux
}

func (s *Server) handleListEntities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if _, ok := s.requirePermission(w, r, auth.PermissionLibraryRead); !ok {
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

func (s *Server) handleLibraryEntityRoutes(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/library/entities/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		respondError(w, http.StatusNotFound, "library entity resource not found")
		return
	}

	parts := strings.Split(trimmed, "/")
	entityID := strings.TrimSpace(parts[0])
	if entityID == "" {
		respondError(w, http.StatusNotFound, "library entity resource not found")
		return
	}

	if len(parts) == 1 {
		respondError(w, http.StatusNotFound, "library entity resource not found")
		return
	}

	switch parts[1] {
	case "rename-history":
		s.handleRenameHistory(w, r, entityID)
	default:
		respondError(w, http.StatusNotFound, "library entity resource not found")
	}
}

func (s *Server) handleRenameHistory(w http.ResponseWriter, r *http.Request, entityID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if _, ok := s.requirePermission(w, r, auth.PermissionLibraryRead); !ok {
		return
	}

	limitParam := strings.TrimSpace(r.URL.Query().Get("limit"))
	limit := 0
	if limitParam != "" {
		parsed, err := strconv.Atoi(limitParam)
		if err != nil || parsed < 0 {
			respondError(w, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
		limit = parsed
	}

	history, err := s.service.RenameHistory(r.Context(), entityID, limit)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, library.ErrUnknownEntity):
			status = http.StatusNotFound
		}
		respondError(w, status, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"history": history})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, ok := s.requirePermission(w, r, auth.PermissionLibraryRead); !ok {
			return
		}
		jobs, err := s.service.ListJobs(r.Context())
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to list jobs")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
	case http.MethodPost:
		if _, ok := s.requirePermission(w, r, auth.PermissionLibraryWrite); !ok {
			return
		}
		var payload struct {
			JobType        string `json:"job_type"`
			TargetID       string `json:"target_id"`
			PayloadURI     string `json:"payload_uri"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		job, err := s.service.TriggerJob(r.Context(), library.TriggerJobInput{
			JobType:        payload.JobType,
			TargetID:       payload.TargetID,
			PayloadURI:     payload.PayloadURI,
			IdempotencyKey: payload.IdempotencyKey,
		})
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, library.ErrInvalidJobType), errors.Is(err, library.ErrInvalidTarget):
				status = http.StatusBadRequest
			case errors.Is(err, library.ErrUnknownEntity):
				status = http.StatusNotFound
			case errors.Is(err, library.ErrDuplicateJob):
				status = http.StatusConflict
			case errors.Is(err, library.ErrQueueUnavailable):
				status = http.StatusServiceUnavailable
			}
			respondError(w, status, err.Error())
			return
		}
		respondJSON(w, http.StatusCreated, map[string]any{"job": job})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleIndexerSearch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, ok := s.requirePermission(w, r, auth.PermissionLibraryRead); !ok {
			return
		}
		entityID := strings.TrimSpace(r.URL.Query().Get("entity_id"))
		if entityID == "" {
			respondError(w, http.StatusBadRequest, "entity_id is required")
			return
		}
		result, found, err := s.service.LatestManualSearch(r.Context(), entityID)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, library.ErrManualSearchEntityRequired):
				status = http.StatusBadRequest
			case errors.Is(err, library.ErrUnknownEntity):
				status = http.StatusNotFound
			}
			respondError(w, status, err.Error())
			return
		}
		if !found {
			respondJSON(w, http.StatusOK, map[string]any{"search": nil})
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"search": result})
	case http.MethodPost:
		if _, ok := s.requirePermission(w, r, auth.PermissionLibraryWrite); !ok {
			return
		}
		var payload struct {
			EntityID string `json:"entity_id"`
			Query    string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		result, err := s.service.RunManualSearch(r.Context(), payload.EntityID, payload.Query)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, library.ErrManualSearchEntityRequired):
				status = http.StatusBadRequest
			case errors.Is(err, library.ErrUnknownEntity):
				status = http.StatusNotFound
			}
			respondError(w, status, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"search": result})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleIndexerGrab(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if _, ok := s.requirePermission(w, r, auth.PermissionLibraryWrite); !ok {
		return
	}

	var payload struct {
		EntityID  string `json:"entity_id"`
		ReleaseID string `json:"release_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	release, err := s.service.ForceGrabRelease(r.Context(), payload.EntityID, payload.ReleaseID)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, library.ErrManualSearchEntityRequired), errors.Is(err, library.ErrManualSearchReleaseRequired):
			status = http.StatusBadRequest
		case errors.Is(err, library.ErrUnknownEntity), errors.Is(err, library.ErrManualSearchReleaseNotFound):
			status = http.StatusNotFound
		case errors.Is(err, library.ErrManualSearchReleaseGrabbed):
			status = http.StatusConflict
		}
		respondError(w, status, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"release": release})
}

func (s *Server) handleMetadataOverrides(w http.ResponseWriter, r *http.Request) {
	entityID := strings.TrimPrefix(r.URL.Path, "/api/v1/library/overrides/")
	entityID = strings.Trim(entityID, "/")
	if entityID == "" || strings.Contains(entityID, "/") {
		respondError(w, http.StatusNotFound, "metadata overrides resource not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if _, ok := s.requirePermission(w, r, auth.PermissionLibraryRead); !ok {
			return
		}
		overrides, found, err := s.service.GetMetadataOverrides(r.Context(), entityID)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, library.ErrMetadataOverrideEntityRequired):
				status = http.StatusBadRequest
			case errors.Is(err, library.ErrUnknownEntity):
				status = http.StatusNotFound
			}
			respondError(w, status, err.Error())
			return
		}
		if !found {
			respondJSON(w, http.StatusOK, map[string]any{"overrides": nil})
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"overrides": overrides})
	case http.MethodPut:
		if _, ok := s.requirePermission(w, r, auth.PermissionLibraryWrite); !ok {
			return
		}
		var payload library.MetadataOverrides
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		overrides, err := s.service.SetMetadataOverrides(r.Context(), entityID, payload)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, library.ErrMetadataOverrideEntityRequired),
				errors.Is(err, library.ErrMetadataOverrideInvalidEpisode),
				errors.Is(err, library.ErrMetadataOverrideInvalidEdition),
				errors.Is(err, library.ErrMetadataOverrideInvalidTrack),
				errors.Is(err, library.ErrMetadataOverrideInvalidTrackNum):
				status = http.StatusBadRequest
			case errors.Is(err, library.ErrUnknownEntity):
				status = http.StatusNotFound
			}
			respondError(w, status, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"overrides": overrides})
	case http.MethodDelete:
		if _, ok := s.requirePermission(w, r, auth.PermissionLibraryWrite); !ok {
			return
		}
		if err := s.service.ClearMetadataOverrides(r.Context(), entityID); err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, library.ErrMetadataOverrideEntityRequired):
				status = http.StatusBadRequest
			case errors.Is(err, library.ErrUnknownEntity):
				status = http.StatusNotFound
			}
			respondError(w, status, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut, http.MethodDelete)
	}
}

func (s *Server) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	if s.workerService == nil {
		respondError(w, http.StatusServiceUnavailable, "worker service unavailable")
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if _, ok := s.requirePermission(w, r, auth.PermissionWorkerRead); !ok {
		return
	}

	workers, err := s.workerService.ListWorkers(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workers")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"workers": workers})
}

func (s *Server) handleRegisterWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if s.workerService == nil {
		respondError(w, http.StatusServiceUnavailable, "worker service unavailable")
		return
	}

	if _, ok := s.requirePermission(w, r, auth.PermissionWorkerManage); !ok {
		return
	}

	var payload struct {
		Pool         string   `json:"pool"`
		Capabilities []string `json:"capabilities"`
		Endpoint     string   `json:"endpoint"`
		Version      string   `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := s.workerService.RegisterWorker(r.Context(), worker.RegistrationRequest{
		Pool:         payload.Pool,
		Capabilities: payload.Capabilities,
		Endpoint:     payload.Endpoint,
		Version:      payload.Version,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, worker.ErrInvalidPool) {
			status = http.StatusBadRequest
		}
		respondError(w, status, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"worker": result.Worker,
		"assignment": map[string]any{
			"stream":                     result.Assignment.Stream,
			"consumer_group":             result.Assignment.ConsumerGroup,
			"heartbeat_interval_seconds": int(result.Assignment.HeartbeatInterval / time.Second),
		},
	})
}

func (s *Server) handleWorkerRoutes(w http.ResponseWriter, r *http.Request) {
	if s.workerService == nil {
		respondError(w, http.StatusServiceUnavailable, "worker service unavailable")
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/heartbeat") {
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	if _, ok := s.requirePermission(w, r, auth.PermissionWorkerManage); !ok {
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/workers/")
	trimmed = strings.TrimSuffix(trimmed, "/heartbeat")
	workerID := strings.Trim(trimmed, "/")
	if workerID == "" {
		respondError(w, http.StatusBadRequest, "worker id required")
		return
	}

	var payload struct {
		ActiveJobs *int `json:"active_jobs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	activeJobs := 0
	if payload.ActiveJobs != nil {
		activeJobs = *payload.ActiveJobs
	}

	result, err := s.workerService.RecordHeartbeat(r.Context(), worker.HeartbeatInput{
		WorkerID:   workerID,
		ActiveJobs: activeJobs,
	})
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, worker.ErrInvalidWorkerID):
			status = http.StatusBadRequest
		case errors.Is(err, worker.ErrUnknownWorker):
			status = http.StatusNotFound
		}
		respondError(w, status, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"worker": result.Worker,
		"stale":  result.Stale,
	})
}

func (s *Server) handleWorkerPools(w http.ResponseWriter, r *http.Request) {
	if s.workerService == nil {
		respondError(w, http.StatusServiceUnavailable, "worker service unavailable")
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if _, ok := s.requirePermission(w, r, auth.PermissionWorkerRead); !ok {
		return
	}

	configs, err := s.workerService.ListPoolConfigs(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list worker pools")
		return
	}

	pools := make([]map[string]any, 0, len(configs))
	for _, cfg := range configs {
		pools = append(pools, poolConfigPayload(cfg))
	}

	respondJSON(w, http.StatusOK, map[string]any{"pools": pools})
}

func (s *Server) handleWorkerPoolConfig(w http.ResponseWriter, r *http.Request) {
	if s.workerService == nil {
		respondError(w, http.StatusServiceUnavailable, "worker service unavailable")
		return
	}
	if r.Method != http.MethodPut {
		methodNotAllowed(w, http.MethodPut)
		return
	}

	if _, ok := s.requirePermission(w, r, auth.PermissionWorkerManage); !ok {
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/worker-pools/")
	pool := strings.Trim(trimmed, "/")
	if pool == "" {
		respondError(w, http.StatusBadRequest, "pool name required")
		return
	}

	var payload struct {
		Description              string `json:"description"`
		Stream                   string `json:"stream"`
		ConsumerGroup            string `json:"consumer_group"`
		HeartbeatIntervalSeconds *int   `json:"heartbeat_interval_seconds"`
		StaleAfterSeconds        *int   `json:"stale_after_seconds"`
		MaxConcurrentJobs        *int   `json:"max_concurrent_jobs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cfg := worker.PoolConfig{
		Pool:          pool,
		Description:   payload.Description,
		Stream:        payload.Stream,
		ConsumerGroup: payload.ConsumerGroup,
	}
	if payload.HeartbeatIntervalSeconds != nil {
		cfg.HeartbeatInterval = time.Duration(*payload.HeartbeatIntervalSeconds) * time.Second
	}
	if payload.StaleAfterSeconds != nil {
		cfg.StaleAfter = time.Duration(*payload.StaleAfterSeconds) * time.Second
	}
	if payload.MaxConcurrentJobs != nil {
		cfg.MaxConcurrentJobs = *payload.MaxConcurrentJobs
	}

	updated, err := s.workerService.UpsertPoolConfig(r.Context(), cfg)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, worker.ErrInvalidPool) {
			status = http.StatusBadRequest
		}
		respondError(w, status, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"pool": poolConfigPayload(updated)})
}

func poolConfigPayload(cfg worker.PoolConfig) map[string]any {
	return map[string]any{
		"pool":                       cfg.Pool,
		"description":                cfg.Description,
		"stream":                     cfg.Stream,
		"consumer_group":             cfg.ConsumerGroup,
		"heartbeat_interval_seconds": int(cfg.HeartbeatInterval / time.Second),
		"stale_after_seconds":        int(cfg.StaleAfter / time.Second),
		"max_concurrent_jobs":        cfg.MaxConcurrentJobs,
	}
}

func (s *Server) instrument(route string, handler http.Handler) http.Handler {
	return otelhttp.NewHandler(handler, route,
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return r.Method + " " + route
		}),
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
	)
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

// Authorizer verifies API tokens and permission grants.
type Authorizer interface {
	Authorize(ctx context.Context, token string, permission auth.Permission) (auth.Principal, error)
}

const (
	headerAPIKey        = "X-Arrgo-Api-Key"
	headerAuthorization = "Authorization"
)

func (s *Server) requirePermission(w http.ResponseWriter, r *http.Request, permission auth.Permission) (auth.Principal, bool) {
	if s.authorizer == nil {
		return auth.Principal{}, true
	}

	token := extractToken(r)
	if token == "" {
		respondError(w, http.StatusUnauthorized, "missing api key")
		return auth.Principal{}, false
	}

	principal, err := s.authorizer.Authorize(r.Context(), token, permission)
	if err == nil {
		return principal, true
	}

	status := http.StatusUnauthorized
	message := "not authorized"
	switch {
	case errors.Is(err, auth.ErrPermissionDenied):
		status = http.StatusForbidden
		message = "forbidden"
	case errors.Is(err, auth.ErrAPIKeyRevoked):
		message = "api key revoked"
	case errors.Is(err, auth.ErrInvalidToken):
		message = "invalid api key"
	default:
		status = http.StatusInternalServerError
		message = "authorization failure"
	}

	respondError(w, status, message)
	return auth.Principal{}, false
}

func extractToken(r *http.Request) string {
	authz := strings.TrimSpace(r.Header.Get(headerAuthorization))
	if authz != "" {
		lower := strings.ToLower(authz)
		if strings.HasPrefix(lower, "bearer ") {
			return strings.TrimSpace(authz[7:])
		}
		return authz
	}
	return strings.TrimSpace(r.Header.Get(headerAPIKey))
}

func registerPprofHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
}
