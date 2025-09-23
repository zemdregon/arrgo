package metadata

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestPipelineReturnsCacheHit(t *testing.T) {
	cache := newStubCache()
	existing := Metadata{
		EntityID:       "series-123",
		EntityType:     EntityTypeSeries,
		CanonicalTitle: "Existing Series",
		LastRefreshed:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	cache.store["metadata:series:series-123"] = existing

	provider := &stubProvider{name: "test-provider"}

	pipeline, err := NewPipeline(PipelineConfig{
		Cache:      cache,
		Providers:  []Provider{provider},
		DefaultTTL: time.Hour,
		Clock:      fakeClock{now: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewPipeline() error = %v", err)
	}

	req := Request{EntityType: EntityTypeSeries, EntityID: "series-123"}
	result, err := pipeline.Ingest(context.Background(), req)
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	if !result.CacheHit {
		t.Fatalf("expected cache hit")
	}
	if !reflect.DeepEqual(result.Metadata, existing) {
		t.Fatalf("metadata mismatch: got %+v want %+v", result.Metadata, existing)
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not to be called on cache hit, got %d", provider.calls)
	}
}

func TestPipelineFetchesProvidersAndCaches(t *testing.T) {
	cache := newStubCache()
	clock := fakeClock{now: time.Date(2024, 3, 10, 12, 0, 0, 0, time.UTC)}

	releaseDate := time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC)
	provider := &stubProvider{
		name: "tmdb",
		resp: ProviderResponse{
			Metadata: Metadata{
				EntityID:       "movie-1",
				EntityType:     EntityTypeMovie,
				CanonicalTitle: "Example Movie",
				Synopsis:       "An illustrative movie for testing.",
				ReleaseDate:    &releaseDate,
				Genres:         []string{"Drama"},
			},
			Priority:    5,
			TTL:         45 * time.Minute,
			RetrievedAt: clock.now.Add(-time.Minute),
		},
	}

	pipeline, err := NewPipeline(PipelineConfig{
		Cache:      cache,
		Providers:  []Provider{provider},
		DefaultTTL: time.Hour,
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("NewPipeline() error = %v", err)
	}

	req := Request{EntityType: EntityTypeMovie, EntityID: "movie-1"}
	result, err := pipeline.Ingest(context.Background(), req)
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	if result.CacheHit {
		t.Fatalf("expected cache miss")
	}

	if result.Metadata.CanonicalTitle != "Example Movie" {
		t.Fatalf("unexpected canonical title: %s", result.Metadata.CanonicalTitle)
	}
	if result.Metadata.ReleaseDate == nil || !result.Metadata.ReleaseDate.Equal(releaseDate) {
		t.Fatalf("release date mismatch: %+v", result.Metadata.ReleaseDate)
	}
	if len(result.Metadata.Genres) != 1 || result.Metadata.Genres[0] != "Drama" {
		t.Fatalf("genres not merged correctly: %+v", result.Metadata.Genres)
	}

	if cache.saveCount != 1 {
		t.Fatalf("expected cache save, got %d", cache.saveCount)
	}
	ttl, ok := cache.ttl["metadata:movie:movie-1"]
	if !ok {
		t.Fatalf("cached entry missing")
	}
	if ttl != 45*time.Minute {
		t.Fatalf("unexpected ttl: %s", ttl)
	}

	if len(result.ProviderReports) != 1 {
		t.Fatalf("unexpected provider reports: %+v", result.ProviderReports)
	}
	if !result.ProviderReports[0].Used {
		t.Fatalf("expected provider marked as used")
	}
	if result.ProviderReports[0].Priority != 5 {
		t.Fatalf("expected provider priority recorded")
	}
}

func TestPipelineMergesProviderResults(t *testing.T) {
	cache := newStubCache()
	clock := fakeClock{now: time.Date(2024, 4, 1, 9, 0, 0, 0, time.UTC)}

	first := &stubProvider{
		name: "tmdb",
		resp: ProviderResponse{
			Metadata: Metadata{
				CanonicalTitle:  "Primary Title",
				SortTitle:       "primary title",
				AlternateTitles: []string{"Alt One"},
				Genres:          []string{"Science Fiction"},
			},
			Priority:    5,
			TTL:         30 * time.Minute,
			RetrievedAt: clock.now.Add(-2 * time.Minute),
		},
	}

	second := &stubProvider{
		name: "musicbrainz",
		resp: ProviderResponse{
			Metadata: Metadata{
				CanonicalTitle: "Override Title",
				Synopsis:       "Detailed synopsis.",
				Tags:           []string{"cult", "classic"},
			},
			Priority:    2,
			TTL:         time.Hour,
			RetrievedAt: clock.now.Add(-time.Minute),
		},
	}

	pipeline, err := NewPipeline(PipelineConfig{
		Cache:      cache,
		Providers:  []Provider{first, second},
		DefaultTTL: 2 * time.Hour,
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("NewPipeline() error = %v", err)
	}

	req := Request{EntityType: EntityTypeMovie, EntityID: "movie-42"}
	result, err := pipeline.Ingest(context.Background(), req)
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	if result.Metadata.CanonicalTitle != "Override Title" {
		t.Fatalf("expected canonical title override, got %s", result.Metadata.CanonicalTitle)
	}
	if result.Metadata.SortTitle != "primary title" {
		t.Fatalf("expected sort title from first provider, got %s", result.Metadata.SortTitle)
	}
	if len(result.Metadata.Genres) != 1 || result.Metadata.Genres[0] != "Science Fiction" {
		t.Fatalf("expected genres merged, got %+v", result.Metadata.Genres)
	}
	if len(result.Metadata.Tags) != 2 {
		t.Fatalf("expected tags merged, got %+v", result.Metadata.Tags)
	}
	if len(result.Metadata.SourceAttribution) != 2 {
		t.Fatalf("expected source attribution for two providers, got %+v", result.Metadata.SourceAttribution)
	}

	if cache.saveCount != 1 {
		t.Fatalf("expected cache save, got %d", cache.saveCount)
	}
	ttl := cache.ttl["metadata:movie:movie-42"]
	if ttl != 30*time.Minute {
		t.Fatalf("expected ttl from highest priority provider, got %s", ttl)
	}
}

func TestPipelineForceRefreshBypassesCache(t *testing.T) {
	cache := newStubCache()
	stale := Metadata{
		EntityID:       "artist-5",
		EntityType:     EntityTypeArtist,
		CanonicalTitle: "Stale Artist",
	}
	cache.store["metadata:artist:artist-5"] = stale

	clock := fakeClock{now: time.Date(2024, 5, 5, 12, 0, 0, 0, time.UTC)}
	provider := &stubProvider{
		name: "discogs",
		resp: ProviderResponse{
			Metadata: Metadata{CanonicalTitle: "Fresh Artist"},
		},
	}

	pipeline, err := NewPipeline(PipelineConfig{
		Cache:      cache,
		Providers:  []Provider{provider},
		DefaultTTL: time.Hour,
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("NewPipeline() error = %v", err)
	}

	req := Request{EntityType: EntityTypeArtist, EntityID: "artist-5", ForceRefresh: true}
	result, err := pipeline.Ingest(context.Background(), req)
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	if result.CacheHit {
		t.Fatalf("expected forced refresh to bypass cache")
	}
	if provider.calls == 0 {
		t.Fatalf("expected provider to be invoked during forced refresh")
	}
	if result.Metadata.CanonicalTitle != "Fresh Artist" {
		t.Fatalf("expected fresh metadata, got %s", result.Metadata.CanonicalTitle)
	}
}

func TestPipelineReturnsErrorWhenNoData(t *testing.T) {
	cache := newStubCache()
	clock := fakeClock{now: time.Date(2024, 6, 1, 15, 0, 0, 0, time.UTC)}

	provider := &stubProvider{
		name: "empty",
		resp: ProviderResponse{
			Metadata: Metadata{},
		},
	}

	pipeline, err := NewPipeline(PipelineConfig{
		Cache:      cache,
		Providers:  []Provider{provider},
		DefaultTTL: time.Hour,
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("NewPipeline() error = %v", err)
	}

	req := Request{EntityType: EntityTypeSeries, EntityID: "missing"}
	_, err = pipeline.Ingest(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error when no metadata returned")
	}
	if err != ErrMetadataNotFound {
		t.Fatalf("unexpected error: %v", err)
	}
	if cache.saveCount != 0 {
		t.Fatalf("expected no cache writes on failure")
	}
}

// --- Test fixtures ---

type stubProvider struct {
	name      string
	resp      ProviderResponse
	err       error
	calls     int
	supported map[EntityType]bool
}

func (s *stubProvider) Name() string { return s.name }

func (s *stubProvider) Supports(entityType EntityType) bool {
	if len(s.supported) == 0 {
		return true
	}
	return s.supported[entityType]
}

func (s *stubProvider) Fetch(ctx context.Context, req Request) (ProviderResponse, error) {
	s.calls++
	if s.err != nil {
		return ProviderResponse{}, s.err
	}
	return s.resp, nil
}

type stubCache struct {
	store      map[string]Metadata
	ttl        map[string]time.Duration
	fetchErr   error
	saveErr    error
	fetchCount int
	saveCount  int
}

func newStubCache() *stubCache {
	return &stubCache{
		store: make(map[string]Metadata),
		ttl:   make(map[string]time.Duration),
	}
}

func (s *stubCache) Fetch(ctx context.Context, key string) (Metadata, bool, error) {
	if s.fetchErr != nil {
		return Metadata{}, false, s.fetchErr
	}
	s.fetchCount++
	value, ok := s.store[key]
	if !ok {
		return Metadata{}, false, nil
	}
	return value, true, nil
}

func (s *stubCache) Save(ctx context.Context, key string, value Metadata, ttl time.Duration) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saveCount++
	s.store[key] = value
	s.ttl[key] = ttl
	return nil
}

type fakeClock struct {
	now time.Time
}

func (f fakeClock) Now() time.Time { return f.now }
