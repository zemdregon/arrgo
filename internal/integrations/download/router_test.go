package download

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeClient struct {
	name     string
	priority int

	supportsFn func(Request) bool

	healthMu sync.Mutex
	health   []error
	healthAt int

	enqueueMu sync.Mutex
	enqueue   []error
	enqueueAt int
}

func (f *fakeClient) Name() string { return f.name }

func (f *fakeClient) Priority() int { return f.priority }

func (f *fakeClient) Supports(req Request) bool {
	if f.supportsFn != nil {
		return f.supportsFn(req)
	}
	return true
}

func (f *fakeClient) CheckHealth(ctx context.Context) error {
	_ = ctx
	f.healthMu.Lock()
	defer f.healthMu.Unlock()
	if len(f.health) == 0 {
		return nil
	}
	if f.healthAt >= len(f.health) {
		return f.health[len(f.health)-1]
	}
	err := f.health[f.healthAt]
	f.healthAt++
	return err
}

func (f *fakeClient) Enqueue(ctx context.Context, req Request) error {
	_ = ctx
	_ = req
	f.enqueueMu.Lock()
	defer f.enqueueMu.Unlock()
	if len(f.enqueue) == 0 {
		return nil
	}
	if f.enqueueAt >= len(f.enqueue) {
		return f.enqueue[len(f.enqueue)-1]
	}
	err := f.enqueue[f.enqueueAt]
	f.enqueueAt++
	return err
}

type staticClock struct {
	mu sync.Mutex
	t  time.Time
}

func newStaticClock(start time.Time) *staticClock {
	return &staticClock{t: start}
}

func (c *staticClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *staticClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestRouterDispatchPrioritizesByPriority(t *testing.T) {
	router := NewRouter(RouterConfig{})

	primary := &fakeClient{
		name:     "primary",
		priority: 0,
		health:   []error{nil},
		enqueue:  []error{nil},
	}
	secondary := &fakeClient{
		name:     "secondary",
		priority: 10,
		health:   []error{nil},
		enqueue:  []error{nil},
	}

	if err := router.RegisterClient(secondary); err != nil {
		t.Fatalf("register secondary: %v", err)
	}
	if err := router.RegisterClient(primary); err != nil {
		t.Fatalf("register primary: %v", err)
	}

	result, err := router.Dispatch(context.Background(), Request{Protocol: "torrent"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.ClientName != "primary" {
		t.Fatalf("expected primary client, got %s", result.ClientName)
	}
	if result.Attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", result.Attempts)
	}
	if secondary.enqueueAt != 0 {
		t.Fatalf("expected secondary client unused, got %d attempts", secondary.enqueueAt)
	}
}

func TestRouterFailoverOnUnavailableClient(t *testing.T) {
	router := NewRouter(RouterConfig{})

	primary := &fakeClient{
		name:     "primary",
		priority: 0,
		health:   []error{nil},
		enqueue:  []error{ErrClientUnavailable},
	}
	secondary := &fakeClient{
		name:     "secondary",
		priority: 5,
		health:   []error{nil},
		enqueue:  []error{nil},
	}

	if err := router.RegisterClient(primary); err != nil {
		t.Fatalf("register primary: %v", err)
	}
	if err := router.RegisterClient(secondary); err != nil {
		t.Fatalf("register secondary: %v", err)
	}

	result, err := router.Dispatch(context.Background(), Request{MediaType: "series"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.ClientName != "secondary" {
		t.Fatalf("expected failover to secondary, got %s", result.ClientName)
	}
	if result.Attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", result.Attempts)
	}
	if primary.enqueueAt != 1 {
		t.Fatalf("primary enqueue attempts: expected 1, got %d", primary.enqueueAt)
	}
}

func TestRouterSkipsUnsupportedClients(t *testing.T) {
	router := NewRouter(RouterConfig{})

	torrentOnly := &fakeClient{
		name:     "torrent",
		priority: 0,
		health:   []error{nil},
		enqueue:  []error{nil},
		supportsFn: func(req Request) bool {
			return req.Protocol == "torrent"
		},
	}
	usenet := &fakeClient{
		name:     "usenet",
		priority: 5,
		health:   []error{nil},
		enqueue:  []error{nil},
		supportsFn: func(req Request) bool {
			return req.Protocol == "usenet"
		},
	}

	if err := router.RegisterClient(torrentOnly); err != nil {
		t.Fatalf("register torrent: %v", err)
	}
	if err := router.RegisterClient(usenet); err != nil {
		t.Fatalf("register usenet: %v", err)
	}

	result, err := router.Dispatch(context.Background(), Request{Protocol: "usenet"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.ClientName != "usenet" {
		t.Fatalf("expected usenet client, got %s", result.ClientName)
	}
	if torrentOnly.enqueueAt != 0 {
		t.Fatalf("expected torrent client skipped, attempts=%d", torrentOnly.enqueueAt)
	}
}

func TestRouterRechecksHealthAfterTTL(t *testing.T) {
	clock := newStaticClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	router := NewRouter(RouterConfig{HealthTTL: time.Minute, Clock: clock})

	primary := &fakeClient{
		name:     "primary",
		priority: 0,
		health: []error{
			errors.New("down"),
			nil,
		},
		enqueue: []error{nil},
	}
	secondary := &fakeClient{
		name:     "secondary",
		priority: 5,
		health:   []error{nil},
		enqueue:  []error{nil},
	}

	if err := router.RegisterClient(primary); err != nil {
		t.Fatalf("register primary: %v", err)
	}
	if err := router.RegisterClient(secondary); err != nil {
		t.Fatalf("register secondary: %v", err)
	}

	result, err := router.Dispatch(context.Background(), Request{MediaType: "movie"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.ClientName != "secondary" {
		t.Fatalf("expected secondary due to unhealthy primary, got %s", result.ClientName)
	}

	clock.Advance(time.Minute + time.Second)

	result, err = router.Dispatch(context.Background(), Request{MediaType: "movie"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.ClientName != "primary" {
		t.Fatalf("expected primary after recovery, got %s", result.ClientName)
	}
}

func TestRouterErrorsWhenNoSupportingClients(t *testing.T) {
	router := NewRouter(RouterConfig{})

	if err := router.RegisterClient(&fakeClient{
		name:     "torrent",
		priority: 0,
		supportsFn: func(req Request) bool {
			return req.Protocol == "torrent"
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err := router.Dispatch(context.Background(), Request{Protocol: "usenet"})
	if !errors.Is(err, ErrNoSupportingClients) {
		t.Fatalf("expected ErrNoSupportingClients, got %v", err)
	}
}

func TestRouterErrorsWhenAllClientsUnhealthy(t *testing.T) {
	router := NewRouter(RouterConfig{})

	if err := router.RegisterClient(&fakeClient{
		name:     "primary",
		priority: 0,
		health:   []error{errors.New("offline")},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := router.RegisterClient(&fakeClient{
		name:     "secondary",
		priority: 1,
		health:   []error{errors.New("offline")},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err := router.Dispatch(context.Background(), Request{})
	if !errors.Is(err, ErrNoHealthyClients) {
		t.Fatalf("expected ErrNoHealthyClients, got %v", err)
	}
}

func TestRouterRejectsDuplicateNames(t *testing.T) {
	router := NewRouter(RouterConfig{})

	if err := router.RegisterClient(&fakeClient{name: "dup", priority: 1}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := router.RegisterClient(&fakeClient{name: "dup", priority: 2}); !errors.Is(err, ErrDuplicateClient) {
		t.Fatalf("expected ErrDuplicateClient, got %v", err)
	}
}
