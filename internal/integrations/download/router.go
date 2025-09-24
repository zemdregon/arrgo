package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"

	downloadclient "github.com/arrgo/arrgo/sdk/downloadclient"
)

// Request captures the information required to enqueue a download operation.
type Request = downloadclient.Request

// Client models an individual download client integration that can accept work.
type Client = downloadclient.Client

// Clock abstracts time retrieval for deterministic testing.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// RouterConfig controls Router construction.
type RouterConfig struct {
	Clock     Clock
	Logger    *slog.Logger
	HealthTTL time.Duration
}

// DispatchResult records information about a successful dispatch.
type DispatchResult struct {
	ClientName string
	Attempts   int
}

// Errors exposed by the router.
var (
	ErrNoClientsRegistered  = errors.New("download: no download clients registered")
	ErrNoSupportingClients  = errors.New("download: no download clients support the request")
	ErrNoHealthyClients     = errors.New("download: no healthy download clients available")
	ErrClientUnavailable    = downloadclient.ErrClientUnavailable
	ErrDuplicateClient      = errors.New("download: client with name already registered")
	errUnsupportedOperation = errors.New("download: operation unsupported")
)

// Router handles prioritized routing of download requests across one or more clients.
type Router struct {
	mu        sync.RWMutex
	clients   []*registeredClient
	byName    map[string]*registeredClient
	clock     Clock
	logger    *slog.Logger
	healthTTL time.Duration
}

type registeredClient struct {
	client    Client
	mu        sync.Mutex
	healthy   bool
	lastCheck time.Time
	lastErr   error
}

// NewRouter constructs a Router using the supplied configuration.
func NewRouter(cfg RouterConfig) *Router {
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	ttl := cfg.HealthTTL
	if ttl < 0 {
		ttl = 0
	}

	return &Router{
		clients:   make([]*registeredClient, 0),
		byName:    make(map[string]*registeredClient),
		clock:     clock,
		logger:    logger,
		healthTTL: ttl,
	}
}

// RegisterClient adds a new client to the routing pool.
func (r *Router) RegisterClient(client Client) error {
	if client == nil {
		return fmt.Errorf("download: client is required: %w", errUnsupportedOperation)
	}

	rc := &registeredClient{client: client}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byName[client.Name()]; ok {
		return ErrDuplicateClient
	}

	insertIdx := sort.Search(len(r.clients), func(i int) bool {
		left := r.clients[i].client.Priority()
		right := client.Priority()
		if left == right {
			return r.clients[i].client.Name() > client.Name()
		}
		return left > right
	})

	r.clients = append(r.clients, nil)
	copy(r.clients[insertIdx+1:], r.clients[insertIdx:])
	r.clients[insertIdx] = rc
	r.byName[client.Name()] = rc
	r.logger.Info("registered download client", "client", client.Name(), "priority", client.Priority())
	return nil
}

// Dispatch attempts to enqueue the request against the highest priority healthy client.
func (r *Router) Dispatch(ctx context.Context, req Request) (*DispatchResult, error) {
	r.mu.RLock()
	if len(r.clients) == 0 {
		r.mu.RUnlock()
		return nil, ErrNoClientsRegistered
	}
	snapshot := make([]*registeredClient, 0, len(r.clients))
	for _, rc := range r.clients {
		snapshot = append(snapshot, rc)
	}
	r.mu.RUnlock()

	supported := snapshot[:0]
	for _, rc := range snapshot {
		if rc.client.Supports(req) {
			supported = append(supported, rc)
		}
	}
	if len(supported) == 0 {
		return nil, ErrNoSupportingClients
	}

	var lastErr error
	attempts := 0
	for _, rc := range supported {
		if !r.ensureHealthy(ctx, rc) {
			lastErr = rc.failure()
			continue
		}

		attempts++
		if err := rc.client.Enqueue(ctx, req); err != nil {
			if errors.Is(err, ErrClientUnavailable) {
				r.markUnhealthy(rc, err)
				lastErr = err
				r.logger.Warn("download client unavailable, attempting failover", "client", rc.client.Name(), "err", err)
				continue
			}
			return nil, err
		}

		r.logger.Info("dispatched download request", "client", rc.client.Name(), "attempts", attempts)
		return &DispatchResult{ClientName: rc.client.Name(), Attempts: attempts}, nil
	}

	if lastErr != nil {
		return nil, errors.Join(ErrNoHealthyClients, lastErr)
	}
	return nil, ErrNoHealthyClients
}

func (r *Router) ensureHealthy(ctx context.Context, rc *registeredClient) bool {
	ttl := r.healthTTL

	rc.mu.Lock()
	healthy := rc.healthy
	lastCheck := rc.lastCheck
	rc.mu.Unlock()

	if ttl > 0 && healthy && r.clock.Now().Sub(lastCheck) <= ttl {
		return true
	}

	if err := rc.client.CheckHealth(ctx); err != nil {
		rc.mu.Lock()
		rc.healthy = false
		rc.lastErr = err
		rc.lastCheck = r.clock.Now()
		rc.mu.Unlock()
		r.logger.Warn("download client health check failed", "client", rc.client.Name(), "err", err)
		return false
	}

	rc.mu.Lock()
	rc.healthy = true
	rc.lastErr = nil
	rc.lastCheck = r.clock.Now()
	rc.mu.Unlock()
	return true
}

func (r *Router) markUnhealthy(rc *registeredClient, err error) {
	rc.mu.Lock()
	rc.healthy = false
	rc.lastErr = err
	rc.lastCheck = r.clock.Now()
	rc.mu.Unlock()
}

func (rc *registeredClient) failure() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.lastErr
}
