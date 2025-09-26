package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Migration struct {
	ID          string
	Description string
	Up          string
	Down        string
}

var migrations = []Migration{
	catalogSchemaMigration,
	jobHistoryMigration,
	demoSeedMigration,
}

// Run applies all pending migrations in order. It creates the schema_migrations
// bookkeeping table when absent and validates checksums for applied
// migrations.
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("migrate: pool is nil")
	}

	ordered := append([]Migration(nil), migrations...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("migrate: begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := ensureMigrationsTable(ctx, tx); err != nil {
		return err
	}

	applied, err := loadApplied(ctx, tx)
	if err != nil {
		return err
	}

	for _, m := range ordered {
		checksum := hashMigration(m)
		if appliedChecksum, ok := applied[m.ID]; ok {
			if appliedChecksum != checksum {
				return fmt.Errorf("migrate: checksum mismatch for %s", m.ID)
			}
			continue
		}

		if strings.TrimSpace(m.Up) == "" {
			return fmt.Errorf("migrate: empty up migration for %s", m.ID)
		}

		if _, err := tx.Exec(ctx, m.Up); err != nil {
			return fmt.Errorf("migrate: apply %s: %w", m.ID, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(id, checksum, applied_at) VALUES ($1,$2,$3)`, m.ID, checksum, time.Now().UTC()); err != nil {
			return fmt.Errorf("migrate: record %s: %w", m.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: commit: %w", err)
	}
	return nil
}

// RollbackLast rolls back the most recently applied migrations up to the
// provided number of steps. It requires each migration to define a down
// script.
func RollbackLast(ctx context.Context, pool *pgxpool.Pool, steps int) error {
	if pool == nil {
		return errors.New("migrate: pool is nil")
	}
	if steps <= 0 {
		return nil
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("migrate: begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := ensureMigrationsTable(ctx, tx); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `SELECT id, checksum FROM schema_migrations ORDER BY applied_at DESC, id DESC`)
	if err != nil {
		return fmt.Errorf("migrate: list applied: %w", err)
	}
	defer rows.Close()

	appliedOrder := make([]string, 0)
	appliedChecksums := make(map[string]string)
	for rows.Next() {
		var id, checksum string
		if err := rows.Scan(&id, &checksum); err != nil {
			return fmt.Errorf("migrate: scan applied: %w", err)
		}
		appliedOrder = append(appliedOrder, id)
		appliedChecksums[id] = checksum
	}
	if rows.Err() != nil {
		return fmt.Errorf("migrate: iterate applied: %w", rows.Err())
	}

	available := make(map[string]Migration, len(migrations))
	for _, m := range migrations {
		available[m.ID] = m
	}

	for i := 0; i < steps && i < len(appliedOrder); i++ {
		id := appliedOrder[i]
		m, ok := available[id]
		if !ok {
			return fmt.Errorf("migrate: unknown migration %s", id)
		}
		if strings.TrimSpace(m.Down) == "" {
			return fmt.Errorf("migrate: down migration missing for %s", id)
		}
		expectedChecksum := hashMigration(m)
		if appliedChecksums[id] != expectedChecksum {
			return fmt.Errorf("migrate: checksum mismatch for %s", id)
		}
		if _, err := tx.Exec(ctx, m.Down); err != nil {
			return fmt.Errorf("migrate: rollback %s: %w", id, err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE id = $1`, id); err != nil {
			return fmt.Errorf("migrate: delete record %s: %w", id, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: commit rollback: %w", err)
	}
	return nil
}

func ensureMigrationsTable(ctx context.Context, tx pgx.Tx) error {
	const ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
        id TEXT PRIMARY KEY,
        checksum TEXT NOT NULL,
        applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );`
	if _, err := tx.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("migrate: ensure table: %w", err)
	}
	return nil
}

func loadApplied(ctx context.Context, tx pgx.Tx) (map[string]string, error) {
	rows, err := tx.Query(ctx, `SELECT id, checksum FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: load applied: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]string)
	for rows.Next() {
		var id, checksum string
		if err := rows.Scan(&id, &checksum); err != nil {
			return nil, fmt.Errorf("migrate: scan applied: %w", err)
		}
		applied[id] = checksum
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("migrate: iterate applied: %w", rows.Err())
	}
	return applied, nil
}

func hashMigration(m Migration) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{m.ID, m.Up, m.Down}, "::")))
	return hex.EncodeToString(sum[:])
}

var catalogSchemaMigration = Migration{
	ID:          "2025031001_catalog_schema",
	Description: "Create catalog, worker, and job tables",
	Up: `
CREATE TABLE IF NOT EXISTS catalog_entities (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    canonical_title TEXT NOT NULL,
    sort_title TEXT NOT NULL,
    synopsis TEXT NOT NULL DEFAULT '',
    release_date DATE,
    original_language TEXT,
    status TEXT NOT NULL DEFAULT 'announced',
    genres TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    tags TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    external_ids JSONB NOT NULL DEFAULT '{}'::JSONB,
    primary_artwork_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS catalog_entities_type_idx ON catalog_entities(entity_type);
CREATE INDEX IF NOT EXISTS catalog_entities_sort_idx ON catalog_entities(sort_title);

CREATE TABLE IF NOT EXISTS series (
    entity_id TEXT PRIMARY KEY REFERENCES catalog_entities(id) ON DELETE CASCADE,
    total_seasons INTEGER NOT NULL DEFAULT 0,
    episode_runtime_min INTEGER,
    origin_network TEXT,
    air_schedule TEXT,
    official_site TEXT,
    rating_certificate TEXT
);

CREATE TABLE IF NOT EXISTS seasons (
    entity_id TEXT PRIMARY KEY REFERENCES catalog_entities(id) ON DELETE CASCADE,
    series_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    season_number INTEGER NOT NULL,
    title_variant TEXT,
    release_year INTEGER,
    summary TEXT,
    poster_art_id TEXT
);
CREATE INDEX IF NOT EXISTS seasons_series_idx ON seasons(series_id);

CREATE TABLE IF NOT EXISTS episodes (
    entity_id TEXT PRIMARY KEY REFERENCES catalog_entities(id) ON DELETE CASCADE,
    season_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    episode_number INTEGER NOT NULL,
    absolute_number INTEGER,
    runtime_min INTEGER,
    air_date DATE,
    production_code TEXT,
    sequence_overrides JSONB NOT NULL DEFAULT '{}'::JSONB
);
CREATE INDEX IF NOT EXISTS episodes_season_idx ON episodes(season_id);

CREATE TABLE IF NOT EXISTS movies (
    entity_id TEXT PRIMARY KEY REFERENCES catalog_entities(id) ON DELETE CASCADE,
    runtime_min INTEGER,
    content_rating TEXT,
    production_status TEXT,
    budget BIGINT,
    revenue BIGINT,
    franchise TEXT,
    box_office_region_breakdown JSONB NOT NULL DEFAULT '{}'::JSONB
);

CREATE TABLE IF NOT EXISTS artists (
    entity_id TEXT PRIMARY KEY REFERENCES catalog_entities(id) ON DELETE CASCADE,
    artist_type TEXT NOT NULL DEFAULT 'group',
    formed_year INTEGER,
    disbanded_year INTEGER,
    origin_location TEXT,
    aliases TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    primary_genres TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    biography TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS albums (
    entity_id TEXT PRIMARY KEY REFERENCES catalog_entities(id) ON DELETE CASCADE,
    artist_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    album_type TEXT NOT NULL DEFAULT 'studio',
    label TEXT,
    release_status TEXT NOT NULL DEFAULT 'announced',
    barcode TEXT,
    musicbrainz_release_group_id TEXT
);
CREATE INDEX IF NOT EXISTS albums_artist_idx ON albums(artist_id);

CREATE TABLE IF NOT EXISTS tracks (
    entity_id TEXT PRIMARY KEY REFERENCES catalog_entities(id) ON DELETE CASCADE,
    album_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    disc_number INTEGER NOT NULL DEFAULT 1,
    track_number INTEGER NOT NULL DEFAULT 1,
    duration_ms INTEGER,
    explicit BOOLEAN NOT NULL DEFAULT FALSE,
    isrc TEXT,
    lyrics TEXT,
    preview_url TEXT
);
CREATE INDEX IF NOT EXISTS tracks_album_idx ON tracks(album_id);

CREATE TABLE IF NOT EXISTS media_releases (
    id TEXT PRIMARY KEY,
    work_type TEXT NOT NULL,
    work_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    release_title TEXT NOT NULL,
    edition TEXT NOT NULL DEFAULT 'standard',
    quality_profile_id TEXT,
    source TEXT NOT NULL DEFAULT 'web',
    resolution TEXT,
    audio_channels TEXT,
    audio_format TEXT,
    release_date DATE,
    external_ids JSONB NOT NULL DEFAULT '{}'::JSONB,
    checksum TEXT
);
CREATE INDEX IF NOT EXISTS media_releases_work_idx ON media_releases(work_id);

CREATE TABLE IF NOT EXISTS library_items (
    id TEXT PRIMARY KEY,
    release_id TEXT NOT NULL REFERENCES media_releases(id) ON DELETE CASCADE,
    storage_path TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    duration_ms BIGINT,
    bit_rate INTEGER,
    video_codec TEXT,
    audio_codec TEXT,
    subtitle_tracks JSONB NOT NULL DEFAULT '[]'::JSONB,
    media_hash TEXT,
    availability TEXT NOT NULL DEFAULT 'present',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS library_items_release_idx ON library_items(release_id);

CREATE TABLE IF NOT EXISTS people (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    birth_date DATE,
    death_date DATE,
    biography TEXT,
    external_ids JSONB NOT NULL DEFAULT '{}'::JSONB
);

CREATE TABLE IF NOT EXISTS series_people (
    series_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    person_id TEXT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    character_name TEXT,
    PRIMARY KEY (series_id, person_id, role)
);

CREATE TABLE IF NOT EXISTS episode_people (
    episode_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    person_id TEXT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    character_name TEXT,
    PRIMARY KEY (episode_id, person_id, role)
);

CREATE TABLE IF NOT EXISTS movie_people (
    movie_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    person_id TEXT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    character_name TEXT,
    PRIMARY KEY (movie_id, person_id, role)
);

CREATE TABLE IF NOT EXISTS track_people (
    track_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    person_id TEXT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    instrument TEXT,
    PRIMARY KEY (track_id, person_id, role)
);

CREATE TABLE IF NOT EXISTS collections (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    collection_type TEXT NOT NULL DEFAULT 'curated'
);

CREATE TABLE IF NOT EXISTS collection_members (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    entity_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (collection_id, entity_id)
);

CREATE TABLE IF NOT EXISTS library_jobs (
    id TEXT PRIMARY KEY,
    job_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    state TEXT NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL,
    payload_uri TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL DEFAULT '',
    attempt INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    stream_id TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS library_jobs_target_idx ON library_jobs(target_id);
CREATE INDEX IF NOT EXISTS library_jobs_state_idx ON library_jobs(state);

CREATE TABLE IF NOT EXISTS library_metadata_overrides (
    entity_id TEXT PRIMARY KEY REFERENCES catalog_entities(id) ON DELETE CASCADE,
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS worker_nodes (
    id TEXT PRIMARY KEY,
    pool TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    version TEXT NOT NULL,
    capabilities JSONB NOT NULL DEFAULT '[]'::JSONB,
    status TEXT NOT NULL,
    registered_at TIMESTAMPTZ NOT NULL,
    last_heartbeat_at TIMESTAMPTZ NOT NULL,
    active_jobs INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS worker_nodes_pool_idx ON worker_nodes(pool);
`,
	Down: `
DROP TABLE IF EXISTS worker_nodes;
DROP TABLE IF EXISTS library_metadata_overrides;
DROP TABLE IF EXISTS library_jobs;
DROP TABLE IF EXISTS collection_members;
DROP TABLE IF EXISTS collections;
DROP TABLE IF EXISTS track_people;
DROP TABLE IF EXISTS movie_people;
DROP TABLE IF EXISTS episode_people;
DROP TABLE IF EXISTS series_people;
DROP TABLE IF EXISTS people;
DROP TABLE IF EXISTS library_items;
DROP TABLE IF EXISTS media_releases;
DROP TABLE IF EXISTS tracks;
DROP TABLE IF EXISTS albums;
DROP TABLE IF EXISTS artists;
DROP TABLE IF EXISTS movies;
DROP TABLE IF EXISTS episodes;
DROP TABLE IF EXISTS seasons;
DROP TABLE IF EXISTS series;
DROP TABLE IF EXISTS catalog_entities;
`,
}

var jobHistoryMigration = Migration{
	ID:          "2025031501_job_history",
	Description: "Track job execution history",
	Up: `
CREATE TABLE IF NOT EXISTS library_job_history (
    id BIGSERIAL PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES library_jobs(id) ON DELETE CASCADE,
    attempt INTEGER NOT NULL,
    state TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS library_job_history_job_idx ON library_job_history(job_id, occurred_at DESC, id DESC);
`,
	Down: `
DROP TABLE IF EXISTS library_job_history;
`,
}

var demoSeedMigration = Migration{
	ID:          "2025031002_demo_seed",
	Description: "Seed demo catalog entities",
	Up: `
INSERT INTO collections (id, title, description, collection_type)
VALUES
    ('collection-demo-requests', 'Demo Spotlight', 'Sample collection showcasing arrgo capabilities', 'curated')
ON CONFLICT (id) DO NOTHING;

INSERT INTO catalog_entities (id, entity_type, canonical_title, sort_title, synopsis, status, genres, tags)
VALUES
    ('series-demo-chronicles', 'series', 'Chronicles of Automation', 'chronicles of automation', 'Follow the arrgo team as they conquer backlog chaos.', 'released', ARRAY['Sci-Fi','Drama'], ARRAY['demo','featured']),
    ('movie-demo-signal', 'movie', 'The Signal in the Noise', 'signal in the noise', 'A documentary on observability-driven operations.', 'released', ARRAY['Documentary'], ARRAY['demo','observability']),
    ('artist-demo-synth', 'artist', 'Synthwave Architects', 'synthwave architects', 'Collective of producers building neon soundtracks for arrgo.', 'released', ARRAY['Electronic'], ARRAY['demo','soundtrack'])
ON CONFLICT (id) DO NOTHING;

INSERT INTO series (entity_id, total_seasons, episode_runtime_min, origin_network)
VALUES ('series-demo-chronicles', 3, 48, 'arrgo.tv')
ON CONFLICT (entity_id) DO NOTHING;

INSERT INTO movies (entity_id, runtime_min, content_rating, production_status)
VALUES ('movie-demo-signal', 92, 'PG', 'released')
ON CONFLICT (entity_id) DO NOTHING;

INSERT INTO artists (entity_id, artist_type, origin_location, primary_genres)
VALUES ('artist-demo-synth', 'group', 'Neo Cascadia', ARRAY['Electronic','Synthwave'])
ON CONFLICT (entity_id) DO NOTHING;

INSERT INTO collection_members (collection_id, entity_id, entity_type, position)
VALUES
    ('collection-demo-requests', 'series-demo-chronicles', 'series', 1),
    ('collection-demo-requests', 'movie-demo-signal', 'movie', 2),
    ('collection-demo-requests', 'artist-demo-synth', 'artist', 3)
ON CONFLICT (collection_id, entity_id) DO NOTHING;
`,
	Down: `
DELETE FROM collection_members WHERE collection_id = 'collection-demo-requests' AND entity_id IN ('series-demo-chronicles','movie-demo-signal','artist-demo-synth');
DELETE FROM artists WHERE entity_id = 'artist-demo-synth';
DELETE FROM movies WHERE entity_id = 'movie-demo-signal';
DELETE FROM series WHERE entity_id = 'series-demo-chronicles';
DELETE FROM catalog_entities WHERE id IN ('artist-demo-synth','movie-demo-signal','series-demo-chronicles');
DELETE FROM collections WHERE id = 'collection-demo-requests';
`,
}
