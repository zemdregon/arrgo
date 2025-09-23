# Observability Baseline

This document captures the initial observability plan for arrgo. It defines the
metrics, traces, and logs that every service must emit as well as the baseline
dashboards and alerting hooks expected in the alpha release. The goal is to
provide contributors and operators with a consistent, actionable signal across
the master and worker services.

## Guiding Principles

- **Instrument everything that matters.** Expose metrics for request/response
  flows, job orchestration, caching, and any long-running background activity.
- **Prefer open standards.** Reuse Prometheus, OpenTelemetry, and structured
  logging libraries to minimize integration friction.
- **Correlate signals.** Ensure metrics, traces, and logs share common labels
  such as job ID, library ID, and worker pool so issues can be triaged quickly.
- **Automate validation.** Add CI checks (linting/tests) to ensure required
  instrumentation exists when new services or endpoints are added.

## Metrics

arrgo adopts Prometheus as the system of record for metrics. All binaries must
expose an `/metrics` HTTP endpoint using the Go OpenTelemetry SDK with the
Prometheus exporter. Each metric should be namespaced under `arrgo_`.

### Core Metrics

| Metric | Type | Description |
| --- | --- | --- |
| `arrgo_http_requests_total` | Counter | Total REST/gRPC requests handled by service and method, labeled with response code and outcome. |
| `arrgo_http_request_duration_seconds` | Histogram | Latency distribution for API handlers. Buckets should capture sub-10ms through 30s to cover metadata lookups and ingest jobs. |
| `arrgo_jobs_enqueued_total` | Counter | Number of jobs created by type (scan, import, transcode, metadata refresh). |
| `arrgo_jobs_running` | Gauge | Count of in-flight jobs per worker pool. |
| `arrgo_job_duration_seconds` | Histogram | Execution time for jobs, labeled with status (success, retry, failed). |
| `arrgo_worker_heartbeat_seconds` | Gauge | Time since the most recent heartbeat per worker instance. Enables stale worker detection. |
| `arrgo_cache_hit_ratio` | Gauge | Rolling hit ratio for Redis-backed metadata caches. |
| `arrgo_db_query_duration_seconds` | Histogram | Latency for Postgres queries grouped by DAO or query label. |
| `arrgo_external_dependency_failures_total` | Counter | Number of failures when calling indexers, download clients, or metadata providers. |

### Service-Specific Additions

- **Master:**
  - `arrgo_scheduler_backlog` (Gauge) for queued but unscheduled jobs.
  - `arrgo_event_dispatch_failures_total` (Counter) for Redis stream publish
    failures.
- **Worker:**
  - `arrgo_worker_queue_lag_seconds` (Gauge) measuring delay between job
    creation and start.
  - `arrgo_transcode_resource_seconds_total` (Counter) tracking CPU/GPU seconds
    consumed by transcoding tasks.

### Metrics Collection

- Deploy Prometheus and Alertmanager alongside the stack (see
  `deploy/compose`).
- Scrape master and worker `/metrics` endpoints every 15 seconds.
- Configure service discovery labels for `cluster`, `service`, `instance`, and
  `worker_pool` to power multi-cluster dashboards.
- Record rules to compute availability SLIs (e.g., request success ratio,
  scheduler latency P95).

## Tracing

OpenTelemetry tracing is required for cross-service observability. The Go SDK
should instrument HTTP/gRPC handlers, Redis interactions, and database calls.

### Trace Requirements

- Propagate W3C `traceparent` headers through all HTTP/gRPC calls.
- Wrap job scheduling and execution with parent spans so the full lifecycle is
  traceable across master and worker nodes.
- Attach attributes: `job.id`, `job.type`, `media.id`, `media.type`,
  `worker.pool`, `download.client`, and `retry.count` where applicable.
- Record span events for retries, slow metadata fetches, and cache misses.
- Export traces to the OpenTelemetry Collector via OTLP/HTTP. The collector can
  forward traces to Jaeger, Tempo, or any backend chosen by deployments.

### Sampling Strategy

- Default to parent-based 10% probability sampling for API and scheduler spans.
- Always sample failed requests and spans with duration > 5 seconds using span
  processors.
- Allow deployments to override sampling via environment variables.

## Logging

Structured logging enables reliable correlation and downstream analysis.

- Use a JSON logger (e.g., `zap` or `zerolog`) with RFC3339 timestamps, log
  levels, and contextual fields.
- Minimum fields: `service`, `version`, `instance`, `trace_id`, `span_id`,
  `job.id`, `media.id` (if available), and `worker.pool`.
- Emit logs at `info` by default. Use `debug` for high-frequency instrumentation
  and `error` for failures. Avoid `fatal` except for unrecoverable conditions.
- Integrate log rotation via container runtime (Docker) and ship logs to a
  centralized sink (e.g., Loki, Elasticsearch) through Fluent Bit or Vector.

## Dashboards

Grafana is the default dashboarding tool. Provide the following baseline
Dashboards under `docs/dashboards` (to be added when JSON definitions are ready):

1. **Service Health Overview**
   - API request rate, error rate, and latency (P50/P95/P99).
   - Worker heartbeat age and queue lag.
   - Database connection saturation and query latency.
2. **Job Orchestration**
   - Jobs enqueued/completed by type.
   - Job duration percentiles by worker pool.
   - Retries and failures by external dependency.
3. **Metadata Ingestion**
   - Cache hit ratio, call volume to metadata providers.
   - Slowest providers by average response time.
   - Rate of stale metadata refreshes.
4. **Infrastructure**
   - CPU/memory/disk usage for master and worker containers (integrate cAdvisor
     metrics).
   - Redis latency and pending commands.
   - Postgres throughput (transactions/sec) and replication lag (if enabled).

The Grafana folder should ship with variables for `cluster`, `service`,
`worker_pool`, and `media_type` to enable multi-tenant views.

## Alerting

Alerting rules should balance responsiveness with noise reduction.

- **High error rate:** Trigger when `arrgo_http_requests_total` failure ratio
  exceeds 5% for 5 minutes.
- **Scheduler backlog:** Alert if `arrgo_scheduler_backlog` grows for 10 minutes
  without downward trend.
- **Worker heartbeat stale:** Page when `arrgo_worker_heartbeat_seconds` exceeds
  120 seconds.
- **Job retries:** Warn when retry count per job type exceeds baseline for 15
  minutes.
- Route alerts through Alertmanager with receivers for on-call and Slack.

## Operational Checklist

- [x] Master and worker binaries expose `/metrics` and `/debug/pprof` endpoints.
- [ ] Services propagate trace context and export spans to the collector.
- [ ] Structured logs include trace IDs and job identifiers.
- [ ] Prometheus, Grafana, Alertmanager, and the OpenTelemetry Collector are
      included in development and production manifests.
- [ ] Dashboards and alert rules are version-controlled within the repository.

## Next Steps

- Automate validation of observability instrumentation via CI linting.
- Provide example Grafana JSON dashboards and Alertmanager configuration files
  under `docs/` or `deploy/` directories.
- Extend observability coverage to the UI once it lands.
