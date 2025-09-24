package list

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arrgo/arrgo/sdk/listingester"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

func TestManager_RegisterProviderDuplicate(t *testing.T) {
	mgr := NewManager(Config{})
	provider := &stubProvider{name: "trakt"}
	if err := mgr.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := mgr.RegisterProvider(provider); !errors.Is(err, ErrDuplicateProvider) {
		t.Fatalf("expected duplicate provider error, got %v", err)
	}
}

func TestManager_SyncFullSnapshot(t *testing.T) {
	clock := &fakeClock{now: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)}
	provider := &stubProvider{
		name: "trakt",
		supports: func(req listingester.ListRequest) bool {
			return req.ListID == "watchlist"
		},
		syncFn: func(ctx context.Context, req listingester.SyncRequest) (*listingester.SyncResult, error) {
			if req.Mode != listingester.SyncModeFull {
				t.Fatalf("expected full sync, got %s", req.Mode)
			}
			return &listingester.SyncResult{
				Reset:      true,
				NextCursor: "cursor-1",
				Items: []listingester.Item{
					{Title: "Dune", MediaType: "movie", ExternalIDs: map[string]string{"imdb": "tt1160419"}},
					{Title: "Foundation", MediaType: "series", ExternalIDs: map[string]string{"trakt": "foundation"}},
				},
			}, nil
		},
	}

	mgr := NewManager(Config{Clock: clock})
	if err := mgr.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	outcome, err := mgr.Sync(context.Background(), Subscription{
		ID:       "sub-1",
		Provider: "trakt",
		List: listingester.ListRequest{
			ListID:    "watchlist",
			MediaType: "movie",
		},
		Mode: listingester.SyncModeFull,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !outcome.FullSync {
		t.Fatalf("expected full sync")
	}
	if len(outcome.Added) != 2 {
		t.Fatalf("expected 2 added items, got %d", len(outcome.Added))
	}
	if outcome.Cursor != "cursor-1" {
		t.Fatalf("unexpected cursor: %s", outcome.Cursor)
	}
	if len(outcome.Snapshot) != 2 {
		t.Fatalf("expected snapshot of 2 items, got %d", len(outcome.Snapshot))
	}
}

func TestManager_SyncDeltaAddsAndRemoves(t *testing.T) {
	clock := &fakeClock{now: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)}
	provider := &stubProvider{
		name:     "imdb",
		supports: func(req listingester.ListRequest) bool { return true },
	}
	mgr := NewManager(Config{Clock: clock})
	if err := mgr.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	provider.syncFn = func(ctx context.Context, req listingester.SyncRequest) (*listingester.SyncResult, error) {
		if req.Cursor != "" {
			t.Fatalf("expected empty cursor on first sync")
		}
		return &listingester.SyncResult{
			NextCursor: "cursor-1",
			Items: []listingester.Item{{
				Title:       "Arrival",
				MediaType:   "movie",
				ExternalIDs: map[string]string{"imdb": "tt2543164"},
			}},
		}, nil
	}

	if _, err := mgr.Sync(context.Background(), Subscription{
		ID:       "sub-2",
		Provider: "imdb",
		List:     listingester.ListRequest{ListID: "wl"},
		Mode:     listingester.SyncModeDelta,
	}); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	provider.syncFn = func(ctx context.Context, req listingester.SyncRequest) (*listingester.SyncResult, error) {
		if req.Cursor != "cursor-1" {
			t.Fatalf("unexpected cursor: %s", req.Cursor)
		}
		return &listingester.SyncResult{
			NextCursor: "cursor-2",
			Items: []listingester.Item{{
				Title:       "The Creator",
				MediaType:   "movie",
				ExternalIDs: map[string]string{"imdb": "tt11858890"},
			}},
			Removed: []listingester.ItemRef{{
				MediaType:   "movie",
				ExternalIDs: map[string]string{"imdb": "tt2543164"},
			}},
		}, nil
	}

	outcome, err := mgr.Sync(context.Background(), Subscription{
		ID:       "sub-2",
		Provider: "imdb",
		List:     listingester.ListRequest{ListID: "wl"},
	})
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(outcome.Added) != 1 || outcome.Added[0].Title != "The Creator" {
		t.Fatalf("unexpected added set: %+v", outcome.Added)
	}
	if len(outcome.Removed) != 1 || outcome.Removed[0].ExternalIDs["imdb"] != "tt2543164" {
		t.Fatalf("unexpected removed set: %+v", outcome.Removed)
	}
	if len(outcome.Snapshot) != 1 {
		t.Fatalf("expected snapshot of 1 item, got %d", len(outcome.Snapshot))
	}
}

func TestManager_SyncProviderUnavailable(t *testing.T) {
	provider := &stubProvider{
		name:     "lastfm",
		supports: func(req listingester.ListRequest) bool { return true },
		syncFn: func(ctx context.Context, req listingester.SyncRequest) (*listingester.SyncResult, error) {
			return nil, listingester.ErrProviderUnavailable
		},
	}
	mgr := NewManager(Config{})
	if err := mgr.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	_, err := mgr.Sync(context.Background(), Subscription{
		ID:       "sub-3",
		Provider: "lastfm",
		List:     listingester.ListRequest{ListID: "history"},
	})
	if !errors.Is(err, listingester.ErrProviderUnavailable) {
		t.Fatalf("expected provider unavailable error, got %v", err)
	}

	provider.syncFn = func(ctx context.Context, req listingester.SyncRequest) (*listingester.SyncResult, error) {
		return &listingester.SyncResult{}, nil
	}
	if _, err := mgr.Sync(context.Background(), Subscription{
		ID:       "sub-3",
		Provider: "lastfm",
		List:     listingester.ListRequest{ListID: "history"},
	}); err != nil {
		t.Fatalf("sync after recovery: %v", err)
	}
}

func TestManager_SyncRequiresSupport(t *testing.T) {
	provider := &stubProvider{name: "trakt", supports: func(req listingester.ListRequest) bool { return false }}
	mgr := NewManager(Config{})
	if err := mgr.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	_, err := mgr.Sync(context.Background(), Subscription{
		ID:       "sub-4",
		Provider: "trakt",
		List:     listingester.ListRequest{ListID: "watchlist"},
	})
	if !errors.Is(err, listingester.ErrUnsupportedList) {
		t.Fatalf("expected unsupported list error, got %v", err)
	}
}

type stubProvider struct {
	name     string
	supports func(listingester.ListRequest) bool
	syncFn   func(context.Context, listingester.SyncRequest) (*listingester.SyncResult, error)
	health   error
}

func (p *stubProvider) Name() string { return p.name }

func (p *stubProvider) Supports(req listingester.ListRequest) bool {
	if p.supports == nil {
		return true
	}
	return p.supports(req)
}

func (p *stubProvider) CheckHealth(context.Context) error {
	return p.health
}

func (p *stubProvider) Sync(ctx context.Context, req listingester.SyncRequest) (*listingester.SyncResult, error) {
	if p.syncFn == nil {
		return &listingester.SyncResult{}, nil
	}
	return p.syncFn(ctx, req)
}

func (p *stubProvider) setHealthErr(err error) { p.health = err }

func TestManager_HealthCheckFailure(t *testing.T) {
	provider := &stubProvider{name: "trakt"}
	provider.setHealthErr(errors.New("boom"))
	mgr := NewManager(Config{})
	if err := mgr.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	_, err := mgr.Sync(context.Background(), Subscription{
		ID:       "sub-5",
		Provider: "trakt",
		List:     listingester.ListRequest{ListID: "watchlist"},
	})
	if err == nil {
		t.Fatalf("expected health failure")
	}
	if provider.health == nil {
		t.Fatalf("health error reset unexpectedly")
	}
}

func TestComputeKey(t *testing.T) {
	ids := map[string]string{"imdb": "TT123", "slug": "Example"}
	key := computeKey("movie", ids, "Example", []string{"imdb", "slug"})
	if key != "movie:imdb:tt123" {
		t.Fatalf("unexpected key: %s", key)
	}
	key = computeKey("movie", nil, "Example", []string{"imdb"})
	if key != "movie:title:example" {
		t.Fatalf("unexpected fallback key: %s", key)
	}
}
