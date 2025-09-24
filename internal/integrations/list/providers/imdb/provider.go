package imdb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arrgo/arrgo/internal/integrations/list/providers/static"
	"github.com/arrgo/arrgo/sdk/listingester"
)

// Config drives IMDb provider construction.
type Config struct {
	Profile string
	Lists   map[string]ListConfig
}

// ListConfig captures IMDb list metadata.
type ListConfig struct {
	Kind      listingester.ListKind
	MediaType string
	Entries   []Entry
	Removals  []Removal
}

// Entry represents an IMDb list entry.
type Entry struct {
	IMDBID    string
	TMDBID    string
	Title     string
	Year      int
	Rank      int
	MediaType string
	AddedAt   time.Time
	Metadata  map[string]any
}

// Removal captures a removal event.
type Removal struct {
	IMDBID    string
	TMDBID    string
	Title     string
	MediaType string
	RemovedAt time.Time
}

// Provider implements the IMDb list ingestion provider.
type Provider struct {
	profile  string
	delegate *static.Provider
}

// New constructs a new IMDb provider instance.
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
			addID(ids, "imdb", entry.IMDBID)
			addID(ids, "tmdb", entry.TMDBID)
			metadata := mergeMetadata(entry.Metadata)
			if entry.Year > 0 {
				if metadata == nil {
					metadata = make(map[string]any)
				}
				metadata["year"] = entry.Year
			}
			if entry.Rank > 0 {
				if metadata == nil {
					metadata = make(map[string]any)
				}
				metadata["rank"] = entry.Rank
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
		profile:  cfg.Profile,
		delegate: static.New("imdb", lists),
	}
}

// Name returns the provider name, including profile when supplied.
func (p *Provider) Name() string {
	if p.profile != "" {
		return fmt.Sprintf("imdb:%s", strings.ToLower(p.profile))
	}
	return "imdb"
}

// Supports delegates to the static provider.
func (p *Provider) Supports(req listingester.ListRequest) bool {
	return p.delegate.Supports(req)
}

// CheckHealth delegates to the static provider.
func (p *Provider) CheckHealth(ctx context.Context) error {
	return p.delegate.CheckHealth(ctx)
}

// Sync synchronizes the requested list.
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
