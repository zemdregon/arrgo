package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// EntityType enumerates the catalog entity groups the pipeline can ingest.
type EntityType string

const (
	EntityTypeSeries  EntityType = "series"
	EntityTypeSeason  EntityType = "season"
	EntityTypeEpisode EntityType = "episode"
	EntityTypeMovie   EntityType = "movie"
	EntityTypeArtist  EntityType = "artist"
	EntityTypeAlbum   EntityType = "album"
	EntityTypeTrack   EntityType = "track"
	EntityTypeUnknown EntityType = "unknown"
)

// Request represents a metadata ingestion request for a catalog entity.
type Request struct {
	// EntityID is the canonical arrgo identifier for the entity. It should be
	// provided whenever the entity already exists in the arrgo database.
	EntityID string
	// EntityType describes the category of media being ingested.
	EntityType EntityType
	// ExternalIDs maps metadata provider identifiers (e.g., tmdb, musicbrainz)
	// to their corresponding values.
	ExternalIDs map[string]string
	// ForceRefresh bypasses cached metadata when true.
	ForceRefresh bool
	// CacheKeyOverride allows callers to provide a deterministic cache key.
	CacheKeyOverride string
}

// Validate ensures the request contains sufficient information to be executed.
func (r Request) Validate() error {
	if r.EntityType == "" {
		return fmt.Errorf("metadata: entity type is required")
	}
	if r.EntityID == "" && len(r.ExternalIDs) == 0 {
		return fmt.Errorf("metadata: entity id or at least one external id is required")
	}
	return nil
}

// CacheKey returns the cache key used for storing metadata records.
func (r Request) CacheKey() string {
	if r.CacheKeyOverride != "" {
		return r.CacheKeyOverride
	}

	base := strings.Builder{}
	base.WriteString("metadata:")
	base.WriteString(string(r.EntityType))
	base.WriteString(":")

	if r.EntityID != "" {
		base.WriteString(strings.ToLower(r.EntityID))
		return base.String()
	}

	if len(r.ExternalIDs) == 0 {
		return base.String()
	}

	keys := make([]string, 0, len(r.ExternalIDs))
	for provider := range r.ExternalIDs {
		keys = append(keys, provider)
	}
	sortStrings(keys)

	for idx, provider := range keys {
		if idx > 0 {
			base.WriteString("|")
		}
		base.WriteString(strings.ToLower(provider))
		base.WriteString("=")
		base.WriteString(strings.ToLower(r.ExternalIDs[provider]))
	}

	return base.String()
}

// Metadata captures the normalized metadata attributes used by arrgo.
type Metadata struct {
	EntityID          string              `json:"entity_id"`
	EntityType        EntityType          `json:"entity_type"`
	CanonicalTitle    string              `json:"canonical_title,omitempty"`
	SortTitle         string              `json:"sort_title,omitempty"`
	AlternateTitles   []string            `json:"alternate_titles,omitempty"`
	Synopsis          string              `json:"synopsis,omitempty"`
	ReleaseDate       *time.Time          `json:"release_date,omitempty"`
	Genres            []string            `json:"genres,omitempty"`
	Tags              []string            `json:"tags,omitempty"`
	ExternalIDs       map[string]string   `json:"external_ids,omitempty"`
	ArtworkURL        string              `json:"artwork_url,omitempty"`
	RuntimeMinutes    int                 `json:"runtime_minutes,omitempty"`
	SourceAttribution []SourceAttribution `json:"source_attribution,omitempty"`
	LastRefreshed     time.Time           `json:"last_refreshed"`
}

// SourceAttribution documents which provider supplied particular fields.
type SourceAttribution struct {
	Provider    string    `json:"provider"`
	Fields      []string  `json:"fields"`
	RetrievedAt time.Time `json:"retrieved_at"`
}

// MarshalBinary serializes the metadata to JSON for cache storage.
func (m Metadata) MarshalBinary() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalBinary loads metadata from its JSON representation.
func (m *Metadata) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, m)
}

// ProviderResponse is returned by metadata providers during ingestion.
type ProviderResponse struct {
	Metadata    Metadata
	Priority    int
	TTL         time.Duration
	RetrievedAt time.Time
}

// Provider defines the interface for upstream metadata providers.
type Provider interface {
	Name() string
	Supports(entityType EntityType) bool
	Fetch(ctx context.Context, req Request) (ProviderResponse, error)
}

// Cache abstracts Redis-backed metadata storage. Implementations must be safe
// for concurrent use by the pipeline.
type Cache interface {
	Fetch(ctx context.Context, key string) (Metadata, bool, error)
	Save(ctx context.Context, key string, value Metadata, ttl time.Duration) error
}

// sortStrings orders a string slice in-place using case-insensitive comparison.
func sortStrings(values []string) {
	sort.Slice(values, func(i, j int) bool {
		return strings.ToLower(values[i]) < strings.ToLower(values[j])
	})
}
