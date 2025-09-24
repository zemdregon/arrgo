package static

import (
	"context"
	"testing"
	"time"

	"github.com/arrgo/arrgo/sdk/listingester"
)

func TestProvider_FullSync(t *testing.T) {
	addedAt := time.Date(2024, 1, 10, 15, 0, 0, 0, time.UTC)
	provider := New("trakt", map[string]List{
		"watchlist": {
			Kind:      listingester.ListKindWatchlist,
			MediaType: "movie",
			Items: []Item{{
				Title:     "Dune",
				MediaType: "movie",
				ExternalIDs: map[string]string{
					"imdb": "tt1160419",
				},
				AddedAt: addedAt,
			}},
		},
	})

	result, err := provider.Sync(context.Background(), listingester.SyncRequest{
		List: listingester.ListRequest{ListID: "watchlist"},
		Mode: listingester.SyncModeFull,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].ExternalIDs["imdb"] != "tt1160419" {
		t.Fatalf("unexpected item ids: %+v", result.Items[0].ExternalIDs)
	}
	if !result.Reset {
		t.Fatalf("expected reset for full sync")
	}
}

func TestProvider_DeltaSync(t *testing.T) {
	added1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	added2 := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	removed := time.Date(2024, 1, 3, 9, 0, 0, 0, time.UTC)
	provider := New("imdb", map[string]List{
		"wl": {
			MediaType: "movie",
			Items:     []Item{{Title: "Arrival", MediaType: "movie", AddedAt: added1}, {Title: "The Creator", MediaType: "movie", AddedAt: added2}},
			Removals:  []Removal{{Title: "Arrival", MediaType: "movie", RemovedAt: removed}},
		},
	})

	result, err := provider.Sync(context.Background(), listingester.SyncRequest{
		List:   listingester.ListRequest{ListID: "wl"},
		Cursor: added1.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "The Creator" {
		t.Fatalf("unexpected delta items: %+v", result.Items)
	}
	if len(result.Removed) != 1 || result.Removed[0].Title != "Arrival" {
		t.Fatalf("unexpected removals: %+v", result.Removed)
	}
	if result.Reset {
		t.Fatalf("expected delta to not reset state")
	}
}

func TestProvider_UnsupportedList(t *testing.T) {
	provider := New("lastfm", map[string]List{})
	if provider.Supports(listingester.ListRequest{ListID: "unknown"}) {
		t.Fatalf("expected unsupported list")
	}
	if _, err := provider.Sync(context.Background(), listingester.SyncRequest{List: listingester.ListRequest{ListID: "unknown"}}); err == nil {
		t.Fatalf("expected unsupported list error")
	}
}
