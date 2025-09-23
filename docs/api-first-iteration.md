# arrgo API - First Iteration

The initial API surface exposes lightweight REST and gRPC interfaces for library
management and orchestration. The implementations use an in-memory catalog so
the endpoints can be exercised before the PostgreSQL backing store lands, while
Redis Streams now handle orchestration job fan-out and worker coordination.

## REST Endpoints

Base path: `/api/v1`

### `GET /api/v1/library/entities`
- **Description:** List catalog entities.
- **Query Parameters:**
  - `type` (optional) — filter by entity type (`series`, `movie`, `artist`, etc.).
- **Response:**
  ```json
  {
    "entities": [
      {
        "id": "series-1",
        "entity_type": "series",
        "title": "The Expanse",
        "synopsis": "..."
      }
    ]
  }
  ```

### `GET /api/v1/jobs`
- **Description:** List orchestration jobs ordered by submission time.
- **Response:**
  ```json
  {
    "jobs": [
      {
        "id": "job-123",
        "job_type": "refresh",
        "target_id": "series-1",
        "state": "queued",
        "submitted_at": "2024-05-01T12:00:00Z",
        "payload_uri": "s3://arrgo/jobs/job-123.json",
        "idempotency_key": "refresh-series-1",
        "attempt": 1,
        "last_attempt_at": "2024-05-01T12:00:05Z"
      }
    ]
  }
  ```

### `POST /api/v1/jobs`
- **Description:** Schedule a new orchestration job.
- **Request Body:**
  ```json
  {
    "job_type": "refresh",
    "target_id": "series-1",
    "payload_uri": "s3://arrgo/jobs/job-123.json",
    "idempotency_key": "refresh-series-1"
  }
  ```
- **Responses:**
  - `201 Created` with the persisted job payload.
  - `400 Bad Request` if inputs are missing.
  - `404 Not Found` when the target entity is unknown.
  - `409 Conflict` when a duplicate idempotency key is supplied.
  - `503 Service Unavailable` if the orchestration queue is unreachable.

## gRPC Service

Package: `arrgo.api.v1`

### Service: `LibraryService`
- **`ListLibraryEntities(ListLibraryEntitiesRequest)`** → `ListLibraryEntitiesResponse`
  - Optional `entity_type` filter matches the REST query parameter.
- **`ListJobs(ListJobsRequest)`** → `ListJobsResponse`
  - Returns queued jobs ordered by submission time.
- **`TriggerJob(TriggerJobRequest)`** → `TriggerJobResponse`
  - Validates the target entity and returns a queued job summary.

Proto definition: [`api/proto/arrgo/library.proto`](../api/proto/arrgo/library.proto)

The gRPC server shares the same in-memory service logic as the REST handlers so
behavior remains consistent across protocols.
