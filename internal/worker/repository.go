package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines persistence operations for worker metadata.
type Repository interface {
	SaveWorker(ctx context.Context, worker Worker) error
	GetWorker(ctx context.Context, id string) (Worker, error)
	ListWorkers(ctx context.Context) ([]Worker, error)
}

// InMemoryRepository provides an in-memory implementation of Repository for tests.
type InMemoryRepository struct {
	mu      sync.RWMutex
	workers map[string]Worker
}

// NewInMemoryRepository constructs an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{workers: make(map[string]Worker)}
}

// SaveWorker stores or updates worker metadata.
func (r *InMemoryRepository) SaveWorker(_ context.Context, worker Worker) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := worker
	r.workers[worker.ID] = cp
	return nil
}

// GetWorker retrieves a worker by identifier.
func (r *InMemoryRepository) GetWorker(_ context.Context, id string) (Worker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	worker, ok := r.workers[id]
	if !ok {
		return Worker{}, ErrUnknownWorker
	}
	return worker, nil
}

// ListWorkers returns all registered workers.
func (r *InMemoryRepository) ListWorkers(_ context.Context) ([]Worker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]Worker, 0, len(r.workers))
	for _, worker := range r.workers {
		results = append(results, worker)
	}
	return results, nil
}

// PostgresRepository stores worker metadata in PostgreSQL.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository builds a Postgres-backed repository.
func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("worker: pgx pool is required")
	}
	return &PostgresRepository{pool: pool}, nil
}

const (
	upsertWorkerQuery = `
INSERT INTO worker_nodes (
        id,
        pool,
        endpoint,
        version,
        capabilities,
        status,
        registered_at,
        last_heartbeat_at,
        active_jobs
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (id) DO UPDATE SET
        pool = EXCLUDED.pool,
        endpoint = EXCLUDED.endpoint,
        version = EXCLUDED.version,
        capabilities = EXCLUDED.capabilities,
        status = EXCLUDED.status,
        registered_at = LEAST(worker_nodes.registered_at, EXCLUDED.registered_at),
        last_heartbeat_at = EXCLUDED.last_heartbeat_at,
        active_jobs = EXCLUDED.active_jobs;
`
	selectWorkerQuery = `
SELECT id, pool, endpoint, version, capabilities, status, registered_at, last_heartbeat_at, active_jobs
FROM worker_nodes
WHERE id = $1;
`
	listWorkersQuery = `
SELECT id, pool, endpoint, version, capabilities, status, registered_at, last_heartbeat_at, active_jobs
FROM worker_nodes;
`
)

// SaveWorker persists the worker state in PostgreSQL.
func (r *PostgresRepository) SaveWorker(ctx context.Context, worker Worker) error {
	worker.RegisteredAt = worker.RegisteredAt.UTC()
	worker.LastHeartbeatAt = worker.LastHeartbeatAt.UTC()

	payload, err := json.Marshal(worker.Capabilities)
	if err != nil {
		return fmt.Errorf("worker: encode capabilities: %w", err)
	}
	_, err = r.pool.Exec(ctx, upsertWorkerQuery,
		worker.ID,
		worker.Pool,
		worker.Endpoint,
		worker.Version,
		payload,
		string(worker.Status),
		worker.RegisteredAt,
		worker.LastHeartbeatAt,
		worker.ActiveJobs,
	)
	if err != nil {
		return fmt.Errorf("worker: upsert worker: %w", err)
	}
	return nil
}

// GetWorker fetches a worker record from PostgreSQL.
func (r *PostgresRepository) GetWorker(ctx context.Context, id string) (Worker, error) {
	row := r.pool.QueryRow(ctx, selectWorkerQuery, id)

	var (
		payload []byte
		worker  Worker
	)
	worker.ID = id
        if err := row.Scan(
                &worker.ID,
                &worker.Pool,
                &worker.Endpoint,
                &worker.Version,
                &payload,
                &worker.Status,
                &worker.RegisteredAt,
                &worker.LastHeartbeatAt,
                &worker.ActiveJobs,
        ); err != nil {
                if errors.Is(err, pgx.ErrNoRows) {
                        return Worker{}, ErrUnknownWorker
                }
                return Worker{}, fmt.Errorf("worker: query worker: %w", err)
        }
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &worker.Capabilities); err != nil {
			return Worker{}, fmt.Errorf("worker: decode capabilities: %w", err)
		}
	}
	worker.RegisteredAt = worker.RegisteredAt.UTC()
	worker.LastHeartbeatAt = worker.LastHeartbeatAt.UTC()
	return worker, nil
}

// ListWorkers returns all worker rows.
func (r *PostgresRepository) ListWorkers(ctx context.Context) ([]Worker, error) {
	rows, err := r.pool.Query(ctx, listWorkersQuery)
	if err != nil {
		return nil, fmt.Errorf("worker: list workers: %w", err)
	}
	defer rows.Close()

	workers := []Worker{}
	for rows.Next() {
		var (
			payload []byte
			worker  Worker
		)
		if err := rows.Scan(
			&worker.ID,
			&worker.Pool,
			&worker.Endpoint,
			&worker.Version,
			&payload,
			&worker.Status,
			&worker.RegisteredAt,
			&worker.LastHeartbeatAt,
			&worker.ActiveJobs,
		); err != nil {
			return nil, fmt.Errorf("worker: scan worker: %w", err)
		}
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &worker.Capabilities); err != nil {
				return nil, fmt.Errorf("worker: decode capabilities: %w", err)
			}
		}
		worker.RegisteredAt = worker.RegisteredAt.UTC()
		worker.LastHeartbeatAt = worker.LastHeartbeatAt.UTC()
		workers = append(workers, worker)
	}
	return workers, nil
}

// EnsureSchema creates the worker_nodes table if it is absent.
func (r *PostgresRepository) EnsureSchema(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS worker_nodes (
        id TEXT PRIMARY KEY,
        pool TEXT NOT NULL,
        endpoint TEXT NOT NULL,
        version TEXT NOT NULL,
        capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
        status TEXT NOT NULL,
        registered_at TIMESTAMPTZ NOT NULL,
        last_heartbeat_at TIMESTAMPTZ NOT NULL,
        active_jobs INTEGER NOT NULL DEFAULT 0
);
`
	_, err := r.pool.Exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("worker: ensure schema: %w", err)
	}
	return nil
}
