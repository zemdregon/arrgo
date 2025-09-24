# arrgo

> A horizontally scalable, Go-first automation platform that unifies television,
> movie, and music management behind one API and operations model.

## Vision and Value Proposition
arrgo replaces the patchwork of *arr services with a single control plane and a
consistent automation experience. The project is designed for homelab
enthusiasts that have outgrown the traditional single-node model and for media
teams that expect cloud-native resiliency, observability, and policy controls.

The core objectives that drive every milestone are:

- **Unify the media automation ecosystem.** Television, film, and music share a
  canonical domain model, scheduling engine, and orchestration queue so that new
  features automatically apply across media types.
- **Deliver predictable performance at scale.** Go-based services, Redis-backed
  coordination, and aggressive caching keep libraries responsive even as the
  catalog grows into tens of thousands of entities and automation frequency
  increases.
- **Simplify distributed operations.** The master/worker topology is tuned for
  hybrid deployments—run everything on a single host, or stretch workers across
  availability zones while keeping orchestration latency low.
- **Make observability and security first-class.** Structured logging,
  Prometheus metrics, OpenTelemetry tracing, and API key–scoped RBAC are built
  into the platform instead of treated as afterthoughts.

## Platform Snapshot
| Capability | Current Status |
| --- | --- |
| **Language & runtime** | Go 1.23 services with generated REST and gRPC surfaces. |
| **Data stores** | PostgreSQL (authoritative catalog), Redis (caching + queues), optional OpenSearch/Elasticsearch for search. |
| **Automation** | Redis Streams orchestration queue, cron-backed scheduler, worker pools with heartbeats and failover. |
| **Metadata** | Multi-provider ingestion pipeline with caching, backoff, and override tooling. |
| **Integrations** | Download client adapters, indexer health scoring, list ingestion SDKs, and plugin contract tests. |
| **User experience** | Next.js dashboard for visibility, manual overrides, and worker management. |
| **Deployment** | Docker Compose for local stacks, Helm + Terraform for production clusters, GHCR image publication. |
| **Observability** | Metrics/trace/logging baselines with documented dashboards and alert hooks. |

## Architecture Overview
### Service Topology
- **Master service** orchestrates scheduling, metadata ingestion, policy
  evaluation, and REST/gRPC APIs. It exposes observability endpoints and acts as
  the single source of truth for cluster configuration.
- **Worker pools** execute filesystem scans, imports, transcoding, and
  housekeeping jobs close to storage. Workers register with the master, receive
  queue assignments, report heartbeats, and advertise capabilities.
- **Supporting infrastructure** supplies PostgreSQL for persistence, Redis for
  caching/queueing, and optional OpenSearch for discovery/analytics. Docker
  Compose, Helm, and Terraform examples provision consistent environments.

### Data Flow & Automation Lifecycle
1. **Catalog ingestion** normalizes metadata from upstream providers using a
   pluggable pipeline with Redis caching, rate-limit awareness, and structured
   provenance.
2. **Policy evaluation** leverages release profiles, quality targets, and
   retention rules to determine the next action for each entity.
3. **Job scheduling** enqueues work on Redis Streams with idempotency checks and
   distributed locking to prevent double-processing.
4. **Workers** claim jobs, execute downloads/imports/transcodes using the
   adapter SDKs, and push progress into the master for auditing and dashboards.
5. **Observability** spans the entire loop with metrics, tracing, and logs so
   operators can reason about queue health and job outcomes in real time.

### Security & Access Control
- API access is mediated by labeled API keys tied to RBAC roles (`admin`,
  `operator`, `observer`).
- Secrets are provided via environment variables or secret managers; credentials
  never ship in the repository.
- TLS is expected in production deployments, with ingress controllers or service
  meshes terminating HTTPS.

## Feature Coverage (Documentation Map)
The following documents provide deep dives into the subsystems listed above:

- [`docs/project-status.md`](docs/project-status.md) — live milestone tracking,
  risk register, and release readiness summaries.
- [`docs/domain-model.md`](docs/domain-model.md) — canonical catalog entities
  and relationships.
- [`docs/library-scheduling.md`](docs/library-scheduling.md) — cron-driven job
  orchestration and concurrency controls.
- [`docs/metadata-ingestion.md`](docs/metadata-ingestion.md) — ingestion
  pipeline design, caching strategy, and resiliency techniques.
- [`docs/metadata-overrides.md`](docs/metadata-overrides.md) — operator tooling
  for overrides and correction workflows.
- [`docs/indexer-health-scoring.md`](docs/indexer-health-scoring.md) — adaptive
  indexer scoring, discovery, and failover recommendations.
- [`docs/list-ingestion.md`](docs/list-ingestion.md) — pluggable list ingestion
  modules with contract tests.
- [`docs/release-profile-scoring.md`](docs/release-profile-scoring.md) — quality
  evaluation, custom formats, and policy tuning.
- [`docs/download-client-failover.md`](docs/download-client-failover.md) —
  resilient routing across download clients.
- [`docs/download-plugin-sdk.md`](docs/download-plugin-sdk.md) — SDK contract
  tests for third-party adapters.
- [`docs/file-renaming-templates.md`](docs/file-renaming-templates.md) — rename
  rules and audit trails.
- [`docs/dashboard-operations.md`](docs/dashboard-operations.md) — dashboard
  UX, manual overrides, and job drill-downs.
- [`docs/frontend-todo.md`](docs/frontend-todo.md) — work breakdown for the
  Next.js dashboard implementation.
- [`docs/worker-coordination.md`](docs/worker-coordination.md) — worker
  registration, heartbeat enforcement, and pool configuration.
- [`docs/media-replication.md`](docs/media-replication.md) — multi-region
  storage strategies and retention policies.
- [`docs/performance-regression.md`](docs/performance-regression.md) — load test
  suites and scale validation.
- [`docs/deployment/production.md`](docs/deployment/production.md) — Helm and
  Terraform production runbooks.
- [`docs/development-setup.md`](docs/development-setup.md) — contributor
  bootstrapping guide.
- [`docs/migration-guidelines.md`](docs/migration-guidelines.md) — planning for
  Sonarr/Radarr/Lidarr migrations.
- [`docs/access-control.md`](docs/access-control.md) — RBAC and API key
  management details.
- [`docs/observability-baseline.md`](docs/observability-baseline.md) — metrics,
  tracing, and logging instrumentation requirements.
- [`docs/api-first-iteration.md`](docs/api-first-iteration.md) — endpoint
  catalogue for the initial REST/gRPC surface.
- [`docs/adrs/0001-job-queue-selection.md`](docs/adrs/0001-job-queue-selection.md)
  — Architecture Decision Record covering the Redis Streams queue.

## Getting Started
1. Follow the [development setup guide](docs/development-setup.md) to install Go
   1.23+, Node.js, and Docker Compose.
2. Launch the local Docker Compose topology to provision Redis, PostgreSQL,
   OpenSearch, the master service, workers, and the dashboard UI.
3. Explore the REST API documented in
   [`docs/api-first-iteration.md`](docs/api-first-iteration.md) or browse the
   dashboard at <http://localhost:3000> (served by Compose) to inspect library
   entities and queues. Compose seeds an `admin` API token via the
   `ARRGO_BOOTSTRAP_ADMIN_TOKEN` environment variable (default
   `arrgo-admin.devtoken`). The token is injected into the dashboard container
   automatically so authenticated requests succeed out of the box. Override the
   variable in `deploy/compose/.env` to generate a unique credential for your
   environment. The dashboard can also be run directly from
   [`web/dashboard`](web/dashboard) for local iteration.
4. Review [`docs/project-status.md`](docs/project-status.md) for current
   deliverables, owners, and outstanding risks before contributing changes.

## Deployment Pathways
- **Local development:** Docker Compose manifests under
  [`deploy/compose`](deploy/compose) bootstrap the full stack with sensible
  defaults, including the Next.js dashboard. Scale worker containers to simulate
  larger deployments.
- **Kubernetes:** The Helm chart in [`deploy/helm/arrgo`](deploy/helm/arrgo)
  deploys master/worker components, configures secrets, and integrates with
  ingress. Terraform modules in [`deploy/terraform`](deploy/terraform) automate
  namespace creation, secret provisioning, and Helm releases.
- **Container images:** GitHub Actions publish images to GHCR on every main
  branch push and release tag. See `.github/workflows` for workflows and reuse
  them in downstream automation.

## Roadmap
The roadmap is maintained collaboratively between the README, the dedicated
[`docs/project-status.md`](docs/project-status.md) report, and issue trackers.
Timelines are expressed in relative milestones rather than calendar quarters to
stay adaptable as community feedback arrives.

### Alpha — Foundational Infrastructure
Focus: establish the core control plane, worker model, and baseline UX.

- PostgreSQL-backed CRUD for catalog entities and job history.
- Redis Streams orchestration queue with worker registration and heartbeats.
- Minimal dashboard for visibility into libraries, jobs, and worker pools.
- Observability baseline (metrics/traces/logs) and containerized deployment
  assets (Docker Compose, Helm, Terraform).

### Beta — Feature Completeness & Ecosystem
Focus: harden integrations, automation, and security for shared environments.

- Hardened indexer and download client integrations with failover policies.
- Policy-driven automation (quality upgrades, retention, manual overrides).
- Search abstraction via optional OpenSearch/Elasticsearch, including
  dashboard facets.
- Unified book and audiobook workflows delivering feature parity with a
  Readarr-class experience, from metadata ingestion to release monitoring and
  library synchronization.
- Multi-tenant RBAC with labeled API keys, audit trails, and single sign-on
  integrations.
- Expanded dashboard capabilities: queue analytics, manual search overrides,
  worker pool editing.
- Request management portal replacing Overseerr with quota controls, approval
  workflows, and automation hand-offs into the scheduling engine.
- SDKs and contract tests for download clients and list ingestion modules.

### General Availability — Scale, Reliability, Extensibility
Focus: production hardening, analytics, and a thriving plugin ecosystem.

- High-availability master deployment, worker auto-scaling, and queue
  partitioning strategies.
- Advanced analytics dashboards, alerting hooks, and runbooks for operators.
- Plugin marketplace covering download clients, metadata providers,
  notifications, media servers, automation hooks, storage targets, and
  authentication providers.
- Migration tooling for importing/exporting from legacy *arr deployments.
- Extended replication workflows, regional failover, and storage lifecycle
  policies.

## Contributing
Contributions are welcome once the public roadmap items reach the beta phase.
When preparing a change:

1. Discuss significant proposals via issues or discussions to align on scope.
2. Follow the coding standards in [AGENTS.md](AGENTS.md) and run `go test ./...`
   plus relevant front-end or lint commands before submitting a PR.
3. Document architectural decisions using ADRs and update relevant guides when
   behavior changes.
4. Provide actionable commit messages and PR descriptions that reference
   affected subsystems and include test evidence.

## License
This project is licensed under the [Apache License 2.0](LICENSE).
