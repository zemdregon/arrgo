package lastfm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arrgo/arrgo/internal/integrations/list/providers/static"
	"github.com/arrgo/arrgo/sdk/listingester"
)

// Config drives Last.fm provider construction.
type Config struct {
	User  string
	Lists map[string]ListConfig
}

// ListConfig captures Last.fm list configuration.
type ListConfig struct {
	Kind      listingester.ListKind
	MediaType string
	Entries   []Entry
	Removals  []Removal
}

// Entry represents a Last.fm scrobble or list entry.
type Entry struct {
	LastFMID  string
	MBID      string
	SpotifyID string
	Title     string
	Artist    string
	Album     string
	MediaType string
	PlayedAt  time.Time
	Metadata  map[string]any
}

// Removal captures a removal event from Last.fm history.
type Removal struct {
	LastFMID  string
	MBID      string
	Title     string
	MediaType string
	RemovedAt time.Time
}

// Provider implements the Last.fm list ingestion provider.
type Provider struct {
	user     string
	delegate *static.Provider
}

// New constructs a Last.fm provider instance.
func New(cfg Config) *Provider {
	lists := make(map[string]static.List, len(cfg.Lists))
	for id, listCfg := range cfg.Lists {
		list := static.List{
			Kind:      listCfg.Kind,
			MediaType: listCfg.MediaType,
			Items:     make([]static.Item, 0, len(listCfg.Entries)),
			Removals:  make([]static.Removal, 0, len(listCfg.Removals)),
		}
		for _, entry := range listCfg.Entries {
			ids := make(map[string]string)
			addID(ids, "lastfm", entry.LastFMID)
			addID(ids, "musicbrainz", entry.MBID)
			addID(ids, "spotify", entry.SpotifyID)
			metadata := mergeMetadata(entry.Metadata)
			if entry.Artist != "" {
				if metadata == nil {
					metadata = make(map[string]any)
				}
				metadata["artist"] = entry.Artist
			}
			if entry.Album != "" {
				if metadata == nil {
					metadata = make(map[string]any)
				}
				metadata["album"] = entry.Album
			}
			list.Items = append(list.Items, static.Item{
				Title:       entry.Title,
				MediaType:   firstNonEmpty(entry.MediaType, listCfg.MediaType, "track"),
				ExternalIDs: ids,
				AddedAt:     entry.PlayedAt,
				Metadata:    metadata,
			})
		}
		for _, removal := range listCfg.Removals {
			ids := make(map[string]string)
			addID(ids, "lastfm", removal.LastFMID)
			addID(ids, "musicbrainz", removal.MBID)
			list.Removals = append(list.Removals, static.Removal{
				Title:       removal.Title,
				MediaType:   firstNonEmpty(removal.MediaType, listCfg.MediaType, "track"),
				ExternalIDs: ids,
				RemovedAt:   removal.RemovedAt,
			})
		}
		lists[id] = list
	}

	return &Provider{
		user:     cfg.User,
		delegate: static.New("lastfm", lists),
	}
}

// Name returns the provider name including the configured username.
func (p *Provider) Name() string {
	if p.user != "" {
		return fmt.Sprintf("lastfm:%s", strings.ToLower(p.user))
	}
	return "lastfm"
}

// Supports delegates to the static provider.
func (p *Provider) Supports(req listingester.ListRequest) bool {
	return p.delegate.Supports(req)
}

// CheckHealth delegates to the static provider health probe.
func (p *Provider) CheckHealth(ctx context.Context) error {
	return p.delegate.CheckHealth(ctx)
}

// Sync performs a synchronization cycle.
func (p *Provider) Sync(ctx context.Context, req listingester.SyncRequest) (*listingester.SyncResult, error) {
	return p.delegate.Sync(ctx, req)
}

// SetHealthError updates the health error returned by CheckHealth.
func (p *Provider) SetHealthError(err error) {
	p.delegate.SetHealthError(err)
}

func addID(ids map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	ids[key] = value
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func mergeMetadata(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	clone := make(map[string]any, len(src))
	for k, v := range src {
		clone[k] = v
	}
	return clone
}
