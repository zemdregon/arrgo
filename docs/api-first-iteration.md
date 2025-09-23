# arrgo API - First Iteration

The initial API surface exposes lightweight REST and gRPC interfaces for library
management and orchestration. The implementations use an in-memory catalog so
the endpoints can be exercised before the PostgreSQL backing store lands, while
Redis Streams now handle orchestration job fan-out and worker coordination.

## REST Endpoints

Base path: `/api/v1`

> **JSON casing:** Responses reuse the exported Go struct field names. Keys appear in
> upper camel case (for example, `ID`, `EntityType`, `SubmittedAt`). Payloads that are
> constructed as plain maps—such as the worker assignment envelope—use lowercase keys as
> shown below.

### Library endpoints

#### `GET /api/v1/library/entities`
- **Description:** List catalog entities. Results default to a seeded in-memory catalog
  until PostgreSQL persistence is wired in.
- **Query Parameters:**
  - `type` (optional) — filter by entity type (`series`, `movie`, `artist`, etc.).
- **Response:**
  ```json
  {
    "entities": [
      {
        "ID": "series-1",
        "EntityType": "series",
        "Title": "The Expanse",
        "Synopsis": "..."
      }
    ]
  }
  ```

#### `GET /api/v1/jobs`
- **Description:** List orchestration jobs ordered by submission time (newest first).
- **Response:**
  ```json
  {
    "jobs": [
      {
        "ID": "job-123",
        "JobType": "scan",
        "TargetID": "series-1",
        "State": "queued",
        "SubmittedAt": "2024-05-01T12:00:00Z",
        "PayloadURI": "s3://arrgo/jobs/job-123.json",
        "IdempotencyKey": "scan-series-1",
        "Attempt": 0,
        "LastAttemptAt": ""
      }
    ]
  }
  ```
  `CompletedAt` is populated when a job transitions to `completed`.

#### `POST /api/v1/jobs`
- **Description:** Schedule a new orchestration job. Common job types today are
  `scan` and `import` (see [`internal/library/service.go`](../internal/library/service.go)).
- **Request Body:**
  ```json
  {
    "job_type": "scan",
    "target_id": "series-1",
    "payload_uri": "s3://arrgo/jobs/job-123.json",
    "idempotency_key": "scan-series-1"
  }
  ```
- **Responses:**
  - `201 Created` with the queued job payload (fields as shown in the `GET /api/v1/jobs`
    response).
  - `400 Bad Request` if inputs are missing or malformed.
  - `404 Not Found` when the target entity is unknown.
  - `409 Conflict` when a duplicate idempotency key is supplied.
  - `503 Service Unavailable` if the Redis-backed orchestration queue is not configured.

### Worker endpoints

#### `GET /api/v1/workers`
- **Description:** Return registered workers. The service serves cached results from Redis
  when available and falls back to PostgreSQL or the in-memory repository otherwise.
- **Response:**
  ```json
  {
    "workers": [
      {
        "ID": "worker-123",
        "Pool": "transcode",
        "Capabilities": ["gpu", "hevc"],
        "Endpoint": "http://worker-1:9000",
        "Version": "1.2.3",
        "Status": "healthy",
        "RegisteredAt": "2025-01-01T12:00:00Z",
        "LastHeartbeatAt": "2025-01-01T12:00:00Z",
        "ActiveJobs": 0
      }
    ]
  }
  ```

#### `POST /api/v1/workers/register`
- **Description:** Register or update a worker. Registration is idempotent and returns the
  Redis Streams assignment the worker should join.
- **Request Body:**
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
      "ID": "worker-123",
      "Pool": "transcode",
      "Capabilities": ["gpu", "hevc"],
      "Endpoint": "http://worker-1:9000",
      "Version": "1.2.3",
      "Status": "healthy",
      "RegisteredAt": "2025-01-01T12:00:00Z",
      "LastHeartbeatAt": "2025-01-01T12:00:00Z",
      "ActiveJobs": 0
    },
    "assignment": {
      "stream": "arrgo:library:jobs",
      "consumer_group": "arrgo-library-workers",
      "heartbeat_interval_seconds": 15
    }
  }
  ```
- **Responses:**
  - `201 Created` for successful registrations.
  - `400 Bad Request` when the pool name is missing.
  - `503 Service Unavailable` when worker coordination is disabled on the server.

#### `POST /api/v1/workers/{worker_id}/heartbeat`
- **Description:** Update a worker heartbeat and active job count.
- **Request Body:**
  ```json
  {
    "active_jobs": 2
  }
  ```
- **Response:**
  ```json
  {
    "worker": {
      "ID": "worker-123",
      "Pool": "transcode",
      "Capabilities": ["gpu", "hevc"],
      "Endpoint": "http://worker-1:9000",
      "Version": "1.2.3",
      "Status": "healthy",
      "RegisteredAt": "2025-01-01T12:00:00Z",
      "LastHeartbeatAt": "2025-01-01T12:00:15Z",
      "ActiveJobs": 2
    },
    "stale": false
  }
  ```
- **Responses:**
  - `200 OK` when the heartbeat is processed.
  - `400 Bad Request` for missing IDs.
  - `404 Not Found` when the worker is unknown.

### Observability endpoints

- `GET /metrics` exposes Prometheus metrics from the Go services and registered subsystem
  collectors.
- `GET /debug/pprof/*` provides profiling handlers (`profile`, `heap`, etc.) for local and
  production diagnostics.

## gRPC Services

Package: `arrgo.api.v1`

### `LibraryService` (see [`api/proto/arrgo/library.proto`](../api/proto/arrgo/library.proto))
- `ListLibraryEntities(ListLibraryEntitiesRequest)` → `ListLibraryEntitiesResponse`
  - Optional `entity_type` filter matches the REST query parameter.
- `ListJobs(ListJobsRequest)` → `ListJobsResponse`
  - Returns queued jobs ordered by submission time.
- `TriggerJob(TriggerJobRequest)` → `TriggerJobResponse`
  - Validates the target entity and returns a queued job summary.

### `WorkerService` (see [`api/proto/arrgo/worker.proto`](../api/proto/arrgo/worker.proto))
- `RegisterWorker(WorkerRegistrationRequest)` → `WorkerRegistrationResponse`
  - Returns the worker descriptor plus Redis Streams assignment.
- `Heartbeat(HeartbeatRequest)` → `HeartbeatResponse`
  - Refreshes heartbeat metadata and reports whether the worker is considered stale.

The gRPC server shares the same in-memory service logic as the REST handlers so behavior
remains consistent across protocols. Errors map to standard gRPC status codes: invalid
arguments return `INVALID_ARGUMENT`, missing resources return `NOT_FOUND`, and disabled
subsystems yield `UNAVAILABLE`.
