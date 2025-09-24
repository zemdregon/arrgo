# arrgo

## Overview
arrgo is an all-in-one, Go-powered replacement for the Sonarr, Radarr, and Lidarr media management stack. It is engineered for ultimate performance and horizontal scalability so that large libraries and high-frequency automation workloads remain responsive even when traditional *arr services begin to lag. The project unifies television, movie, and music automation behind a single API surface while embracing cloud-native primitives for reliability and velocity.

## Project Goals
- **Unify the media automation ecosystem** under a single, consistent service.
- **Deliver predictable performance at scale** through Go-based services and concurrency-aware architecture.
- **Simplify distributed deployments** with a master/worker container topology that keeps workloads close to media and download infrastructure.
- **Make observability a first-class concern** via structured logging, metrics, and health checks.

## Core Capabilities
- End-to-end management of series, films, and music catalogs.
- Event-driven workflow that integrates with download clients, indexers, and storage back-ends.
- Policy engine for profile-based quality upgrades, retention, and deduplication.
- Rich REST and gRPC APIs for UI, automation, and third-party integration.
- Pluggable metadata providers with caching for near-instant lookups.

## Architecture Overview
### Service Topology
arrgo is deployed as a cluster of Docker containers:
- **Master node** orchestrates scheduling, ingest pipelines, metadata aggregation, and global coordination.
- **Worker nodes** execute scan, import, and transcoding jobs close to storage or download agents. Multiple worker pools can be defined to target specific media types or hardware accelerators.
- **Auxiliary services** deliver persistence, caching, and optional search capabilities.

### Data Layer
| Role | Technology | Purpose |
| --- | --- | --- |
| Primary database | PostgreSQL | Authoritative store for media entities, schedules, history, and configuration. Battle-tested reliability and powerful query planner make it ideal for relational workloads. |
| Cache layer | Redis | Hot-path accelerator for metadata, job state, throttling tokens, and distributed locks. Enables microsecond responses for frequently accessed data. |
| Optional search | Elasticsearch / OpenSearch | Provides fuzzy matching, advanced filtering, and aggregated analytics across artists, albums, tracks, series, and movies. Can be omitted in smaller deployments. |

Each component is provisioned as a dedicated Docker container. Infrastructure-as-code examples (e.g., Docker Compose, Helm charts) will be published to streamline cluster bootstrap.

### Messaging and Coordination
- gRPC and REST APIs for master ↔ worker communication.
- Redis streams (or an alternative message bus) for fan-out job distribution.
- Configurable backpressure and retry strategies to keep workloads resilient.

## Deployment Model
1. Deploy PostgreSQL, Redis, and (optionally) Elasticsearch/OpenSearch using the provided container definitions.
2. Bring up the arrgo master container, configured with connection strings and cluster metadata.
3. Scale worker containers horizontally, pinning them to specific hosts or availability zones as required.
4. Integrate download clients, indexers, and notification services via API keys or OAuth, with future support for per-application tokens.
5. Monitor via built-in Prometheus metrics, Grafana dashboards, and structured logging exporters.

## Getting Started TODOs
Until the initial release is available, track the following tasks.

### Completed
- ✅ Add automated indexer health scoring and discovery with percentile-based failover recommendations. See [`internal/integrations/indexer`](internal/integrations/indexer) and [`docs/indexer-health-scoring.md`](docs/indexer-health-scoring.md).
- ✅ Implement cross-region media replication workflows with S3-compatible object storage targets and policy-driven retention. See [`docs/media-replication.md`](docs/media-replication.md).
- ✅ Integrate prioritized download client failover with health-aware routing. See [`docs/download-client-failover.md`](docs/download-client-failover.md).
- ✅ Publish the download client plugin SDK and contract tests so third-party adapters can target arrgo releases. See [`sdk/downloadclient`](sdk/downloadclient) and [`docs/download-plugin-sdk.md`](docs/download-plugin-sdk.md).
- ✅ Implement role-based access controls with per-application API key generation, labeling, and rotation workflows. See [`docs/access-control.md`](docs/access-control.md).
- ✅ Harden metadata ingestion resilience with rate limit backoff, provider-specific cache TTLs, tracing, and failure metrics.
- ✅ Automate scheduled library scans and media imports with cron-driven orchestration and audit fan-out. See [`docs/library-scheduling.md`](docs/library-scheduling.md).
- ✅ Finalize the domain model for unified media entities (series, movies, artists, albums, tracks). See [`docs/domain-model.md`](docs/domain-model.md).
- ✅ Stand up Docker Compose assets for Postgres, Redis, Elasticsearch/OpenSearch, master, and worker containers.
- ✅ Build a GitHub Actions workflow that publishes arrgo container images to GHCR for distribution.
- ✅ Document the local development bootstrap process for contributors. See [`docs/development-setup.md`](docs/development-setup.md).
- ✅ Implement the metadata ingestion pipeline with Redis-backed caching. See [`internal/metadata`](internal/metadata) and [`docs/metadata-ingestion.md`](docs/metadata-ingestion.md).
- ✅ Expose the first iteration of the REST/gRPC APIs for library management and job orchestration. See [`docs/api-first-iteration.md`](docs/api-first-iteration.md).
- ✅ Build CI pipelines for linting and automated testing. See [`.github/workflows/ci.yml`](.github/workflows/ci.yml).
- ✅ Draft migration guidelines for users coming from existing Sonarr/Radarr/Lidarr setups. See [`docs/migration-guidelines.md`](docs/migration-guidelines.md).
- ✅ Design and document the observability baseline (metrics, tracing, logging dashboards). See [`docs/observability-baseline.md`](docs/observability-baseline.md).
- ✅ Author an ADR capturing the Redis Streams job queue selection and retry strategy. See [`docs/adrs/0001-job-queue-selection.md`](docs/adrs/0001-job-queue-selection.md).
- ✅ Implement the Redis Streams-backed orchestration queue with producer/consumer plumbing in the library service.
- ✅ Establish worker registration and heartbeat coordination across REST and gRPC surfaces. See [`docs/worker-coordination.md`](docs/worker-coordination.md).
- ✅ Deliver the initial web dashboard for cluster visibility. See [`web/dashboard`](web/dashboard) and the updated front-end workflow guidance in [`docs/development-setup.md`](docs/development-setup.md#web-dashboard-development).
- ✅ Expand the dashboard with interactive controls for queueing library jobs now that the write APIs are available. See [`web/dashboard`](web/dashboard).
- ✅ Publish Helm charts and Terraform modules for production-grade deployments. See [`deploy/helm/arrgo`](deploy/helm/arrgo), [`deploy/terraform`](deploy/terraform), and [`docs/deployment/production.md`](docs/deployment/production.md).
- ✅ Stand up automated performance and regression suites for large media libraries, covering scheduler fan-out and queue drain behavior. See [`docs/performance-regression.md`](docs/performance-regression.md).
- ✅ Expand the web dashboard with queue analytics, job drill-downs, and worker pool configuration editing. See [`web/dashboard`](web/dashboard) and [`docs/dashboard-operations.md`](docs/dashboard-operations.md).
- ✅ Implement external list ingestion with pluggable modules for Trakt, IMDb, and Last.fm so new list providers can be added without core changes. See [`internal/integrations/list`](internal/integrations/list) and [`docs/list-ingestion.md`](docs/list-ingestion.md).
- ✅ Build release profile/custom format scoring with preferred group/codec filters, blocklists, and backwards-compatible support for importing Sonarr/Radarr custom format definitions to match the advanced grab rules in the existing *arr applications. See [`internal/automation/releaseprofile`](internal/automation/releaseprofile) and [`docs/release-profile-scoring.md`](docs/release-profile-scoring.md).
- ✅ Add interactive/manual search workflows in the dashboard and APIs for reviewing indexer results and force-grabbing specific releases when automation falls short. See [`internal/library/service.go`](internal/library/service.go).
- ✅ Deliver configurable file renaming and folder organization templates for episodes, movies, and music libraries with history-aware rename jobs. See [`docs/file-renaming-templates.md`](docs/file-renaming-templates.md) and [`internal/library/renamer.go`](internal/library/renamer.go).

### Up Next
- 🚧 Expose rich metadata override tools (episode/season cutovers, album editions, track numbering) so operators can correct catalog data without waiting on upstream providers.

### Docker Compose development stack

Local infrastructure is defined under [`deploy/compose`](deploy/compose) to make it easy to boot an end-to-end arrgo topology for experimentation.

1. Change into the compose directory: `cd deploy/compose`.
2. Copy `.env.example` to `.env` if you want to override the default credentials or container image tag.
3. Build and start the stack: `docker compose up --build`.
4. (Optional) Scale workers to simulate a larger cluster: `docker compose up --build --scale arrgo-worker=3`.

The stack provisions PostgreSQL, Redis, OpenSearch, a master container, and at least one worker on an isolated bridge network. Persistent data is stored in named volumes so that containers can be restarted without losing state. The arrgo containers currently run placeholder commands until the application binaries are available.

## Continuous Integration & Delivery

Container images are automatically built and published to the GitHub Container Registry (GHCR) whenever changes land on the `main` branch or a release is published. The [`publish-image.yml`](.github/workflows/publish-image.yml) workflow authenticates with GHCR using the repository's `GITHUB_TOKEN`, builds the Docker image defined in the repository `Dockerfile`, and pushes tags such as `:main`, release tags, and a commit-specific `:sha` reference.

To use the published image locally:

1. Authenticate with GHCR: `echo $CR_PAT | docker login ghcr.io -u <github-username> --password-stdin`.
2. Pull the latest image: `docker pull ghcr.io/<github-organization>/arrgo:main`.
3. Run the container or incorporate it into deployment tooling (e.g., Docker Compose, Helm charts) as they become available.

## Roadmap
### Alpha (Foundational Infrastructure)
- CRUD support for media catalogs with Postgres persistence.
- Distributed job queue with master/worker coordination via Redis.
- Minimal UI shell for library visibility and manual actions.
- Containerized deployment artifacts and observability baseline.

### Beta (Feature Completeness)
- Integration with popular indexers and download clients via a hardened plugin interface.
- Policy-driven automation for quality upgrades and post-processing.
- Search abstraction with optional Elasticsearch/OpenSearch backing.
- Multi-tenant support and RBAC for shared environments, including labeled API key issuance for individual applications and automations.
- Single sign-on integration with major identity providers to simplify dashboard and API access management.

### General Availability (Scale and Polish)
- High-availability master election and worker auto-scaling.
- Advanced analytics dashboards and alerting hooks.
- Full plugin marketplace covering connectors for download clients, metadata providers, notifications, indexers, media servers, and event automation with curated verification.
- Migration tooling for import/export from legacy *arr installations.

## Planned Extensibility and Plugin Ecosystem
High modularity is a core design goal for arrgo once the foundational services stabilize. The platform will expose a plugin SDK, lifecycle hooks, and packaging specifications so that connectors can be built, versioned, and distributed independently of the core release cadence. Planned plugin categories include:

| Plugin type | Purpose |
| --- | --- |
| **Notification agents** | Deliver events to chat platforms, webhooks, incident systems, and custom alerting pipelines.
| **Download client adapters** | Support a broad range of BitTorrent, Usenet, and emerging download clients with health-aware routing and failover strategies.
| **Metadata providers** | Enrich media entities with external catalog data, artwork, and localization through swappable provider integrations.
| **Indexer connectors** | Integrate with traditional Usenet/torrent indexers and custom search services, including private catalogs or enterprise content directories.
| **Media server bridges** | Synchronize libraries, playback status, and artwork with Plex, Jellyfin, Emby, and other media-serving platforms.
| **Automation and workflow hooks** | Allow bespoke scripting, quality gates, and policy engines to react to job lifecycle events or library state changes.
| **Storage and delivery targets** | Extend import/export to cloud object stores, cold storage archives, CDN origins, or air-gapped repositories.
| **Transcoding and analysis modules** | Plug in specialized media processing pipelines, including AI-assisted tagging, loudness normalization, or subtitle enhancement.
| **Authentication and access providers** | Offer optional SSO, OAuth/OIDC, or custom credential brokers to complement the built-in RBAC model.

Plugin discovery, dependency management, and marketplace distribution will be documented alongside governance guidelines for community submissions. The API surface will also expose per-plugin API keys and secrets scopes so operators can audit and rotate credentials without affecting the entire deployment.

## Development Best Practices
- **Language & Tooling:** Services are written in Go. Use Go 1.24+ (the module currently targets Go 1.24.3) and run `go fmt`/`goimports` on touched files. Front-end components (when introduced) should follow the repository’s lint and formatting rules.
- **Testing:** Maintain a healthy unit and integration test suite. Always run `go test ./...` (plus any relevant front-end or end-to-end suites) before submitting changes.
- **Documentation:** Keep architecture diagrams, API contracts, and configuration docs synchronized with the codebase. Update this README and in-repo ADRs when behavior changes.
- **Git Workflow:** Work on feature branches, squash commits locally, and open pull requests with descriptive summaries, test evidence, and rollout notes.
- **Observability:** Include metrics and tracing instrumentation alongside new features. Prefer structured logging with log levels and correlation IDs.
- **Security:** Store secrets in environment variables or secret managers. Enforce TLS for external services and validate all inbound payloads.

## Contributing
We welcome contributions once the core is published. Please open a discussion thread for large proposals, adhere to the best practices above, and ensure all CI checks pass before requesting review.

## License
License information will be finalized closer to the first release.
