package migrations

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	SeedDemoData bool
}

type Condition func(Options) bool

type Migration struct {
	Version   int
	Name      string
	Up        []string
	Down      []string
	Condition Condition
}

func (m Migration) shouldRun(opts Options) bool {
	if m.Condition == nil {
		return true
	}
	return m.Condition(opts)
}

var registeredMigrations = []Migration{
	{
		Version: 1,
		Name:    "library_initial_schema",
		Up: []string{
			`CREATE TABLE IF NOT EXISTS catalog_entities (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    canonical_title TEXT NOT NULL,
    sort_title TEXT NOT NULL,
    synopsis TEXT NOT NULL DEFAULT ''
);`,
			`CREATE TABLE IF NOT EXISTS library_jobs (
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
    stream_id TEXT NOT NULL DEFAULT ''
);`,
			`CREATE INDEX IF NOT EXISTS library_jobs_target_idx ON library_jobs(target_id);`,
			`CREATE TABLE IF NOT EXISTS library_metadata_overrides (
    entity_id TEXT PRIMARY KEY,
    payload JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);`,
		},
		Down: []string{
			`DROP TABLE IF EXISTS library_metadata_overrides;`,
			`DROP INDEX IF EXISTS library_jobs_target_idx;`,
			`DROP TABLE IF EXISTS library_jobs;`,
			`DROP TABLE IF EXISTS catalog_entities;`,
		},
	},
	{
		Version: 2,
		Name:    "library_demo_seed_data",
		Up: []string{
			`INSERT INTO catalog_entities (id, entity_type, canonical_title, sort_title, synopsis)
VALUES
    ('demo-movie-001', 'movie', 'Demo Movie', 'demo movie', 'A placeholder movie used to demonstrate catalog views.'),
    ('demo-series-001', 'series', 'Demo Series', 'demo series', 'A placeholder series used to demonstrate request workflows.')
ON CONFLICT (id) DO NOTHING;`,
			`INSERT INTO library_jobs (
    id, job_type, target_id, state, submitted_at, payload_uri, idempotency_key,
    attempt, last_attempt_at, completed_at, stream_id
) VALUES
    ('demo-job-import', 'import', 'demo-movie-001', 'completed', NOW() - INTERVAL '2 hours', 's3://demo/import/movie', 'demo-job-import', 1, NOW() - INTERVAL '90 minutes', NOW() - INTERVAL '75 minutes', ''),
    ('demo-job-refresh', 'refresh', 'demo-series-001', 'queued', NOW() - INTERVAL '30 minutes', 'https://demo.example/request', 'demo-job-refresh', 0, NULL, NULL, '')
ON CONFLICT (id) DO NOTHING;`,
			`INSERT INTO library_metadata_overrides (entity_id, payload, updated_at)
VALUES
    ('demo-movie-001', '{"quality": "1080p", "cut": "extended", "tags": ["demo", "showcase"]}', NOW())
ON CONFLICT (entity_id) DO UPDATE SET payload = EXCLUDED.payload, updated_at = EXCLUDED.updated_at;`,
		},
		Down: []string{
			`DELETE FROM library_metadata_overrides WHERE entity_id IN ('demo-movie-001');`,
			`DELETE FROM library_jobs WHERE id IN ('demo-job-import', 'demo-job-refresh');`,
			`DELETE FROM catalog_entities WHERE id IN ('demo-movie-001', 'demo-series-001');`,
		},
		Condition: func(opts Options) bool {
			return opts.SeedDemoData
		},
	},
	{
		Version: 3,
		Name:    "library_request_persistence",
		Up: []string{
			`CREATE TABLE IF NOT EXISTS library_requests (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    policy TEXT NOT NULL DEFAULT '',
    external_ids JSONB NOT NULL DEFAULT '{}'::jsonb,
    quota_remaining INTEGER NOT NULL DEFAULT 0,
    submitted_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    sla_deadline TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'pending'
);`,
			`CREATE INDEX IF NOT EXISTS library_requests_status_idx ON library_requests(status);`,
			`CREATE TABLE IF NOT EXISTS library_request_approvals (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL REFERENCES library_requests(id) ON DELETE CASCADE,
    decision TEXT NOT NULL,
    actor TEXT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    decided_at TIMESTAMPTZ NOT NULL
);`,
			`CREATE INDEX IF NOT EXISTS library_request_approvals_request_idx ON library_request_approvals(request_id);`,
			`CREATE TABLE IF NOT EXISTS library_request_notifications (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL REFERENCES library_requests(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,
    target TEXT NOT NULL,
    status TEXT NOT NULL,
    attempted_at TIMESTAMPTZ NOT NULL,
    error_message TEXT NOT NULL DEFAULT ''
);`,
			`CREATE INDEX IF NOT EXISTS library_request_notifications_request_idx ON library_request_notifications(request_id);`,
		},
		Down: []string{
			`DROP INDEX IF EXISTS library_request_notifications_request_idx;`,
			`DROP TABLE IF EXISTS library_request_notifications;`,
			`DROP INDEX IF EXISTS library_request_approvals_request_idx;`,
			`DROP TABLE IF EXISTS library_request_approvals;`,
			`DROP INDEX IF EXISTS library_requests_status_idx;`,
			`DROP TABLE IF EXISTS library_requests;`,
		},
	},
}

const migrationsTableDDL = `
CREATE TABLE IF NOT EXISTS library_schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL
);
`

func Apply(ctx context.Context, pool *pgxpool.Pool, opts Options) error {
	if pool == nil {
		return fmt.Errorf("migrations: pgx pool is nil")
	}
	if _, err := pool.Exec(ctx, migrationsTableDDL); err != nil {
		return fmt.Errorf("migrations: ensure schema table: %w", err)
	}

	applied, err := loadApplied(ctx, pool)
	if err != nil {
		return err
	}

	pending := planMigrations(registeredMigrations, applied, opts)
	if len(pending) == 0 {
		return nil
	}

	for _, migration := range pending {
		if err := applyMigration(ctx, pool, migration); err != nil {
			return err
		}
	}
	return nil
}

func Rollback(ctx context.Context, pool *pgxpool.Pool, targetVersion int) error {
	if pool == nil {
		return fmt.Errorf("migrations: pgx pool is nil")
	}
	if _, err := pool.Exec(ctx, migrationsTableDDL); err != nil {
		return fmt.Errorf("migrations: ensure schema table: %w", err)
	}

	rows, err := pool.Query(ctx, `SELECT version FROM library_schema_migrations WHERE version > $1 ORDER BY version DESC;`, targetVersion)
	if err != nil {
		return fmt.Errorf("migrations: query applied migrations: %w", err)
	}
	defer rows.Close()

	versions := []int{}
	for rows.Next() {
		var version int
		if scanErr := rows.Scan(&version); scanErr != nil {
			return fmt.Errorf("migrations: scan applied version: %w", scanErr)
		}
		versions = append(versions, version)
	}

	if len(versions) == 0 {
		return nil
	}

	migrationMap := map[int]Migration{}
	for _, migration := range registeredMigrations {
		migrationMap[migration.Version] = migration
	}

	for _, version := range versions {
		migration, ok := migrationMap[version]
		if !ok {
			return fmt.Errorf("migrations: version %d not registered", version)
		}
		if len(migration.Down) == 0 {
			return fmt.Errorf("migrations: version %d (%s) cannot be rolled back", version, migration.Name)
		}
		if err := rollbackMigration(ctx, pool, migration); err != nil {
			return err
		}
	}
	return nil
}

func loadApplied(ctx context.Context, pool *pgxpool.Pool) (map[int]struct{}, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM library_schema_migrations;`)
	if err != nil {
		return nil, fmt.Errorf("migrations: list applied versions: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]struct{})
	for rows.Next() {
		var version int
		if scanErr := rows.Scan(&version); scanErr != nil {
			return nil, fmt.Errorf("migrations: scan applied version: %w", scanErr)
		}
		applied[version] = struct{}{}
	}
	return applied, nil
}

func planMigrations(all []Migration, applied map[int]struct{}, opts Options) []Migration {
	pending := make([]Migration, 0, len(all))
	for _, migration := range all {
		if !migration.shouldRun(opts) {
			continue
		}
		if _, ok := applied[migration.Version]; ok {
			continue
		}
		pending = append(pending, migration)
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].Version < pending[j].Version
	})
	return pending
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, migration Migration) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("migrations: begin %s: %w", migration.Name, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	for _, stmt := range migration.Up {
		if _, execErr := tx.Exec(ctx, stmt); execErr != nil {
			return fmt.Errorf("migrations: apply %s: %w", migration.Name, execErr)
		}
	}

	if _, err := tx.Exec(ctx, `INSERT INTO library_schema_migrations (version, name, applied_at) VALUES ($1, $2, $3);`, migration.Version, migration.Name, time.Now().UTC()); err != nil {
		return fmt.Errorf("migrations: record %s: %w", migration.Name, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrations: commit %s: %w", migration.Name, err)
	}
	committed = true
	return nil
}

func rollbackMigration(ctx context.Context, pool *pgxpool.Pool, migration Migration) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("migrations: begin rollback %s: %w", migration.Name, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	for i := len(migration.Down) - 1; i >= 0; i-- {
		if _, execErr := tx.Exec(ctx, migration.Down[i]); execErr != nil {
			return fmt.Errorf("migrations: rollback %s: %w", migration.Name, execErr)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM library_schema_migrations WHERE version = $1;`, migration.Version); err != nil {
		return fmt.Errorf("migrations: remove history for %s: %w", migration.Name, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrations: commit rollback %s: %w", migration.Name, err)
	}
	committed = true
	return nil
}

func Registered() []Migration {
	copies := make([]Migration, len(registeredMigrations))
	copy(copies, registeredMigrations)
	return copies
}
