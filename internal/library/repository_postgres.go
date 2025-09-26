package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arrgo/arrgo/internal/library/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository persists library state to PostgreSQL.
type PostgresRepository struct {
	pool         *pgxpool.Pool
	seedDemoData bool
}

// PostgresOption configures the Postgres-backed repository.
type PostgresOption func(*PostgresRepository)

// WithDemoSeedData toggles inserting demo seed data when EnsureSchema runs.
func WithDemoSeedData(enabled bool) PostgresOption {
	return func(r *PostgresRepository) {
		r.seedDemoData = enabled
	}
}

// NewPostgresRepository builds a Postgres-backed repository.
func NewPostgresRepository(pool *pgxpool.Pool, opts ...PostgresOption) (*PostgresRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("library: pgx pool is required")
	}
	repo := &PostgresRepository{pool: pool}
	for _, opt := range opts {
		if opt != nil {
			opt(repo)
		}
	}
	return repo, nil
}

// EnsureSchema applies database migrations and optional demo seeds.
func (r *PostgresRepository) EnsureSchema(ctx context.Context) error {
	opts := migrations.Options{SeedDemoData: r.seedDemoData}
	if err := migrations.Apply(ctx, r.pool, opts); err != nil {
		return fmt.Errorf("library: apply migrations: %w", err)
	}
	return nil
}

// UpsertEntity inserts or updates an entity row.
func (r *PostgresRepository) UpsertEntity(ctx context.Context, entity Entity) error {
	id := strings.TrimSpace(entity.ID)
	if id == "" {
		return fmt.Errorf("library: entity id is required")
	}
	entityType := strings.TrimSpace(entity.EntityType)
	title := strings.TrimSpace(entity.Title)
	if title == "" {
		title = id
	}
	sortTitle := strings.ToLower(title)
	synopsis := strings.TrimSpace(entity.Synopsis)

	const stmt = `
INSERT INTO catalog_entities (id, entity_type, canonical_title, sort_title, synopsis)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET
    entity_type = EXCLUDED.entity_type,
    canonical_title = EXCLUDED.canonical_title,
    sort_title = EXCLUDED.sort_title,
    synopsis = EXCLUDED.synopsis;
`
	if _, err := r.pool.Exec(ctx, stmt, id, entityType, title, sortTitle, synopsis); err != nil {
		return fmt.Errorf("library: upsert entity %q: %w", id, err)
	}
	return nil
}

// GetEntity returns a single entity row.
func (r *PostgresRepository) GetEntity(ctx context.Context, id string) (Entity, bool, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return Entity{}, false, nil
	}

	const stmt = `
SELECT id, entity_type, canonical_title, synopsis
FROM catalog_entities
WHERE id = $1;
`
	row := r.pool.QueryRow(ctx, stmt, trimmedID)

	var entity Entity
	if err := row.Scan(&entity.ID, &entity.EntityType, &entity.Title, &entity.Synopsis); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Entity{}, false, nil
		}
		return Entity{}, false, fmt.Errorf("library: get entity %q: %w", trimmedID, err)
	}

	return entity, true, nil
}

// ListEntities returns all entities ordered by title.
func (r *PostgresRepository) ListEntities(ctx context.Context) ([]Entity, error) {
	const stmt = `
SELECT id, entity_type, canonical_title, synopsis
FROM catalog_entities
ORDER BY sort_title ASC;
`
	rows, err := r.pool.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("library: list entities: %w", err)
	}
	defer rows.Close()

	entities := []Entity{}
	for rows.Next() {
		var entity Entity
		if err := rows.Scan(&entity.ID, &entity.EntityType, &entity.Title, &entity.Synopsis); err != nil {
			return nil, fmt.Errorf("library: scan entity: %w", err)
		}
		entities = append(entities, entity)
	}
	return entities, nil
}

// SaveJob inserts a new job row.
func (r *PostgresRepository) SaveJob(ctx context.Context, job Job) error {
	canonical := canonicalizeJob(job)

	const stmt = `
INSERT INTO library_jobs (
    id, job_type, target_id, state, submitted_at, payload_uri, idempotency_key,
    attempt, last_attempt_at, completed_at, stream_id
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);
`
	_, err := r.pool.Exec(ctx, stmt,
		canonical.ID,
		canonical.JobType,
		canonical.TargetID,
		canonical.State,
		canonical.SubmittedAt,
		canonical.PayloadURI,
		canonical.IdempotencyKey,
		canonical.Attempt,
		nullTime(canonical.LastAttemptAt),
		nullTime(canonical.CompletedAt),
		canonical.StreamID,
	)
	if err != nil {
		return fmt.Errorf("library: insert job %q: %w", canonical.ID, err)
	}
	return nil
}

// UpdateJob persists job state mutations.
func (r *PostgresRepository) UpdateJob(ctx context.Context, job Job) error {
	canonical := canonicalizeJob(job)

	const stmt = `
UPDATE library_jobs SET
    job_type = $2,
    target_id = $3,
    state = $4,
    payload_uri = $5,
    idempotency_key = $6,
    attempt = $7,
    last_attempt_at = $8,
    completed_at = $9,
    stream_id = $10
WHERE id = $1;
`
	result, err := r.pool.Exec(ctx, stmt,
		canonical.ID,
		canonical.JobType,
		canonical.TargetID,
		canonical.State,
		canonical.PayloadURI,
		canonical.IdempotencyKey,
		canonical.Attempt,
		nullTime(canonical.LastAttemptAt),
		nullTime(canonical.CompletedAt),
		canonical.StreamID,
	)
	if err != nil {
		return fmt.Errorf("library: update job %q: %w", canonical.ID, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("library: job %q not found", canonical.ID)
	}
	return nil
}

// ListJobs returns jobs ordered by submission time (newest first).
func (r *PostgresRepository) ListJobs(ctx context.Context) ([]Job, error) {
	const stmt = `
SELECT id, job_type, target_id, state, submitted_at, payload_uri, idempotency_key,
       attempt, last_attempt_at, completed_at, stream_id
FROM library_jobs
ORDER BY submitted_at DESC;
`
	rows, err := r.pool.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("library: list jobs: %w", err)
	}
	defer rows.Close()

	jobs := []Job{}
	for rows.Next() {
		var (
			job           Job
			lastAttempt   pgtype.Timestamptz
			completedTime pgtype.Timestamptz
		)
		if err := rows.Scan(
			&job.ID,
			&job.JobType,
			&job.TargetID,
			&job.State,
			&job.SubmittedAt,
			&job.PayloadURI,
			&job.IdempotencyKey,
			&job.Attempt,
			&lastAttempt,
			&completedTime,
			&job.StreamID,
		); err != nil {
			return nil, fmt.Errorf("library: scan job: %w", err)
		}
		job.SubmittedAt = job.SubmittedAt.UTC()
		if lastAttempt.Valid {
			job.LastAttemptAt = lastAttempt.Time.UTC()
		} else {
			job.LastAttemptAt = time.Time{}
		}
		if completedTime.Valid {
			job.CompletedAt = completedTime.Time.UTC()
		} else {
			job.CompletedAt = time.Time{}
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// UpsertMetadataOverrides stores overrides in PostgreSQL.
func (r *PostgresRepository) UpsertMetadataOverrides(ctx context.Context, entityID string, overrides MetadataOverrides) error {
	id := strings.TrimSpace(entityID)
	if id == "" {
		return fmt.Errorf("library: metadata override entity id is required")
	}
	payload, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("library: encode metadata overrides: %w", err)
	}
	updatedAt := overrides.UpdatedAt.UTC()

	const stmt = `
INSERT INTO library_metadata_overrides (entity_id, payload, updated_at)
VALUES ($1,$2,$3)
ON CONFLICT (entity_id) DO UPDATE SET
    payload = EXCLUDED.payload,
    updated_at = EXCLUDED.updated_at;
`
	if _, err := r.pool.Exec(ctx, stmt, id, payload, updatedAt); err != nil {
		return fmt.Errorf("library: upsert metadata overrides for %q: %w", id, err)
	}
	return nil
}

// DeleteMetadataOverrides removes overrides for an entity.
func (r *PostgresRepository) DeleteMetadataOverrides(ctx context.Context, entityID string) error {
	id := strings.TrimSpace(entityID)
	if id == "" {
		return fmt.Errorf("library: metadata override entity id is required")
	}
	const stmt = `DELETE FROM library_metadata_overrides WHERE entity_id = $1;`
	if _, err := r.pool.Exec(ctx, stmt, id); err != nil {
		return fmt.Errorf("library: delete metadata overrides for %q: %w", id, err)
	}
	return nil
}

// GetMetadataOverrides fetches overrides for a single entity.
func (r *PostgresRepository) GetMetadataOverrides(ctx context.Context, entityID string) (MetadataOverrides, bool, error) {
	id := strings.TrimSpace(entityID)
	if id == "" {
		return MetadataOverrides{}, false, nil
	}

	const stmt = `SELECT payload, updated_at FROM library_metadata_overrides WHERE entity_id = $1;`
	row := r.pool.QueryRow(ctx, stmt, id)

	var (
		payload   []byte
		updatedAt time.Time
		override  MetadataOverrides
	)
	if err := row.Scan(&payload, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MetadataOverrides{}, false, nil
		}
		return MetadataOverrides{}, false, fmt.Errorf("library: get metadata overrides for %q: %w", id, err)
	}
	if err := json.Unmarshal(payload, &override); err != nil {
		return MetadataOverrides{}, false, fmt.Errorf("library: decode metadata override: %w", err)
	}
	override.UpdatedAt = updatedAt.UTC()
	return override, true, nil
}

// ListMetadataOverrides returns all override payloads.
func (r *PostgresRepository) ListMetadataOverrides(ctx context.Context) (map[string]MetadataOverrides, error) {
	const stmt = `SELECT entity_id, payload, updated_at FROM library_metadata_overrides;`
	rows, err := r.pool.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("library: list metadata overrides: %w", err)
	}
	defer rows.Close()

	results := map[string]MetadataOverrides{}
	for rows.Next() {
		var (
			entityID  string
			payload   []byte
			updatedAt time.Time
			override  MetadataOverrides
		)
		if err := rows.Scan(&entityID, &payload, &updatedAt); err != nil {
			return nil, fmt.Errorf("library: scan metadata override: %w", err)
		}
		if err := json.Unmarshal(payload, &override); err != nil {
			return nil, fmt.Errorf("library: decode metadata override: %w", err)
		}
		override.UpdatedAt = updatedAt.UTC()
		results[entityID] = override
	}
	return results, nil
}

func canonicalizeJob(job Job) Job {
	canonical := cloneJob(job)
	canonical.ID = strings.TrimSpace(canonical.ID)
	canonical.JobType = strings.TrimSpace(canonical.JobType)
	canonical.TargetID = strings.TrimSpace(canonical.TargetID)
	canonical.State = strings.TrimSpace(canonical.State)
	canonical.PayloadURI = strings.TrimSpace(canonical.PayloadURI)
	canonical.IdempotencyKey = strings.TrimSpace(canonical.IdempotencyKey)
	canonical.StreamID = strings.TrimSpace(canonical.StreamID)
	canonical.SubmittedAt = canonical.SubmittedAt.UTC()
	canonical.LastAttemptAt = canonical.LastAttemptAt.UTC()
	canonical.CompletedAt = canonical.CompletedAt.UTC()
	return canonical
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}
