package metadata

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

// PipelineConfig configures a metadata ingestion pipeline instance.
type PipelineConfig struct {
	Cache      Cache
	Providers  []Provider
	DefaultTTL time.Duration
	Logger     *slog.Logger
	Clock      Clock
}

// Pipeline coordinates metadata ingestion across providers with Redis caching.
type Pipeline struct {
	cache      Cache
	providers  []Provider
	defaultTTL time.Duration
	logger     *slog.Logger
	clock      Clock
}

// Clock abstracts time retrieval for deterministic testing.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// NewPipeline constructs a metadata ingestion pipeline.
func NewPipeline(cfg PipelineConfig) (*Pipeline, error) {
	if len(cfg.Providers) == 0 {
		return nil, ErrNoProviders
	}

	p := &Pipeline{
		cache:      cfg.Cache,
		providers:  make([]Provider, 0, len(cfg.Providers)),
		defaultTTL: cfg.DefaultTTL,
		logger:     cfg.Logger,
		clock:      cfg.Clock,
	}

	if p.defaultTTL <= 0 {
		p.defaultTTL = time.Hour
	}
	if p.logger == nil {
		p.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if p.clock == nil {
		p.clock = systemClock{}
	}

	for _, provider := range cfg.Providers {
		if provider != nil {
			p.providers = append(p.providers, provider)
		}
	}
	if len(p.providers) == 0 {
		return nil, ErrNoProviders
	}

	return p, nil
}

// Result captures the outcome of an ingestion attempt.
type Result struct {
	Metadata        Metadata
	CacheHit        bool
	CacheKey        string
	ProviderReports []ProviderReport
}

// ProviderStatus conveys the outcome of a provider fetch attempt.
type ProviderStatus string

const (
	ProviderStatusFetched ProviderStatus = "fetched"
	ProviderStatusCached  ProviderStatus = "cached"
	ProviderStatusSkipped ProviderStatus = "skipped"
	ProviderStatusError   ProviderStatus = "error"
)

// ProviderReport records execution details for a provider call.
type ProviderReport struct {
	Name     string
	Status   ProviderStatus
	Duration time.Duration
	Priority int
	Error    string
	Used     bool
}

// Ingest executes metadata retrieval and caching.
func (p *Pipeline) Ingest(ctx context.Context, req Request) (Result, error) {
	if err := req.Validate(); err != nil {
		return Result{}, err
	}

	cacheKey := req.CacheKey()
	if !req.ForceRefresh && p.cache != nil {
		cached, ok, err := p.cache.Fetch(ctx, cacheKey)
		if err != nil {
			return Result{}, fmt.Errorf("metadata: cache fetch: %w", err)
		}
		if ok {
			p.logger.Debug("metadata cache hit", "cacheKey", cacheKey, "entityType", req.EntityType, "entityID", req.EntityID)
			return Result{
				Metadata: cached,
				CacheHit: true,
				CacheKey: cacheKey,
				ProviderReports: []ProviderReport{{
					Name:   "cache",
					Status: ProviderStatusCached,
					Used:   true,
				}},
			}, nil
		}
	}

	agg := newAggregator(req)

	type providerOutcome struct {
		name     string
		status   ProviderStatus
		resp     ProviderResponse
		err      error
		duration time.Duration
	}

	outcomes := make([]providerOutcome, 0, len(p.providers))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, provider := range p.providers {
		if provider == nil {
			mu.Lock()
			outcomes = append(outcomes, providerOutcome{
				name:   "",
				status: ProviderStatusSkipped,
			})
			mu.Unlock()
			continue
		}

		if !provider.Supports(req.EntityType) {
			mu.Lock()
			outcomes = append(outcomes, providerOutcome{
				name:   provider.Name(),
				status: ProviderStatusSkipped,
			})
			mu.Unlock()
			continue
		}

		wg.Add(1)
		go func(pr Provider) {
			defer wg.Done()

			start := time.Now()
			resp, err := pr.Fetch(ctx, req)
			duration := time.Since(start)

			outcome := providerOutcome{
				name:     pr.Name(),
				duration: duration,
			}

			if err != nil {
				outcome.status = ProviderStatusError
				outcome.err = err
			} else {
				outcome.status = ProviderStatusFetched
				if resp.RetrievedAt.IsZero() {
					resp.RetrievedAt = p.clock.Now()
				}
				outcome.resp = resp
			}

			mu.Lock()
			outcomes = append(outcomes, outcome)
			mu.Unlock()
		}(provider)
	}

	wg.Wait()

	reports := make([]ProviderReport, 0, len(outcomes))
	var ttl time.Duration

	for _, outcome := range outcomes {
		report := ProviderReport{
			Name:     outcome.name,
			Status:   outcome.status,
			Duration: outcome.duration,
		}

		switch outcome.status {
		case ProviderStatusFetched:
			report.Priority = outcome.resp.Priority
			used := agg.merge(outcome.resp, outcome.name)
			report.Used = used
			if outcome.resp.TTL > 0 {
				if ttl == 0 || outcome.resp.TTL < ttl {
					ttl = outcome.resp.TTL
				}
			}
		case ProviderStatusError:
			if outcome.err != nil {
				report.Error = outcome.err.Error()
			}
		}

		reports = append(reports, report)
	}

	now := p.clock.Now()
	metadata, ok := agg.finalize(now)
	if !ok {
		p.logger.Warn("metadata ingestion produced no results", "cacheKey", cacheKey, "entityType", req.EntityType, "entityID", req.EntityID)
		return Result{CacheKey: cacheKey, ProviderReports: reports}, ErrMetadataNotFound
	}

	result := Result{
		Metadata:        metadata,
		CacheHit:        false,
		CacheKey:        cacheKey,
		ProviderReports: reports,
	}

	effectiveTTL := ttl
	if effectiveTTL <= 0 {
		effectiveTTL = p.defaultTTL
	}

	if p.cache != nil {
		if err := p.cache.Save(ctx, cacheKey, metadata, effectiveTTL); err != nil {
			return Result{}, fmt.Errorf("metadata: cache save: %w", err)
		}
	}

	p.logger.Info("metadata ingested", "cacheKey", cacheKey, "entityType", req.EntityType, "entityID", metadata.EntityID, "ttl", effectiveTTL)

	return result, nil
}

// aggregator combines provider responses into a single metadata document.
type aggregator struct {
	request Request

	metadata Metadata

	fieldPriority       map[string]int
	releaseDatePriority int
	releaseDateSet      bool
	runtimePriority     int
	runtimeSet          bool

	altTitles   []string
	altTitleSet map[string]struct{}

	genres   []string
	genreSet map[string]struct{}

	tags   []string
	tagSet map[string]struct{}

	externalIDs        map[string]string
	externalIDPriority map[string]int

	attributions map[string]map[string]struct{}
	retrievedAt  map[string]time.Time
	dataPoints   int
}

func newAggregator(req Request) *aggregator {
	metadata := Metadata{
		EntityID:    req.EntityID,
		EntityType:  req.EntityType,
		ExternalIDs: make(map[string]string, len(req.ExternalIDs)),
	}

	for provider, value := range req.ExternalIDs {
		metadata.ExternalIDs[strings.ToLower(provider)] = value
	}

	return &aggregator{
		request:            req,
		metadata:           metadata,
		fieldPriority:      make(map[string]int),
		altTitleSet:        make(map[string]struct{}),
		genreSet:           make(map[string]struct{}),
		tagSet:             make(map[string]struct{}),
		externalIDs:        make(map[string]string),
		externalIDPriority: make(map[string]int),
		attributions:       make(map[string]map[string]struct{}),
		retrievedAt:        make(map[string]time.Time),
	}
}

func (a *aggregator) merge(resp ProviderResponse, provider string) bool {
	fieldsUsed := false
	metadata := resp.Metadata
	priority := resp.Priority
	retrievedAt := resp.RetrievedAt

	if metadata.EntityID != "" && a.metadata.EntityID == "" {
		a.metadata.EntityID = metadata.EntityID
	}

	fieldsUsed = a.applyStringField("canonical_title", metadata.CanonicalTitle, priority, provider, retrievedAt) || fieldsUsed
	fieldsUsed = a.applyStringField("sort_title", metadata.SortTitle, priority, provider, retrievedAt) || fieldsUsed
	fieldsUsed = a.applyStringField("synopsis", metadata.Synopsis, priority, provider, retrievedAt) || fieldsUsed
	fieldsUsed = a.applyStringField("artwork_url", metadata.ArtworkURL, priority, provider, retrievedAt) || fieldsUsed

	if metadata.RuntimeMinutes > 0 {
		if !a.runtimeSet || priority < a.runtimePriority {
			a.metadata.RuntimeMinutes = metadata.RuntimeMinutes
			a.runtimePriority = priority
			a.runtimeSet = true
			a.recordAttribution(provider, "runtime_minutes", retrievedAt)
			fieldsUsed = true
		}
	}

	if metadata.ReleaseDate != nil {
		if !a.releaseDateSet || priority < a.releaseDatePriority {
			dateCopy := *metadata.ReleaseDate
			a.metadata.ReleaseDate = &dateCopy
			a.releaseDatePriority = priority
			a.releaseDateSet = true
			a.recordAttribution(provider, "release_date", retrievedAt)
			fieldsUsed = true
		}
	}

	for _, title := range metadata.AlternateTitles {
		if title == "" {
			continue
		}
		normalized := strings.ToLower(title)
		if _, exists := a.altTitleSet[normalized]; !exists {
			a.altTitleSet[normalized] = struct{}{}
			a.altTitles = append(a.altTitles, title)
			a.recordAttribution(provider, "alternate_titles", retrievedAt)
			fieldsUsed = true
		}
	}

	for _, genre := range metadata.Genres {
		if genre == "" {
			continue
		}
		normalized := strings.ToLower(genre)
		if _, exists := a.genreSet[normalized]; !exists {
			a.genreSet[normalized] = struct{}{}
			a.genres = append(a.genres, genre)
			a.recordAttribution(provider, "genres", retrievedAt)
			fieldsUsed = true
		}
	}

	for _, tag := range metadata.Tags {
		if tag == "" {
			continue
		}
		normalized := strings.ToLower(tag)
		if _, exists := a.tagSet[normalized]; !exists {
			a.tagSet[normalized] = struct{}{}
			a.tags = append(a.tags, tag)
			a.recordAttribution(provider, "tags", retrievedAt)
			fieldsUsed = true
		}
	}

	for providerKey, identifier := range metadata.ExternalIDs {
		if identifier == "" {
			continue
		}
		key := strings.ToLower(providerKey)
		currentPriority, ok := a.externalIDPriority[key]
		if !ok || priority < currentPriority {
			a.externalIDs[key] = identifier
			a.externalIDPriority[key] = priority
			a.recordAttribution(provider, fmt.Sprintf("external_ids.%s", key), retrievedAt)
			fieldsUsed = true
		}
	}

	if fieldsUsed {
		a.dataPoints++
	}

	return fieldsUsed
}

func (a *aggregator) applyStringField(field string, value string, priority int, provider string, retrievedAt time.Time) bool {
	if value == "" {
		return false
	}

	currentPriority, ok := a.fieldPriority[field]
	if ok && priority >= currentPriority {
		return false
	}

	switch field {
	case "canonical_title":
		a.metadata.CanonicalTitle = value
	case "sort_title":
		a.metadata.SortTitle = value
	case "synopsis":
		a.metadata.Synopsis = value
	case "artwork_url":
		a.metadata.ArtworkURL = value
	}

	a.fieldPriority[field] = priority
	a.recordAttribution(provider, field, retrievedAt)
	return true
}

func (a *aggregator) recordAttribution(provider string, field string, retrievedAt time.Time) {
	if provider == "" || field == "" {
		return
	}
	fields, ok := a.attributions[provider]
	if !ok {
		fields = make(map[string]struct{})
		a.attributions[provider] = fields
	}
	fields[field] = struct{}{}
	if !retrievedAt.IsZero() {
		if existing, ok := a.retrievedAt[provider]; !ok || retrievedAt.Before(existing) {
			a.retrievedAt[provider] = retrievedAt
		}
	}
}

func (a *aggregator) finalize(now time.Time) (Metadata, bool) {
	if a.metadata.EntityID == "" {
		a.metadata.EntityID = a.request.EntityID
	}
	if a.metadata.EntityType == "" {
		a.metadata.EntityType = a.request.EntityType
	}
	if len(a.metadata.ExternalIDs) == 0 {
		a.metadata.ExternalIDs = make(map[string]string)
	}
	for key, value := range a.externalIDs {
		a.metadata.ExternalIDs[key] = value
	}
	if len(a.altTitles) > 0 {
		a.metadata.AlternateTitles = append([]string(nil), a.altTitles...)
	}
	if len(a.genres) > 0 {
		a.metadata.Genres = append([]string(nil), a.genres...)
	}
	if len(a.tags) > 0 {
		a.metadata.Tags = append([]string(nil), a.tags...)
	}

	if a.dataPoints == 0 && a.metadata.CanonicalTitle == "" && a.metadata.Synopsis == "" && len(a.metadata.ExternalIDs) == len(a.request.ExternalIDs) {
		return Metadata{}, false
	}

	if len(a.attributions) > 0 {
		attributions := make([]SourceAttribution, 0, len(a.attributions))
		for provider, fields := range a.attributions {
			providerFields := make([]string, 0, len(fields))
			for field := range fields {
				providerFields = append(providerFields, field)
			}
			sortStrings(providerFields)
			attributions = append(attributions, SourceAttribution{
				Provider:    provider,
				Fields:      providerFields,
				RetrievedAt: a.providerRetrievedAt(provider, now),
			})
		}
		sort.Slice(attributions, func(i, j int) bool {
			return attributions[i].Provider < attributions[j].Provider
		})
		a.metadata.SourceAttribution = attributions
	}

	a.metadata.LastRefreshed = now

	return a.metadata, true
}

func (a *aggregator) providerRetrievedAt(provider string, fallback time.Time) time.Time {
	if ts, ok := a.retrievedAt[provider]; ok && !ts.IsZero() {
		return ts
	}
	return fallback
}
