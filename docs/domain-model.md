# Unified Media Domain Model

## Purpose
This document finalizes the foundational entity model that arrgo uses to unify
television, movie, and music catalog management. It establishes the canonical
records that power ingestion, metadata enrichment, automation policies, and
library presentation across the platform. The model is intentionally backend-
centric so it can back both REST and gRPC APIs, master/worker workflows, and
future UI implementations.

## Modeling Principles
- **Single source of truth:** Every distinct piece of media has exactly one
  canonical entity in PostgreSQL, with Redis caches derived from these records.
- **Work vs. release separation:** Intellectual properties ("works") are stored
  independently from specific audio/video releases. This supports deduplication
  across regions, cuts, codecs, and masters.
- **Explicit relationships:** Junction tables model many-to-many relationships
  (e.g., artists ↔ tracks) to keep queries and synchronization deterministic.
- **Traceable provenance:** External identifiers from upstream metadata
  providers remain attached so that re-ingestion and conflict resolution are
  idempotent.
- **Extensible typing:** Enumerations and JSONB attributes are used sparingly so
  that new media types or release variants can be introduced without
  destructive migrations.

## Core Entities

### CatalogEntity (abstract)
All top-level media entities embed a shared set of attributes via the
`catalog_entities` table:

| Column | Type | Notes |
| --- | --- | --- |
| `id` | UUID | Primary key. Stable across the cluster. |
| `entity_type` | enum (`series`, `season`, `episode`, `movie`, `artist`, `album`, `track`, `collection`) | Drives polymorphism. |
| `canonical_title` | text | User-visible title preferred by arrgo. |
| `sort_title` | text | Normalized title for consistent ordering. |
| `synopsis` | text | Long-form description. |
| `release_date` | date | Canonical premiere date. |
| `original_language` | text | ISO-639-1 code. |
| `status` | enum (`announced`, `released`, `ended`, `cancelled`) | Lifecycle state. |
| `genres` | text[] | High-level genres. |
| `tags` | text[] | User-managed tags and automation labels. |
| `external_ids` | jsonb | Map of provider → identifier (TMDB, MusicBrainz, etc.). |
| `primary_artwork_id` | UUID | FK to `artwork_assets`. |
| `created_at` / `updated_at` | timestamptz | Audit metadata. |

### Hierarchical Works

#### Series
- **Table:** `series`
- **Extends:** `CatalogEntity`
- **Key attributes:** `total_seasons`, `episode_runtime_min`, `origin_network`,
  `air_schedule` (cron-like string), `official_site`, `rating_certificate`.
- **Relationships:**
  - 1:N with `Season` via `series_id`.
  - M:N with `Person` (cast/crew) via `series_people`.
  - M:N with `Collection` via `collection_members`.

#### Season
- **Table:** `seasons`
- **Key attributes:** `season_number`, `title_variant`, `release_year`,
  `summary`, `poster_art_id`.
- **Relationships:**
  - Belongs to `Series`.
  - 1:N with `Episode`.

#### Episode
- **Table:** `episodes`
- **Key attributes:** `season_id`, `episode_number`, `absolute_number`,
  `runtime_min`, `air_date`, `production_code`, `sequence_overrides` (JSONB to
  handle split/double episodes).
- **Relationships:**
  - N:M with `Person` via `episode_people`.
  - 1:N with `MediaRelease` for specific cuts or encodes.

#### Movie
- **Table:** `movies`
- **Key attributes:** `runtime_min`, `content_rating`, `production_status`,
  `budget`, `revenue`, `franchise`, `box_office_region_breakdown` (JSONB).
- **Relationships:**
  - M:N with `Collection` (box sets, sagas).
  - 1:N with `MediaRelease`.

### Music Works

#### Artist
- **Table:** `artists`
- **Key attributes:** `artist_type` enum (`person`, `group`), `formed_year`,
  `disbanded_year`, `origin_location`, `aliases`, `primary_genres`,
  `biography`.
- **Relationships:**
  - 1:N with `Album`.
  - M:N with `Collection` (festivals, curated playlists).
  - M:N with `Person` for individual contributors.

#### Album
- **Table:** `albums`
- **Key attributes:** `artist_id`, `album_type` enum (`studio`, `live`, `ep`,
  `single`, `compilation`), `label`, `release_status` (`announced`, `released`),
  `barcode`, `musicbrainz_release_group_id`.
- **Relationships:**
  - 1:N with `Track`.
  - 1:N with `MediaRelease` (vinyl vs. digital editions).

#### Track
- **Table:** `tracks`
- **Key attributes:** `album_id`, `disc_number`, `track_number`, `title`,
  `duration_ms`, `explicit`, `isrc`, `lyrics`, `preview_url`.
- **Relationships:**
  - M:N with `Artist` (featured artists).
  - N:1 with `Album`.
  - 1:N with `MediaRelease` (remasters, live versions).

### Cross-Cutting Entities

#### MediaRelease
Represents a concrete, ingestible manifestation of a work (episode, movie,
track). Stores information required for download/import automation.

| Column | Type | Notes |
| --- | --- | --- |
| `id` | UUID | Primary key. |
| `work_type` | enum (`episode`, `movie`, `track`) | Owner reference. |
| `work_id` | UUID | FK to the owning work. |
| `release_title` | text | e.g., `Director's Cut`, `24-bit Remaster`. |
| `edition` | enum (`standard`, `collector`, `live`, `remaster`, `theatrical`) |
| `quality_profile_id` | UUID | FK to automation quality policies. |
| `source` | enum (`web`, `bluray`, `dvd`, `cd`, `vinyl`, `broadcast`, `cam`) |
| `resolution` | enum (`1080p`, `4k`, `sd`, `uhd`, `lossless`, `mp3-320`, etc.) |
| `audio_channels` | text | e.g., `5.1`, `7.1`, `stereo`. |
| `audio_format` | text | e.g., `DTS-HD MA`, `FLAC`. |
| `release_date` | date | Publication date of this release. |
| `external_ids` | jsonb | Upstream release identifiers. |
| `checksum` | text | Optional uniqueness hash for dedupe. |

A `MediaRelease` may be linked to one or more `LibraryItem` records after the
platform imports actual media files.

#### LibraryItem
- Stores the concrete file(s) and their storage metadata.
- Attributes: `id`, `release_id`, `storage_path`, `size_bytes`, `duration_ms`,
  `bit_rate`, `video_codec`, `audio_codec`, `subtitle_tracks` (JSONB),
  `media_hash`, `availability` (`present`, `missing`, `deleted`), `last_seen_at`.
- Maintains references to worker jobs responsible for imports or health checks.

#### Person
- Used for cast, crew, and contributor credits across any media type.
- Attributes: `id`, `name`, `birth_date`, `death_date`, `biography`,
  `external_ids`, `roles` (JSONB array of credit descriptors).
- Relationship tables (`series_people`, `movie_people`, `track_people`, etc.)
  track role-specific metadata like `character_name`, `instrument`, or `job`.

#### Collection
- Arbitrary grouping of catalog entities (e.g., cinematic universe, anthology,
  artist compilations).
- Attributes: `id`, `title`, `description`, `collection_type` enum
  (`franchise`, `curated`, `holiday`, `user_defined`).
- Membership stored in `collection_members` with `entity_type` + `entity_id`.

#### ArtworkAsset
- Stores posters, backdrops, logos, etc.
- Attributes: `id`, `kind` (`poster`, `backdrop`, `logo`, `banner`),
  `image_path`, `source`, `language`, `color_palette`, `width`, `height`,
  `average_luminance`.
- Linked via `primary_artwork_id` on `CatalogEntity` and optional join tables for
  alternates.

#### ExternalReference
- Normalizes third-party IDs to accelerate lookups.
- Attributes: `id`, `provider`, `external_id`, `entity_type`, `entity_id`,
  `confidence`, `fetched_at`.
- Allows multiple references per entity while keeping JSONB payloads slim.

## Relationship Diagram (Textual)
```
Collection 1---* CollectionMember *---1 CatalogEntity
CatalogEntity (Series) 1---* Season 1---* Episode 1---* MediaRelease 1---* LibraryItem
CatalogEntity (Movie)  1---* MediaRelease 1---* LibraryItem
CatalogEntity (Artist) 1---* Album 1---* Track 1---* MediaRelease 1---* LibraryItem
CatalogEntity (Track)  *---* Artist (featured) via TrackArtistRole
CatalogEntity (Series/Movie/Episode) *---* Person via Credit tables
CatalogEntity (Artist/Album/Track)   *---* Person via Credit tables
```

## Automation Touchpoints
- **Quality and retention policies** target `MediaRelease` and `LibraryItem`
  attributes to drive upgrade decisions.
- **Ingest jobs** produce or update `MediaRelease` records before enqueuing file
  imports against worker pools.
- **Health monitors** attach to `LibraryItem` to detect missing or corrupted
  files.
- **User actions** (manual search, re-tagging, overrides) operate primarily on
  `CatalogEntity` while referencing `LibraryItem` for actual files.

## API & Event Considerations
- REST/gRPC endpoints should expose unified filters across entity types, using
  `entity_type` and `external_ids` for polymorphic queries.
- Change data capture events emit a `DomainEvent` envelope with `entity_type`,
  `entity_id`, `event_type`, and a JSON payload describing field diffs.
- Workers subscribe to events for `MediaRelease` and `LibraryItem` to trigger
  download/import workflows.

## Migration Notes
- Existing Sonarr/Radarr/Lidarr libraries map into `Series`, `Movie`, and
  `Artist` hierarchies. Releases and files become `MediaRelease` and
  `LibraryItem` rows.
- Deduplication should leverage `external_ids` + `checksum` to prevent duplicate
  works when importing from multiple sources.
- Future enhancements (e.g., books, podcasts) can introduce new `entity_type`
  values while reusing `MediaRelease` and `LibraryItem` patterns.
