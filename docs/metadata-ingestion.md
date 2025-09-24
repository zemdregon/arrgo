# Metadata Ingestion Pipeline

> **Last reviewed:** 2025-03-08  
> **Status:** Implemented with Redis-backed caching and provider fan-out

The metadata ingestion pipeline coordinates upstream providers, aggregates
records into the unified arrgo domain model, and persists normalized results in
Redis for fast reuse. It powers the first step in onboarding media entities into
the arrgo catalog service.

## Responsibilities

- **Provider orchestration:** Fan out requests to configured metadata providers
  (TMDB, MusicBrainz, etc.) and combine their responses deterministically.
- **Normalization:** Merge titles, summaries, genres, tags, and external IDs into
  the canonical schema defined in [`docs/domain-model.md`](domain-model.md).
- **Caching:** Store normalized metadata in Redis using stable cache keys so
  subsequent workers can reuse data without re-contacting external APIs.
- **Attribution:** Track which provider contributed each field to maintain
  provenance for debugging and data quality reviews.

## Implementation Summary

The pipeline lives under [`internal/metadata`](../internal/metadata) and is
constructed with the following components:

| Component | Responsibility |
| --- | --- |
| `Provider` interface | Encapsulates a metadata source and exposes `Fetch` plus `Supports` guards. |
| `Pipeline` | Coordinates cache lookups, provider fan-out, aggregation, and persistence. |
| `RedisCache` | Implements the `Cache` interface using `github.com/redis/go-redis/v9`. |
| `aggregator` | Reconciles provider responses using priority rules and deduplicates arrays. |

### Execution Flow

1. **Validate request:** Ensures an entity type and identifier (internal or
   external) are present.
2. **Cache lookup:** Unless `ForceRefresh` is set, the pipeline attempts to read
   a cached `Metadata` document using a deterministic key composed of the entity
   type and canonical identifier.
3. **Provider fan-out:** For cache misses, providers run concurrently. The
   pipeline records duration, errors, and response priorities for observability.
4. **Aggregation:** Provider responses merge into a unified `Metadata` document.
   The aggregator favours lower priority numbers (higher authority), deduplicates
   collections, and preserves provider attributions.
5. **Persistence:** The normalized document is written back to Redis with the
   tightest TTL returned by any provider (or a default) and emitted to callers.

The pipeline only returns `ErrMetadataNotFound` when all providers fail to
produce usable fields, preventing downstream components from acting on empty
records while still surfacing partial failures via provider reports.

## Cache Keys and TTLs

Cache keys follow the pattern `metadata:<entity_type>:<identifier>` and fall back
on sorted external IDs when an arrgo identifier is not yet assigned. Provider
responses can override the default TTL to reflect upstream freshness windows.
The minimum non-zero TTL across providers is used for the cached document.

## Testing Strategy

Unit tests (`internal/metadata/pipeline_test.go`) cover the following behavior:

- Returning cached metadata without invoking providers.
- Fetching from providers, writing normalized results to the cache, and tracking
  provider usage.
- Merging multi-provider responses, including priority overrides and
  attribution.
- Respecting forced refresh requests and bypassing cached data.
- Surfacing `ErrMetadataNotFound` when no provider yields usable metadata.

These tests run with stub implementations of both providers and the cache to
avoid external dependencies while validating the orchestration logic.

## Observability & Operations
- Emit Prometheus metrics for cache hit ratio, provider latency, and error
  counts. Alert when a provider experiences sustained failures or when cache hit
  rates drop unexpectedly.
- Annotate traces with provider names, response sizes, and priority decisions to
  aid debugging when metadata conflicts occur.
- Log provenance details (provider, field, winning value) whenever aggregation
  resolves conflicting attributes.

## Future Enhancements
- Persist normalized metadata in Postgres to support auditing and reduce cache
  warm-up times after restarts.
- Introduce schema validation to guard against upstream changes before they
  cascade through the pipeline.
- Support provider-specific throttling strategies that adapt to quota limits and
  burst allowances automatically.
