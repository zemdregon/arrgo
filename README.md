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
4. Integrate download clients, indexers, and notification services via API keys or OAuth.
5. Monitor via built-in Prometheus metrics, Grafana dashboards, and structured logging exporters.

## Getting Started TODOs
Until the initial release is available, track the following tasks:
1. ✅ Finalize the domain model for unified media entities (series, movies, artists, albums, tracks). See [`docs/domain-model.md`](docs/domain-model.md).
2. Stand up Docker Compose assets for Postgres, Redis, Elasticsearch/OpenSearch, master, and worker containers.
3. Implement the metadata ingestion pipeline with Redis-backed caching.
4. Expose the first iteration of the REST/gRPC APIs for library management and job orchestration.
5. ✅ Build a GitHub Actions workflow that publishes arrgo container images to GHCR for distribution.
6. Build CI pipelines for linting and automated testing.
7. Draft migration guidelines for users coming from existing Sonarr/Radarr/Lidarr setups.

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
- Integration with popular indexers and download clients.
- Policy-driven automation for quality upgrades and post-processing.
- Search abstraction with optional Elasticsearch/OpenSearch backing.
- Multi-tenant support and RBAC for shared environments.

### General Availability (Scale and Polish)
- High-availability master election and worker auto-scaling.
- Advanced analytics dashboards and alerting hooks.
- Plugin marketplace for metadata providers and automation scripts.
- Migration tooling for import/export from legacy *arr installations.

## Development Best Practices
- **Language & Tooling:** Services are written in Go. Use Go 1.22+ and run `go fmt`/`goimports` on touched files. Front-end components (when introduced) should follow the repository’s lint and formatting rules.
- **Testing:** Maintain a healthy unit and integration test suite. Always run `go test ./...` (plus any relevant front-end or end-to-end suites) before submitting changes.
- **Documentation:** Keep architecture diagrams, API contracts, and configuration docs synchronized with the codebase. Update this README and in-repo ADRs when behavior changes.
- **Git Workflow:** Work on feature branches, squash commits locally, and open pull requests with descriptive summaries, test evidence, and rollout notes.
- **Observability:** Include metrics and tracing instrumentation alongside new features. Prefer structured logging with log levels and correlation IDs.
- **Security:** Store secrets in environment variables or secret managers. Enforce TLS for external services and validate all inbound payloads.

## Contributing
We welcome contributions once the core is published. Please open a discussion thread for large proposals, adhere to the best practices above, and ensure all CI checks pass before requesting review.

## License
License information will be finalized closer to the first release.
