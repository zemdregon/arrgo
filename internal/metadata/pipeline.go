package metadata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// PipelineConfig configures a metadata ingestion pipeline instance.
type PipelineConfig struct {
	Cache            Cache
	Providers        []Provider
	DefaultTTL       time.Duration
	Logger           *slog.Logger
	Clock            Clock
	MaxAttempts      int
	BaseBackoff      time.Duration
	MaxBackoff       time.Duration
	NegativeCacheTTL time.Duration
	ProviderTTLs     map[string]time.Duration
	Metrics          *Metrics
	Tracer           trace.Tracer
}

// Pipeline coordinates metadata ingestion across providers with Redis caching.
type Pipeline struct {
	cache            Cache
	providers        []Provider
	defaultTTL       time.Duration
	logger           *slog.Logger
	clock            Clock
	maxAttempts      int
	baseBackoff      time.Duration
	maxBackoff       time.Duration
	negativeCacheTTL time.Duration
	providerTTLs     map[string]time.Duration
	metrics          *Metrics
	tracer           trace.Tracer
}

type providerOutcome struct {
	name     string
	status   ProviderStatus
	resp     ProviderResponse
	err      error
	duration time.Duration
	attempts int
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
		cache:            cfg.Cache,
		providers:        make([]Provider, 0, len(cfg.Providers)),
		defaultTTL:       cfg.DefaultTTL,
		logger:           cfg.Logger,
		clock:            cfg.Clock,
		maxAttempts:      cfg.MaxAttempts,
		baseBackoff:      cfg.BaseBackoff,
		maxBackoff:       cfg.MaxBackoff,
		negativeCacheTTL: cfg.NegativeCacheTTL,
		providerTTLs:     make(map[string]time.Duration, len(cfg.ProviderTTLs)),
		metrics:          cfg.Metrics,
		tracer:           cfg.Tracer,
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
	if p.maxAttempts <= 0 {
		p.maxAttempts = 3
	}
	if p.baseBackoff <= 0 {
		p.baseBackoff = 500 * time.Millisecond
	}
	if p.maxBackoff <= 0 {
		p.maxBackoff = 5 * time.Second
	}
	if p.negativeCacheTTL < 0 {
		p.negativeCacheTTL = 0
	}
	if p.negativeCacheTTL == 0 {
		p.negativeCacheTTL = 10 * time.Minute
	}
	if p.tracer == nil {
		p.tracer = trace.NewNoopTracerProvider().Tracer("github.com/arrgo/arrgo/internal/metadata")
	}

	for name, ttl := range cfg.ProviderTTLs {
		if ttl > 0 {
			p.providerTTLs[strings.ToLower(name)] = ttl
		}
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
			if cached.NotFound {
				p.logger.Debug("metadata negative cache hit", "cacheKey", cacheKey, "entityType", req.EntityType, "entityID", req.EntityID)
				return Result{
					CacheHit: true,
					CacheKey: cacheKey,
					ProviderReports: []ProviderReport{{
						Name:   "cache",
						Status: ProviderStatusCached,
					}},
				}, ErrMetadataNotFound
			}
			p.logger.Debug("metadata cache hit", "cacheKey", cacheKey, "entityType", req.EntityType, "entityID", req.EntityID)
			cached.NotFound = false
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
			outcome := p.runProvider(ctx, pr, req)
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
			if used {
				providerTTL := p.providerTTL(outcome.name, outcome.resp.TTL)
				if providerTTL > 0 {
					if ttl == 0 || providerTTL < ttl {
						ttl = providerTTL
					}
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
		if p.cache != nil && p.negativeCacheTTL > 0 {
			negative := Metadata{
				EntityID:      req.EntityID,
				EntityType:    req.EntityType,
				LastRefreshed: now,
				NotFound:      true,
			}
			if len(req.ExternalIDs) > 0 {
				negative.ExternalIDs = make(map[string]string, len(req.ExternalIDs))
				for provider, value := range req.ExternalIDs {
					negative.ExternalIDs[strings.ToLower(provider)] = value
				}
			}
			if err := p.cache.Save(ctx, cacheKey, negative, p.negativeCacheTTL); err != nil {
				p.logger.Warn("metadata negative cache save failed", "cacheKey", cacheKey, "err", err)
			}
		}
		return Result{CacheKey: cacheKey, ProviderReports: reports}, ErrMetadataNotFound
	}

	metadata.NotFound = false

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

func (p *Pipeline) runProvider(ctx context.Context, pr Provider, req Request) providerOutcome {
	outcome := providerOutcome{name: pr.Name()}

	attempts := 0
	var lastErr error
	var duration time.Duration

forLoop:
	for attempts < p.maxAttempts {
		attempts++

		attemptCtx, span := p.tracer.Start(ctx, "metadata.provider.fetch",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("metadata.provider", pr.Name()),
				attribute.String("metadata.entity_type", string(req.EntityType)),
				attribute.String("metadata.entity_id", req.EntityID),
				attribute.Bool("metadata.force_refresh", req.ForceRefresh),
				attribute.Int("metadata.attempt", attempts),
			),
		)

		start := time.Now()
		resp, err := pr.Fetch(attemptCtx, req)
		attemptDuration := time.Since(start)
		duration += attemptDuration

		var retryAfter time.Duration
		rateLimited := false

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "provider fetch failed")
			if rl, ok := AsRateLimitError(err); ok {
				rateLimited = true
				retryAfter = rl.RetryAfter()
				span.AddEvent("rate_limited", trace.WithAttributes(
					attribute.String("metadata.retry_after", retryAfter.String()),
				))
			}
		} else {
			span.SetStatus(codes.Ok, "provider fetch succeeded")
		}
		span.End()

		if err == nil {
			if resp.RetrievedAt.IsZero() {
				resp.RetrievedAt = p.clock.Now()
			}
			outcome.status = ProviderStatusFetched
			outcome.resp = resp
			outcome.duration = duration
			outcome.attempts = attempts
			return outcome
		}

		lastErr = err

		if ctxErr := ctx.Err(); ctxErr != nil {
			lastErr = ctxErr
			break
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			break
		}

		if rateLimited && attempts < p.maxAttempts {
			if p.metrics != nil {
				p.metrics.RateLimitRetries().WithLabelValues(pr.Name()).Inc()
			}
			backoff := p.computeBackoff(attempts, retryAfter)
			if p.logger != nil {
				p.logger.Warn("metadata provider rate limited", "provider", pr.Name(), "attempt", attempts, "backoff", backoff, "retryAfter", retryAfter)
			}
			if backoff > 0 {
				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					if !timer.Stop() {
						<-timer.C
					}
					lastErr = ctx.Err()
					break forLoop
				case <-timer.C:
				}
			}
			continue
		}

		break
	}

	outcome.status = ProviderStatusError
	outcome.err = lastErr
	outcome.duration = duration
	outcome.attempts = attempts

	if lastErr != nil && p.metrics != nil {
		reason := failureReason(lastErr)
		p.metrics.ProviderFailures().WithLabelValues(pr.Name(), reason).Inc()
	}

	if lastErr != nil && p.logger != nil {
		reason := failureReason(lastErr)
		if reason == "context" {
			p.logger.Debug("metadata provider context ended", "provider", pr.Name(), "attempts", attempts, "err", lastErr)
		} else {
			p.logger.Warn("metadata provider fetch failed", "provider", pr.Name(), "attempts", attempts, "err", lastErr)
		}
	}

	return outcome
}

func (p *Pipeline) providerTTL(provider string, reported time.Duration) time.Duration {
	if provider == "" {
		if reported <= 0 {
			return p.defaultTTL
		}
		return reported
	}

	ttl := reported
	if override, ok := p.providerTTLs[strings.ToLower(provider)]; ok && override > 0 {
		if ttl <= 0 || override < ttl {
			ttl = override
		}
	}
	if ttl <= 0 {
		ttl = p.defaultTTL
	}
	return ttl
}

func (p *Pipeline) computeBackoff(attempt int, retryAfter time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	multiplier := 1 << uint(attempt-1)
	backoff := p.baseBackoff * time.Duration(multiplier)
	if retryAfter > backoff {
		backoff = retryAfter
	}
	if p.maxBackoff > 0 && backoff > p.maxBackoff {
		backoff = p.maxBackoff
	}
	return backoff
}

func failureReason(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "context"
	}
	if _, ok := AsRateLimitError(err); ok {
		return "rate_limit"
	}
	return "error"
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
