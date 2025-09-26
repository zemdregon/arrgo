# arrgo API - First Iteration

> **Last reviewed:** 2025-03-09
> **Status:** Implemented (REST and gRPC servers are wired into `internal/api/httpapi` and `internal/api/grpcapi`)

The initial API surface exposes lightweight REST and gRPC interfaces for library
management and orchestration. Catalog entities, jobs, and overrides are
persisted in PostgreSQL, while Redis Streams handle orchestration job fan-out
and worker coordination.

## Implementation Notes
- REST routing is provided by the HTTP server in
  `internal/api/httpapi/server.go` with middleware handling RBAC, metrics, and
  logging.
- gRPC services are defined in [`api/proto/arrgo`](../api/proto/arrgo) and served
  via `internal/api/grpcapi/server.go`. Both surfaces call the same domain
  services so behavior remains consistent.
- OpenAPI/Buf generation is planned for a future milestone; for now, this
  document serves as the authoritative contract reference.

## REST Endpoints

Base path: `/api/v1`

> **JSON casing:** Responses reuse the exported Go struct field names. Keys appear in
> upper camel case (for example, `ID`, `EntityType`, `SubmittedAt`). Payloads that are
> constructed as plain maps—such as the worker assignment envelope—use lowercase keys as
> shown below.

### Library endpoints

#### `GET /api/v1/library/entities`
- **Description:** List catalog entities persisted in PostgreSQL. The example
  payload below reflects a single series row stored in the database.
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
        "Synopsis": "...",
        "PendingImports": 2,
        "QualityUpgrades": 1,
        "Failures": 0
      }
    ]
  }
  ```
  Entities include aggregated orchestration counters that surface ingestion
  health in the dashboard. `PendingImports` captures queued import jobs,
  `QualityUpgrades` highlights refresh/upgrade work outstanding, and `Failures`
  tallies jobs that require operator intervention.

#### `GET /api/v1/library/entities/{entity_id}/rename-history`
- **Description:** Return the rename operations recorded for a library entity.
- **Query Parameters:**
  - `limit` (optional) — maximum history entries to return. Must be a
    non-negative integer when supplied.
- **Response:**
  ```json
  {
    "history": [
      {
        "RequestedAt": "2025-01-17T18:00:00Z",
        "RequestedBy": "automation",
        "OriginalPath": "/library/The Expanse S01E01.mkv",
        "ProposedPath": "/library/The Expanse/Season 01/S01E01.mkv",
        "Applied": true
      }
    ]
  }
  ```
- **Responses:**
  - `200 OK` with the rename history payload above (empty array when no history
    is recorded).
  - `400 Bad Request` if `limit` fails validation.
  - `404 Not Found` when the entity does not exist.

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

### Metadata override endpoints

#### `GET /api/v1/library/overrides/{entity_id}`
- **Description:** Retrieve metadata overrides applied to an entity. Returns
  `null` when no overrides exist.
- **Response:**
  ```json
  {
    "overrides": {
      "Title": "The Expanse (2015)",
      "AlternateTitles": ["Expanse"],
      "Episodes": {
        "S01E01": {
          "Title": "Dulcinea"
        }
      }
    }
  }
  ```
- **Responses:** `200 OK` with overrides or `null`; `400 Bad Request` when the
  entity identifier is malformed; `404 Not Found` when the entity does not exist.

#### `PUT /api/v1/library/overrides/{entity_id}`
- **Description:** Replace overrides for an entity. Payload mirrors the response
  structure above.
- **Responses:** `200 OK` with persisted overrides, `400 Bad Request` when the
  payload references unknown episodes/editions/tracks, `404 Not Found` for
  unknown entities.

#### `DELETE /api/v1/library/overrides/{entity_id}`
- **Description:** Remove overrides for an entity.
- **Responses:** `204 No Content` when cleared, with error codes matching the
  `PUT` endpoint for validation failures.

### Manual indexer endpoints

#### `GET /api/v1/indexer/search`
- **Description:** Return the most recent manual search run for an entity.
- **Query Parameters:** `entity_id` (required).
- **Response:**
  ```json
  {
    "search": {
      "EntityID": "series-1",
      "Query": "The Expanse 1080p",
      "Releases": [
        {
          "ID": "release-1",
          "Title": "The Expanse S01E01 1080p WEB-DL",
          "Size": 2147483648,
          "Indexer": "nzbgeek"
        }
      ],
      "RanAt": "2025-01-17T18:10:00Z"
    }
  }
  ```
- **Responses:** `200 OK` with the search payload or `{"search": null}` when no
  search has been run; `400 Bad Request` if the entity identifier is missing;
  `404 Not Found` when the entity does not exist.

#### `POST /api/v1/indexer/search`
- **Description:** Execute a manual search for an entity.
- **Request Body:**
  ```json
  {
    "entity_id": "series-1",
    "query": "The Expanse 1080p"
  }
  ```
- **Responses:** `200 OK` with the payload shown above; validation and not-found
  errors mirror the `GET` endpoint.

#### `POST /api/v1/indexer/releases/grab`
- **Description:** Force grab a release surfaced by a manual search.
- **Request Body:**
  ```json
  {
    "entity_id": "series-1",
    "release_id": "release-1"
  }
  ```
- **Responses:**
  - `200 OK` with the grabbed release payload.
  - `400 Bad Request` when the release was not part of the manual search.
  - `404 Not Found` when the entity or release cannot be located.
  - `409 Conflict` when the release was already grabbed.

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
- **Description:** Update heartbeat state for a worker. When the optional
  `active_jobs` field is provided, the worker's in-flight job count is updated.
- **Request Body:**
  ```json
  {
    "active_jobs": 3
  }
  ```
- **Responses:**
  - `200 OK` with the worker payload plus a `stale` flag indicating whether the
    worker exceeded its stale threshold.
  - `400 Bad Request` when the worker identifier is missing.
  - `404 Not Found` when the worker has not registered.

#### `GET /api/v1/worker-pools`
- **Description:** List worker pool configuration.
- **Response:**
  ```json
  {
    "pools": [
      {
        "pool": "transcode",
        "description": "GPU-enabled transcode jobs",
        "stream": "arrgo:jobs:transcode",
        "consumer_group": "arrgo-transcode",
        "heartbeat_interval_seconds": 15,
        "stale_after_seconds": 60,
        "max_concurrent_jobs": 4
      }
    ]
  }
  ```

#### `PUT /api/v1/worker-pools/{pool}`
- **Description:** Create or update a worker pool configuration. Omitted fields
  retain their previous values.
- **Responses:**
  - `200 OK` with the updated pool payload shown above.
  - `400 Bad Request` when validation fails (unknown pool name, invalid
    heartbeat intervals, etc.).
  - `503 Service Unavailable` when worker coordination is disabled.

### Request management endpoints *(in development)*

Upcoming backend deliveries introduce persistent request tracking, approvals,
and quotas. The API contracts below will be exercised by the dashboard as soon
as the services are promoted from feature branches; they are documented now to
unblock client work.

#### `GET /api/v1/requests`
- **Description:** List requests with pagination and filtering by `status`,
  `policy`, and `kind`.
- **Query Parameters:**
  - `status`, `policy`, `kind` (optional filters)
  - `cursor`, `limit` (pagination controls)
- **Response:**
  ```json
  {
    "requests": [
      {
        "ID": "req-42",
        "Kind": "series",
        "Title": "The Expanse",
        "RequestedBy": "sarah@arrgo.dev",
        "SubmittedAt": "2025-01-20T15:30:00Z",
        "Status": "pending",
        "Policy": "Public requests limited to 20 per rolling week",
        "QuotaRemaining": 5,
        "SLADeadline": "2025-01-22T15:30:00Z"
      }
    ],
    "next_cursor": null
  }
  ```

#### `POST /api/v1/requests`
- **Description:** Submit a new request for approval.
- **Request Body:**
  ```json
  {
    "kind": "series",
    "external_ids": {
      "tmdb": "63639"
    },
    "requested_by": "sarah@arrgo.dev",
    "notes": "Please monitor 1080p releases"
  }
  ```
- **Responses:**
  - `202 Accepted` with the queued request payload.
  - `400 Bad Request` if identifiers or requester metadata are missing.
  - `409 Conflict` when an equivalent active request already exists.

#### `POST /api/v1/requests/{request_id}/approve`
- **Description:** Approve a request and enqueue automation jobs.
- **Request Body:**
  ```json
  {
    "approved_by": "ops@arrgo.dev",
    "notes": "Fits within quota"
  }
  ```
- **Responses:** `200 OK` with the updated request payload; `409 Conflict` if
  the request has already been finalized.

#### `POST /api/v1/requests/{request_id}/deny`
- **Description:** Deny a request with an optional note. Returns the updated
  request payload with status `denied`.

#### `POST /api/v1/requests/{request_id}/cancel`
- **Description:** Allow requesters to withdraw pending requests. Returns the
  updated payload with status `cancelled`.

#### `GET /api/v1/requests/{request_id}/timeline`
- **Description:** Retrieve a chronological event log for a request including
  approvals, denials, automation hand-offs, and notification deliveries.

### Observability endpoints

- `GET /metrics` exposes Prometheus metrics from the Go services and registered subsystem
  collectors.
- `GET /debug/pprof/*` provides profiling handlers (`profile`, `heap`, etc.) for local and
  production diagnostics.

## Error Handling & Versioning
- REST handlers follow HTTP semantics with JSON error envelopes that include a
  machine-readable `code` field and human-friendly `message`.
- gRPC status codes map directly from domain errors. Invalid input raises
  `INVALID_ARGUMENT`, unknown resources return `NOT_FOUND`, and disabled
  subsystems report `UNAVAILABLE`.
- Versioning follows the `/api/v1` prefix and `arrgo.api.v1` protobuf package.
  Backward-incompatible changes require a new version namespace.

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

## Future Enhancements
- Replace the in-memory repositories with Postgres-backed stores and expose
  pagination, filtering, and sorting options that leverage SQL indexes.
- Publish OpenAPI and protobuf documentation sites (e.g., via Stoplight or Buf
  Docs) so integrators can self-serve schema references.
- Introduce long-running operation resources for multi-step workflows (e.g.,
  manual search overrides) once asynchronous orchestration flows expand.
