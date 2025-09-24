package imdb

import (
	"context"
	"testing"
	"time"

	"github.com/arrgo/arrgo/sdk/listingester"
)

func TestProvider_Sync(t *testing.T) {
	added := time.Date(2024, 3, 2, 10, 30, 0, 0, time.UTC)
	provider := New(Config{
		Profile: "Primary",
		Lists: map[string]ListConfig{
			"top": {
				Kind:      listingester.ListKindCollection,
				MediaType: "movie",
				Entries: []Entry{{
					IMDBID:    "tt0468569",
					TMDBID:    "155",
					Title:     "The Dark Knight",
					Year:      2008,
					Rank:      1,
					MediaType: "movie",
					AddedAt:   added,
				}},
			},
		},
	})
	if provider.Name() != "imdb:primary" {
		t.Fatalf("unexpected name: %s", provider.Name())
	}
	result, err := provider.Sync(context.Background(), listingester.SyncRequest{
		List: listingester.ListRequest{ListID: "top"},
		Mode: listingester.SyncModeFull,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	item := result.Items[0]
	if item.ExternalIDs["imdb"] != "tt0468569" {
		t.Fatalf("unexpected imdb id: %s", item.ExternalIDs["imdb"])
	}
	if item.Metadata["rank"].(int) != 1 {
		t.Fatalf("expected rank metadata")
	}
}

func TestProvider_Removals(t *testing.T) {
	removed := time.Date(2024, 3, 5, 9, 0, 0, 0, time.UTC)
	provider := New(Config{
		Lists: map[string]ListConfig{
			"watch": {
				MediaType: "movie",
				Removals: []Removal{{
					IMDBID:    "tt0133093",
					Title:     "The Matrix",
					MediaType: "movie",
					RemovedAt: removed,
				}},
			},
		},
	})

	result, err := provider.Sync(context.Background(), listingester.SyncRequest{
		List:   listingester.ListRequest{ListID: "watch"},
		Cursor: removed.Add(-time.Minute).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(result.Removed) != 1 {
		t.Fatalf("expected 1 removal, got %d", len(result.Removed))
	}
	if result.Removed[0].ExternalIDs["imdb"] != "tt0133093" {
		t.Fatalf("unexpected removal ids: %+v", result.Removed[0].ExternalIDs)
	}
}
