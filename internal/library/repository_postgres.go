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
    attempt, last_attempt_at, completed_at, stream_id, error_message
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12);
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
		canonical.ErrorMessage,
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
    stream_id = $10,
    error_message = $11
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
		canonical.ErrorMessage,
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
       attempt, last_attempt_at, completed_at, stream_id, error_message
FROM library_jobs
ORDER BY submitted_at DESC, id DESC;
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
			&job.ErrorMessage,
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

// ListJobsPage returns a single page of jobs ordered by submission time (newest first).
func (r *PostgresRepository) ListJobsPage(ctx context.Context, opts ListJobsOptions) (ListJobsResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultJobPageSize
	}
	limitPlusOne := limit + 1

	params := []any{}
	where := ""
	if opts.Cursor != nil {
		params = append(params, opts.Cursor.SubmittedAt, opts.Cursor.JobID)
		where = "WHERE (submitted_at < $1) OR (submitted_at = $1 AND id < $2)"
	}

	query := `SELECT id, job_type, target_id, state, submitted_at, payload_uri, idempotency_key,
       attempt, last_attempt_at, completed_at, stream_id, error_message
FROM library_jobs`
	if where != "" {
		query += "\n" + where
	}
	query += "\nORDER BY submitted_at DESC, id DESC\nLIMIT $" + fmt.Sprint(len(params)+1)
	params = append(params, limitPlusOne)

	rows, err := r.pool.Query(ctx, query, params...)
	if err != nil {
		return ListJobsResult{}, fmt.Errorf("library: list jobs page: %w", err)
	}
	defer rows.Close()

	jobs := make([]Job, 0, limitPlusOne)
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
			&job.ErrorMessage,
		); err != nil {
			return ListJobsResult{}, fmt.Errorf("library: scan job: %w", err)
		}
		job.SubmittedAt = job.SubmittedAt.UTC()
		if lastAttempt.Valid {
			job.LastAttemptAt = lastAttempt.Time.UTC()
		}
		if completedTime.Valid {
			job.CompletedAt = completedTime.Time.UTC()
		}
		jobs = append(jobs, job)
	}

	var next *JobListCursor
	if len(jobs) > limit {
		last := jobs[limit-1]
		next = &JobListCursor{SubmittedAt: last.SubmittedAt, JobID: last.ID}
		jobs = jobs[:limit]
	}

	return ListJobsResult{Jobs: jobs, NextCursor: next}, nil
}

// AppendJobHistory stores a history record while enforcing retention policies.
func (r *PostgresRepository) AppendJobHistory(ctx context.Context, jobID string, record JobHistoryRecord, retention JobHistoryRetention) error {
	trimmedID := strings.TrimSpace(jobID)
	if trimmedID == "" {
		return fmt.Errorf("library: job history job id is required")
	}

	state := strings.TrimSpace(record.State)
	message := strings.TrimSpace(record.Message)
	occurredAt := record.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	} else {
		occurredAt = occurredAt.UTC()
	}
	attempt := record.Attempt
	if attempt < 0 {
		attempt = 0
	}

	const insertStmt = `INSERT INTO library_job_history (job_id, attempt, state, message, occurred_at)
VALUES ($1,$2,$3,$4,$5);`
	if _, err := r.pool.Exec(ctx, insertStmt, trimmedID, attempt, state, message, occurredAt); err != nil {
		return fmt.Errorf("library: insert job history: %w", err)
	}

	if retention.MaxAge > 0 {
		cutoff := occurredAt.Add(-retention.MaxAge)
		if _, err := r.pool.Exec(ctx, `DELETE FROM library_job_history WHERE job_id = $1 AND occurred_at < $2`, trimmedID, cutoff); err != nil {
			return fmt.Errorf("library: prune job history by age: %w", err)
		}
	}

	if retention.MaxEntries > 0 {
		const pruneStmt = `
DELETE FROM library_job_history
WHERE job_id = $1
  AND id NOT IN (
        SELECT id FROM library_job_history
        WHERE job_id = $1
        ORDER BY occurred_at DESC, id DESC
        LIMIT $2
  );`
		if _, err := r.pool.Exec(ctx, pruneStmt, trimmedID, retention.MaxEntries); err != nil {
			return fmt.Errorf("library: prune job history by count: %w", err)
		}
	}

	return nil
}

// ListJobHistories returns the latest history entries for the requested jobs.
func (r *PostgresRepository) ListJobHistories(ctx context.Context, jobIDs []string, limit int) (map[string][]JobHistoryRecord, error) {
	filtered := make([]string, 0, len(jobIDs))
	for _, id := range jobIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if len(filtered) == 0 {
		return map[string][]JobHistoryRecord{}, nil
	}
	if limit <= 0 {
		limit = defaultJobHistoryLimit
	}

	const stmt = `
SELECT job_id, attempt, state, message, occurred_at
FROM (
    SELECT job_id, attempt, state, message, occurred_at,
           ROW_NUMBER() OVER (PARTITION BY job_id ORDER BY occurred_at DESC, id DESC) AS rn
    FROM library_job_history
    WHERE job_id = ANY($1)
)
WHERE rn <= $2
ORDER BY job_id, occurred_at DESC, rn;
`
	rows, err := r.pool.Query(ctx, stmt, filtered, limit)
	if err != nil {
		return nil, fmt.Errorf("library: list job history: %w", err)
	}
	defer rows.Close()

	results := make(map[string][]JobHistoryRecord, len(filtered))
	for rows.Next() {
		var (
			jobID    string
			entry    JobHistoryRecord
			occurred time.Time
		)
		if err := rows.Scan(&jobID, &entry.Attempt, &entry.State, &entry.Message, &occurred); err != nil {
			return nil, fmt.Errorf("library: scan job history: %w", err)
		}
		entry.State = strings.TrimSpace(entry.State)
		entry.Message = strings.TrimSpace(entry.Message)
		entry.OccurredAt = occurred.UTC()
		results[jobID] = append(results[jobID], entry)
	}
	return results, nil
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

func (r *PostgresRepository) SaveRequest(ctx context.Context, request Request) error {
	canonical := canonicalizeRequest(request)
	if canonical.SubmittedAt.IsZero() {
		canonical.SubmittedAt = time.Now().UTC()
	}
	if canonical.UpdatedAt.IsZero() || canonical.UpdatedAt.Before(canonical.SubmittedAt) {
		canonical.UpdatedAt = canonical.SubmittedAt
	}

	payload, err := json.Marshal(canonical.ExternalIDs)
	if err != nil {
		return fmt.Errorf("library: encode request external ids: %w", err)
	}

	const stmt = `
INSERT INTO library_requests (
    id, kind, title, requested_by, notes, policy, external_ids,
    quota_remaining, submitted_at, updated_at, sla_deadline, status
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12);
`
	_, execErr := r.pool.Exec(ctx, stmt,
		canonical.ID,
		canonical.Kind,
		canonical.Title,
		canonical.RequestedBy,
		canonical.Notes,
		canonical.Policy,
		payload,
		canonical.QuotaRemaining,
		canonical.SubmittedAt,
		canonical.UpdatedAt,
		nullTime(canonical.SLADeadline),
		canonical.Status,
	)
	if execErr != nil {
		return fmt.Errorf("library: insert request %q: %w", canonical.ID, execErr)
	}
	return nil
}

func (r *PostgresRepository) UpdateRequest(ctx context.Context, request Request) error {
	canonical := canonicalizeRequest(request)
	if canonical.SubmittedAt.IsZero() {
		canonical.SubmittedAt = time.Now().UTC()
	}
	if canonical.UpdatedAt.IsZero() || canonical.UpdatedAt.Before(canonical.SubmittedAt) {
		canonical.UpdatedAt = canonical.SubmittedAt
	}

	payload, err := json.Marshal(canonical.ExternalIDs)
	if err != nil {
		return fmt.Errorf("library: encode request external ids: %w", err)
	}

	const stmt = `
UPDATE library_requests SET
    kind = $2,
    title = $3,
    requested_by = $4,
    notes = $5,
    policy = $6,
    external_ids = $7,
    quota_remaining = $8,
    submitted_at = $9,
    updated_at = $10,
    sla_deadline = $11,
    status = $12
WHERE id = $1;
`
	result, execErr := r.pool.Exec(ctx, stmt,
		canonical.ID,
		canonical.Kind,
		canonical.Title,
		canonical.RequestedBy,
		canonical.Notes,
		canonical.Policy,
		payload,
		canonical.QuotaRemaining,
		canonical.SubmittedAt,
		canonical.UpdatedAt,
		nullTime(canonical.SLADeadline),
		canonical.Status,
	)
	if execErr != nil {
		return fmt.Errorf("library: update request %q: %w", canonical.ID, execErr)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("library: request %q not found", canonical.ID)
	}
	return nil
}

func (r *PostgresRepository) GetRequest(ctx context.Context, id string) (Request, bool, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return Request{}, false, nil
	}

	const stmt = `
SELECT id, kind, title, requested_by, notes, policy, external_ids,
       quota_remaining, submitted_at, updated_at, sla_deadline, status
FROM library_requests
WHERE id = $1;
`
	row := r.pool.QueryRow(ctx, stmt, trimmed)

	var canonical Request
	var idsJSON []byte
	var sla pgtype.Timestamptz
	if err := row.Scan(
		&canonical.ID,
		&canonical.Kind,
		&canonical.Title,
		&canonical.RequestedBy,
		&canonical.Notes,
		&canonical.Policy,
		&idsJSON,
		&canonical.QuotaRemaining,
		&canonical.SubmittedAt,
		&canonical.UpdatedAt,
		&sla,
		&canonical.Status,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Request{}, false, nil
		}
		return Request{}, false, fmt.Errorf("library: get request %q: %w", trimmed, err)
	}

	canonical.SubmittedAt = canonical.SubmittedAt.UTC()
	canonical.UpdatedAt = canonical.UpdatedAt.UTC()
	if sla.Valid {
		canonical.SLADeadline = sla.Time.UTC()
	}
	if len(idsJSON) > 0 {
		ids := make(map[string]string)
		if err := json.Unmarshal(idsJSON, &ids); err != nil {
			return Request{}, false, fmt.Errorf("library: decode request external ids: %w", err)
		}
		if len(ids) > 0 {
			canonical.ExternalIDs = ids
		}
	}
	return canonical, true, nil
}

func (r *PostgresRepository) ListRequests(ctx context.Context) ([]Request, error) {
	const stmt = `
SELECT id, kind, title, requested_by, notes, policy, external_ids,
       quota_remaining, submitted_at, updated_at, sla_deadline, status
FROM library_requests
ORDER BY submitted_at DESC, id DESC;
`
	rows, err := r.pool.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("library: list requests: %w", err)
	}
	defer rows.Close()

	requests := []Request{}
	for rows.Next() {
		var record Request
		var idsJSON []byte
		var sla pgtype.Timestamptz
		if scanErr := rows.Scan(
			&record.ID,
			&record.Kind,
			&record.Title,
			&record.RequestedBy,
			&record.Notes,
			&record.Policy,
			&idsJSON,
			&record.QuotaRemaining,
			&record.SubmittedAt,
			&record.UpdatedAt,
			&sla,
			&record.Status,
		); scanErr != nil {
			return nil, fmt.Errorf("library: scan request: %w", scanErr)
		}
		record.SubmittedAt = record.SubmittedAt.UTC()
		record.UpdatedAt = record.UpdatedAt.UTC()
		if sla.Valid {
			record.SLADeadline = sla.Time.UTC()
		}
		if len(idsJSON) > 0 {
			ids := make(map[string]string)
			if err := json.Unmarshal(idsJSON, &ids); err != nil {
				return nil, fmt.Errorf("library: decode request external ids: %w", err)
			}
			if len(ids) > 0 {
				record.ExternalIDs = ids
			}
		}
		requests = append(requests, record)
	}
	return requests, nil
}

func (r *PostgresRepository) AppendRequestApproval(ctx context.Context, event RequestApprovalEvent) error {
	canonical := canonicalizeRequestApproval(event)
	if canonical.DecidedAt.IsZero() {
		canonical.DecidedAt = time.Now().UTC()
	}

	const stmt = `
INSERT INTO library_request_approvals (
    id, request_id, decision, actor, notes, decided_at
)
VALUES ($1,$2,$3,$4,$5,$6);
`
	_, err := r.pool.Exec(ctx, stmt,
		canonical.ID,
		canonical.RequestID,
		canonical.Decision,
		canonical.Actor,
		canonical.Notes,
		canonical.DecidedAt,
	)
	if err != nil {
		return fmt.Errorf("library: insert request approval %q: %w", canonical.ID, err)
	}
	return nil
}

func (r *PostgresRepository) ListRequestApprovals(ctx context.Context, requestID string) ([]RequestApprovalEvent, error) {
	trimmed := strings.TrimSpace(requestID)
	if trimmed == "" {
		return nil, nil
	}

	const stmt = `
SELECT id, request_id, decision, actor, notes, decided_at
FROM library_request_approvals
WHERE request_id = $1
ORDER BY decided_at ASC, id ASC;
`
	rows, err := r.pool.Query(ctx, stmt, trimmed)
	if err != nil {
		return nil, fmt.Errorf("library: list request approvals: %w", err)
	}
	defer rows.Close()

	approvals := []RequestApprovalEvent{}
	for rows.Next() {
		var event RequestApprovalEvent
		if scanErr := rows.Scan(
			&event.ID,
			&event.RequestID,
			&event.Decision,
			&event.Actor,
			&event.Notes,
			&event.DecidedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("library: scan request approval: %w", scanErr)
		}
		event.DecidedAt = event.DecidedAt.UTC()
		approvals = append(approvals, event)
	}
	return approvals, nil
}

func (r *PostgresRepository) AppendRequestNotification(ctx context.Context, notification RequestNotification) error {
	canonical := canonicalizeRequestNotification(notification)
	if canonical.AttemptedAt.IsZero() {
		canonical.AttemptedAt = time.Now().UTC()
	}

	const stmt = `
INSERT INTO library_request_notifications (
    id, request_id, channel, target, status, attempted_at, error_message
)
VALUES ($1,$2,$3,$4,$5,$6,$7);
`
	_, err := r.pool.Exec(ctx, stmt,
		canonical.ID,
		canonical.RequestID,
		canonical.Channel,
		canonical.Target,
		canonical.Status,
		canonical.AttemptedAt,
		canonical.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("library: insert request notification %q: %w", canonical.ID, err)
	}
	return nil
}

func (r *PostgresRepository) ListRequestNotifications(ctx context.Context, requestID string) ([]RequestNotification, error) {
	trimmed := strings.TrimSpace(requestID)
	if trimmed == "" {
		return nil, nil
	}

	const stmt = `
SELECT id, request_id, channel, target, status, attempted_at, error_message
FROM library_request_notifications
WHERE request_id = $1
ORDER BY attempted_at ASC, id ASC;
`
	rows, err := r.pool.Query(ctx, stmt, trimmed)
	if err != nil {
		return nil, fmt.Errorf("library: list request notifications: %w", err)
	}
	defer rows.Close()

	notifications := []RequestNotification{}
	for rows.Next() {
		var notification RequestNotification
		if scanErr := rows.Scan(
			&notification.ID,
			&notification.RequestID,
			&notification.Channel,
			&notification.Target,
			&notification.Status,
			&notification.AttemptedAt,
			&notification.ErrorMessage,
		); scanErr != nil {
			return nil, fmt.Errorf("library: scan request notification: %w", scanErr)
		}
		notification.AttemptedAt = notification.AttemptedAt.UTC()
		notifications = append(notifications, notification)
	}
	return notifications, nil
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
	canonical.ErrorMessage = strings.TrimSpace(canonical.ErrorMessage)
	canonical.SubmittedAt = canonical.SubmittedAt.UTC()
	canonical.LastAttemptAt = canonical.LastAttemptAt.UTC()
	canonical.CompletedAt = canonical.CompletedAt.UTC()
	canonical.History = nil
	return canonical
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}
