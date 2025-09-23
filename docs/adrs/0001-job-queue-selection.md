# ADR 0001: Redis Streams Orchestration Queue

- Status: Accepted
- Date: 2024-07-13
- Owners: Platform Engineering

## Context
arrgo coordinates ingest, scan, upgrade, and notification workloads through a master/worker
model. The master must persist job submissions durably, order work predictably,
and fan out jobs to heterogeneous worker pools without sacrificing observability
or multi-tenant isolation. Early bootstrap prototypes used in-memory queues,
but they suffer from the following issues:

- No resilience to master restarts or process crashes.
- Lack of ordering guarantees once multiple workers are involved.
- No built-in acknowledgement mechanism or retry tracking.
- Limited introspection for operational tooling and metrics.

We already depend on Redis for caching, token bucket rate limits, and
low-latency coordination. Leveraging Redis for orchestration avoids
introducing another stateful dependency while still meeting durability and
throughput goals (sub-millisecond append latency with tens of thousands of jobs
per minute on modest hardware).

## Decision
Use Redis Streams as the authoritative job queue. The master node writes jobs to
stream namespaces partitioned by workload (`jobs:series`, `jobs:movies`, etc.).
Each worker pool joins a consumer group on the relevant stream. Workers read via
`XREADGROUP`, acknowledge with `XACK` once processing finishes, and rely on
pending-entry ownership transfers for retries.

### Stream layout
- `jobs:<category>` – primary stream storing enqueued jobs. Stream entries carry
  structured fields: `job_id`, `job_type`, `target_id`, `priority`, `attempt`,
  and `submitted_at` (RFC 3339).
- `jobs:<category>:dead` – dead-letter stream for exhausted jobs. Entries mirror
  the primary schema and include a `failed_at` timestamp and `failure_reason`.
- `jobs:<category>:metrics` – optional rolling stream for coarse-grained metrics.

### Submission
1. Master validates the request through `internal/library.Service` and assigns a
   UUID (`job_id`).
2. The job is appended via `XADD jobs:<category>` with `MAXLEN ~ 10000` to bound
   history while keeping recent backlog visible. The Redis-assigned stream ID is
   stored alongside the job record in PostgreSQL for auditability.
3. The master publishes a lightweight `PUBLISH jobs.dispatch` notification so
   idle workers wake immediately instead of polling.

### Consumption and acknowledgements
- Workers join a consumer group matching their pool name (e.g., `workers:4k`).
- Each worker issues `XREADGROUP GROUP workers:4k <instance-id> COUNT 10 BLOCK
  5000 STREAMS jobs:<category> >` to fetch up to 10 jobs.
- The worker marks the job `running` via the API, executes the orchestration
  pipeline, then calls `XACK` to remove the entry from the pending list.
- Workers expose Prometheus metrics for latency, retries, and success/failure
  totals partitioned by job type and pool.

### Retry strategy
- The master sets `attempt=0` on submission. Workers increment the field locally
  when they fail processing and include it when requeueing.
- A failed job is re-enqueued with exponential backoff: `next_delay =
  min(base * 2^attempt, max_backoff)` where `base = 30s` and `max_backoff = 30m`.
  Jitter of ±15% is added to avoid thundering herds.
- Workers NACK failures by leaving the entry pending. A watchdog goroutine scans
  `XPENDING` every 15 seconds. Entries idle longer than `visibility_timeout`
  (default 2 minutes) are claimed with `XCLAIM` and reprocessed by another
  worker, updating `attempt`.
- After `max_attempts` (default 5) the job is moved to the corresponding
  dead-letter stream, marked `failed` in PostgreSQL, and surfaced to operators
  via the API/notification layer.

### Operational considerations
- Streams are namespaced per environment (e.g., `prod:jobs:series`) to prevent
  cross-environment interference.
- Redis persistence must enable AOF with `appendfsync everysec` to retain writes
  across restarts without unacceptable latency.
- Dashboards track `stream_length`, `pending_idle_ms`, and retry counters. Alert
  if visibility timeouts spike or dead-letter volume exceeds SLOs.
- Backups snapshot both PostgreSQL and Redis to preserve the full job and retry
  history.

## Consequences
### Positive
- Reuses existing Redis dependency, reducing operational surface area.
- Supports horizontal scale by adding worker consumers without rebalancing.
- Provides deterministic ordering within a stream and visibility into pending
  entries for observability.
- Enables targeted retries and dead-letter handling without bespoke plumbing.

### Negative
- Redis Streams require careful consumer management; leaked consumers leave
  pending entries that must be reaped.
- Stream retention must be tuned to avoid unbounded memory growth.
- High-throughput clusters may need Redis clustering or sharding strategies to
  prevent a single node from becoming a bottleneck.

## Alternatives considered
- **RabbitMQ / NATS JetStream:** Provide durable queues, but introduce another
  stateful dependency and operational burden. They were rejected to keep the
  control plane lean during early releases.
- **PostgreSQL advisory locks:** Simplifies dependencies but cannot match the
  required throughput or latency, and would couple transaction processing with
  job dispatch.
- **Kafka:** Excellent durability and replay semantics, yet overkill for the
  relatively small payloads and strict ordering per entity that arrgo requires.

## Follow-up work
- Implement Redis stream client helpers within `internal/library` to replace the
  in-memory job slice.
- Extend API responses with `attempt`, `next_visible_at`, and dead-letter
  metadata.
- Add operational runbooks documenting redis-cli commands for consumer group
  management and emergency draining.
