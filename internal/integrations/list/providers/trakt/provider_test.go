package trakt

import (
	"context"
	"testing"
	"time"

	"github.com/arrgo/arrgo/sdk/listingester"
)

func TestProvider_Sync(t *testing.T) {
	added := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	provider := New(Config{
		Username: "ExampleUser",
		Lists: map[string]ListConfig{
			"watchlist": {
				Kind:      listingester.ListKindWatchlist,
				MediaType: "movie",
				Entries: []Entry{{
					TraktID:   "123",
					Slug:      "dune",
					Title:     "Dune",
					Year:      2021,
					IMDBID:    "tt1160419",
					TMDBID:    "438631",
					MediaType: "movie",
					AddedAt:   added,
				}},
			},
		},
	})

	if provider.Name() != "trakt:exampleuser" {
		t.Fatalf("unexpected name: %s", provider.Name())
	}
	if !provider.Supports(listingester.ListRequest{ListID: "watchlist"}) {
		t.Fatalf("expected list support")
	}
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
	item := result.Items[0]
	if item.ExternalIDs["trakt"] != "123" || item.ExternalIDs["imdb"] != "tt1160419" {
		t.Fatalf("unexpected ids: %+v", item.ExternalIDs)
	}
	if item.Metadata["year"].(int) != 2021 {
		t.Fatalf("expected year metadata")
	}
}

func TestProvider_HealthForwarding(t *testing.T) {
	provider := New(Config{})
	provider.SetHealthError(assertErr)
	if err := provider.CheckHealth(context.Background()); err != assertErr {
		t.Fatalf("unexpected health error: %v", err)
	}
}

type sentinelError string

func (e sentinelError) Error() string { return string(e) }

var assertErr = sentinelError("unhealthy")
