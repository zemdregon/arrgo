# arrgo Project Status

## Highlights
- arrgo targets a unified master/worker architecture that replaces the Sonarr/Radarr/Lidarr stack with a Go-based service tier backed by PostgreSQL, Redis, and optional search infrastructure.【F:README.md†L3-L38】
- The repository already documents and tracks a broad set of completed milestones (metadata pipeline hardening, orchestration queue, worker coordination, dashboard shell) with the next immediate UI goal called out separately.【F:README.md†L50-L69】
- The README now outlines an extensibility roadmap spanning plugin categories (download clients, notifications, metadata, indexers, media servers, automation, storage, analysis, and authentication) plus labeled API key workflows.【F:README.md†L115-L154】
- Automated indexer health scoring, discovery, and failover-threshold recommendations are now documented alongside the new monitoring package, reducing manual tuning for search integrations.【F:docs/indexer-health-scoring.md†L1-L73】【F:internal/integrations/indexer/monitor.go†L1-L358】
- Release profile scoring with reusable custom formats, group/codec preferences, and Servarr import helpers is documented for operators migrating complex automation rules.【F:docs/release-profile-scoring.md†L1-L143】【F:internal/automation/releaseprofile/profile.go†L1-L108】【F:internal/automation/releaseprofile/import_sonarr.go†L1-L337】

## What works today
### Library orchestration
- The library service models catalog entities and orchestration jobs, supports filtered entity listings, and maintains job history with submission timestamps, state transitions, and audit hooks for completed imports.【F:internal/library/service.go†L17-L337】
- Redis Streams integration enforces idempotency, consumer-group coordination, acknowledgements, and retries through the orchestration queue wrapper.【F:internal/library/queue.go†L22-L200】
- A cron-backed scheduler fans out scan/import jobs while respecting per-entity-type concurrency windows and queue availability, with accompanying documentation for operators.【F:internal/library/scheduler.go†L14-L214】【F:docs/library-scheduling.md†L1-L100】

### Worker coordination
- Worker registration validates pool metadata, persists the worker in a repository (with PostgreSQL DDL available) and Redis cache, and returns queue assignment plus heartbeat cadence information.【F:internal/worker/service.go†L67-L208】【F:internal/worker/repository.go†L14-L233】
- Heartbeat handling refreshes activity timestamps, normalizes active job counters, and drives Prometheus gauges that track active, stale, and per-worker heartbeat ages.【F:internal/worker/service.go†L155-L322】【F:internal/worker/metrics.go†L1-L35】
- The coordination design, including REST/gRPC flows and failure behavior, is captured in dedicated documentation for contributors and operators.【F:docs/worker-coordination.md†L1-L132】

### Metadata ingestion
- The pipeline coordinates cache lookups, concurrent provider fan-out, aggregation with priority-based field merging, and Redis persistence, while honoring provider-specific TTL overrides and negative caching for missing entities.【F:internal/metadata/pipeline.go†L19-L521】
- Built-in rate-limit handling, exponential backoff, OpenTelemetry spans, and Prometheus counters harden the ingestion workflow; the documentation mirrors these responsibilities and execution flow.【F:internal/metadata/pipeline.go†L323-L470】【F:docs/metadata-ingestion.md†L1-L70】

### API surfaces and observability
- REST handlers expose library entity listings, job submission/listing, and worker coordination flows alongside `/metrics` and `/debug/pprof` endpoints for runtime insight.【F:internal/api/httpapi/server.go†L18-L255】
- The gRPC server mirrors library and worker APIs with appropriate status mappings, enabling clients to adopt either protocol without behavioral drift.【F:internal/api/grpcapi/server.go†L15-L178】

### Dashboard experience
- A Next.js dashboard pulls library entities, jobs, and worker metadata via typed fetch helpers, summarizing totals, breakdowns, and latest activity with reusable panels, retry-aware data hooks, interactive job queue controls, queue analytics, and job drill-down drawers.【F:web/dashboard/app/page.tsx†L1-L420】【F:web/dashboard/lib/api.ts†L1-L260】【F:web/dashboard/hooks/useApiData.ts†L1-L31】
- Worker pool configuration is editable from the dashboard, with forms backed by new REST endpoints that persist heartbeat cadence, queue assignments, and concurrency limits.【F:web/dashboard/app/page.tsx†L420-L640】【F:internal/api/httpapi/server.go†L18-L280】【F:internal/worker/service.go†L1-L430】【F:docs/dashboard-operations.md†L1-L27】

### Automated verification
- Unit tests cover library validation, idempotency, queue claim/ack, retries, and audit emission, giving confidence that orchestration primitives behave as expected.【F:internal/library/service_test.go†L57-L214】
- Scheduler tests confirm concurrency limits, while worker tests verify persistence, caching, stale detection, and metric updates under time shifts.【F:internal/library/scheduler_test.go†L8-L102】【F:internal/worker/service_test.go†L18-L191】
- Metadata pipeline tests exercise cache hits/misses, provider aggregation, negative caching, TTL overrides, and rate-limit retries; HTTP and gRPC adapter suites assert end-to-end request handling and observability endpoints.【F:internal/metadata/pipeline_test.go†L15-L360】【F:internal/api/httpapi/server_test.go†L20-L200】【F:internal/api/grpcapi/server_test.go†L68-L164】

## Outstanding work and risks
- Observability checklist items remain open for trace propagation, log enrichment, and packaging of dashboards/alerting assets, highlighting instrumentation and operational follow-up work.【F:docs/observability-baseline.md†L130-L138】
- Library data currently lives in memory with default seed entities, so delivering the roadmap’s Postgres-backed CRUD layer is still ahead.【F:internal/library/service.go†L147-L200】【F:README.md†L92-L96】
- Published Docker Compose assets launch placeholder commands until binaries are ready, underscoring the need for build integration and deployment hardening.【F:README.md†L74-L79】
- Broader roadmap items—indexer/download client integrations, policy automation, search abstraction, multi-tenancy, single sign-on, HA workers, analytics, and a multi-surface plugin ecosystem with per-plugin credentials—remain future milestones spanning the beta and GA phases.【F:README.md†L98-L154】

## Upcoming milestones
- Alpha priorities emphasize completing Postgres-backed catalog CRUD, solidifying the distributed Redis queue/worker model, polishing the minimal dashboard, and shipping containerized observability tooling.【F:README.md†L92-L96】
- Beta targets expand integrations (indexers/download clients), policy automation, search, multi-tenant security with labeled API keys, and single sign-on support, while GA layers on HA orchestration, analytics, and a curated plugin marketplace.【F:README.md†L98-L150】
