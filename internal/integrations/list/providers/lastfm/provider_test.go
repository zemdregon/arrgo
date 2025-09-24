package lastfm

import (
	"context"
	"testing"
	"time"

	"github.com/arrgo/arrgo/sdk/listingester"
)

func TestProvider_Sync(t *testing.T) {
	played := time.Date(2024, 4, 1, 8, 30, 0, 0, time.UTC)
	provider := New(Config{
		User: "MusicFan",
		Lists: map[string]ListConfig{
			"recent": {
				Kind:      listingester.ListKindHistory,
				MediaType: "track",
				Entries: []Entry{{
					LastFMID:  "12345",
					MBID:      "mbid-1",
					SpotifyID: "spotify:track:1",
					Title:     "Seventh Heaven",
					Artist:    "Tame Impala",
					Album:     "Currents",
					PlayedAt:  played,
				}},
			},
		},
	})
	if provider.Name() != "lastfm:musicfan" {
		t.Fatalf("unexpected name: %s", provider.Name())
	}
	result, err := provider.Sync(context.Background(), listingester.SyncRequest{
		List: listingester.ListRequest{ListID: "recent"},
		Mode: listingester.SyncModeFull,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	item := result.Items[0]
	if item.ExternalIDs["lastfm"] != "12345" || item.ExternalIDs["musicbrainz"] != "mbid-1" {
		t.Fatalf("unexpected ids: %+v", item.ExternalIDs)
	}
	if item.Metadata["artist"].(string) != "Tame Impala" {
		t.Fatalf("expected artist metadata")
	}
}

func TestProvider_Removals(t *testing.T) {
	removed := time.Date(2024, 4, 2, 11, 0, 0, 0, time.UTC)
	provider := New(Config{
		Lists: map[string]ListConfig{
			"recent": {
				MediaType: "track",
				Removals: []Removal{{
					LastFMID:  "67890",
					Title:     "Old Track",
					RemovedAt: removed,
				}},
			},
		},
	})
	result, err := provider.Sync(context.Background(), listingester.SyncRequest{
		List:   listingester.ListRequest{ListID: "recent"},
		Cursor: removed.Add(-time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(result.Removed) != 1 {
		t.Fatalf("expected 1 removal, got %d", len(result.Removed))
	}
	if result.Removed[0].ExternalIDs["lastfm"] != "67890" {
		t.Fatalf("unexpected removal ids: %+v", result.Removed[0].ExternalIDs)
	}
}
