# Worker Coordination

The worker coordination subsystem keeps track of every worker node in the cluster,
exposes a registration handshake, records periodic heartbeats, and continuously updates
metrics so operators can quickly identify stale or disconnected nodes. The master service
persists worker metadata in PostgreSQL, mirrors hot data in Redis for low-latency lookups,
and surfaces a consistent REST and gRPC API for worker runtimes.

## Registration Handshake

Workers must register before they can consume orchestration jobs. Registration is idempotent
and returns queue metadata so the worker can join the appropriate Redis consumer group.

### REST

- **Endpoint:** `POST /api/v1/workers/register`
- **Payload:**
  ```json
  {
    "pool": "transcode",
    "capabilities": ["gpu", "hevc"],
    "endpoint": "http://worker-1:9000",
    "version": "1.2.3"
  }
  ```
- **Response:**
  ```json
  {
    "worker": {
      "id": "worker-123",
      "pool": "transcode",
      "status": "healthy",
      "registered_at": "2025-01-01T12:00:00Z",
      "last_heartbeat_at": "2025-01-01T12:00:00Z"
    },
    "assignment": {
      "stream": "arrgo:library:jobs",
      "consumer_group": "arrgo-library-workers",
      "heartbeat_interval_seconds": 15
    }
  }
  ```

### gRPC

- **Service:** `WorkerService`
- **Method:** `RegisterWorker(WorkerRegistrationRequest) returns (WorkerRegistrationResponse)`
- **Fields:** mirror the REST payload/response. Timestamps use RFC 3339 formatting.

## Heartbeats

Workers must call the heartbeat endpoint at the interval communicated during registration.
Heartbeat calls update the active job count, refresh the last heartbeat timestamp, and
flag stale nodes whose heartbeats exceed the configured `StaleAfter` window.

### REST

- **Endpoint:** `POST /api/v1/workers/{worker_id}/heartbeat`
- **Payload:**
  ```json
  {
    "active_jobs": 2
  }
  ```
- **Response:**
  ```json
  {
    "worker": {
      "id": "worker-123",
      "status": "healthy",
      "active_jobs": 2,
      "last_heartbeat_at": "2025-01-01T12:00:15Z"
    },
    "stale": false
  }
  ```

### gRPC

- **Method:** `Heartbeat(HeartbeatRequest) returns (HeartbeatResponse)`
- **Semantics:** identical to the REST flow.

## Persistence Model

Worker metadata is stored in PostgreSQL and cached in Redis for fast lookups and
concurrency-friendly updates.

```sql
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
```

The Redis cache indexes workers at `arrgo:workers:index` and stores JSON snapshots under
`arrgo:worker:<worker_id>` with a TTL derived from the heartbeat configuration. Cache reads
are opportunistic; the service always falls back to PostgreSQL on cache misses.

## Metrics

Three Prometheus gauges describe cluster health:

| Metric | Description | Labels |
| --- | --- | --- |
| `arrgo_workers_active` | Number of workers currently heartbeating within the configured threshold. | `pool` |
| `arrgo_workers_heartbeat_seconds` | Age in seconds of the latest heartbeat per worker. | `pool`, `worker_id` |
| `arrgo_workers_stale` | Number of workers marked stale because their heartbeat exceeded the threshold. | `pool` |

The service automatically reconciles metrics whenever a registration or heartbeat occurs,
and exposes a `Reconcile` helper for scheduled evaluations.

## Failure Handling

- Registration validates that a pool name is provided and returns HTTP 400 / gRPC `INVALID_ARGUMENT`
  otherwise.
- Heartbeats without a worker ID return HTTP 400 / gRPC `INVALID_ARGUMENT`.
- Unknown workers return HTTP 404 / gRPC `NOT_FOUND`.
- When the worker subsystem is disabled, REST and gRPC handlers emit `503 Service Unavailable` / `UNAVAILABLE`.

## Operational Notes

- Default heartbeat interval: **15 seconds**.
- Default staleness threshold: **45 seconds** (three missed heartbeats).
- Workers marked stale remain persisted with `status="stale"` until they send a new heartbeat,
  at which point the status transitions back to `healthy`.
