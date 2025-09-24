package trakt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arrgo/arrgo/internal/integrations/list/providers/static"
	"github.com/arrgo/arrgo/sdk/listingester"
)

// Config drives Trakt provider construction.
type Config struct {
	Username string
	Lists    map[string]ListConfig
}

// ListConfig describes a Trakt list to expose via the provider.
type ListConfig struct {
	Kind      listingester.ListKind
	MediaType string
	Entries   []Entry
	Removals  []Removal
}

// Entry models a Trakt list entry.
type Entry struct {
	TraktID   string
	Slug      string
	Title     string
	Year      int
	IMDBID    string
	TMDBID    string
	MediaType string
	AddedAt   time.Time
	Metadata  map[string]any
}

// Removal captures a removal event from a Trakt list.
type Removal struct {
	TraktID   string
	Slug      string
	Title     string
	IMDBID    string
	TMDBID    string
	MediaType string
	RemovedAt time.Time
}

// Provider implements the Trakt list ingestion provider using static backing data.
type Provider struct {
	username string
	delegate *static.Provider
}

// New constructs a Trakt provider instance using the supplied configuration.
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
			addID(ids, "trakt", entry.TraktID)
			addID(ids, "slug", entry.Slug)
			addID(ids, "imdb", entry.IMDBID)
			addID(ids, "tmdb", entry.TMDBID)
			metadata := mergeMetadata(entry.Metadata)
			if entry.Year > 0 {
				if metadata == nil {
					metadata = make(map[string]any)
				}
				metadata["year"] = entry.Year
			}
			list.Items = append(list.Items, static.Item{
				Title:       entry.Title,
				MediaType:   firstNonEmpty(entry.MediaType, listCfg.MediaType),
				ExternalIDs: ids,
				AddedAt:     entry.AddedAt,
				Metadata:    metadata,
			})
		}
		for _, removal := range listCfg.Removals {
			ids := make(map[string]string)
			addID(ids, "trakt", removal.TraktID)
			addID(ids, "slug", removal.Slug)
			addID(ids, "imdb", removal.IMDBID)
			addID(ids, "tmdb", removal.TMDBID)
			list.Removals = append(list.Removals, static.Removal{
				Title:       removal.Title,
				MediaType:   firstNonEmpty(removal.MediaType, listCfg.MediaType),
				ExternalIDs: ids,
				RemovedAt:   removal.RemovedAt,
			})
		}
		lists[id] = list
	}

	return &Provider{
		username: cfg.Username,
		delegate: static.New("trakt", lists),
	}
}

// Name returns the provider name, including the username when provided.
func (p *Provider) Name() string {
	if p.username != "" {
		return fmt.Sprintf("trakt:%s", strings.ToLower(p.username))
	}
	return "trakt"
}

// Supports reports whether the request can be served by this provider.
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

// SetHealthError updates the underlying provider health error.
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
