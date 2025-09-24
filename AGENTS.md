# Agent Guidelines for arrgo

## Scope
These instructions apply to the entire repository. Follow them for all code, configuration, documentation, and asset updates.

## Repository Directory Guide
The following table highlights the most relevant directories and reference files so new contributors can quickly locate the assets they need.

| Path | Purpose |
| --- | --- |
| `README.md` | High-level project overview, roadmap, and contributor expectations. |
| `api/proto` | Source-of-truth protobuf definitions for the public gRPC/REST APIs. |
| `deploy/compose` | Docker Compose stack for local development and integration testing. |
| `deploy/helm/arrgo` | Kubernetes Helm charts for production-grade deployments. |
| `deploy/terraform` | Terraform modules that provision the infrastructure backing arrgo clusters. |
| `docs/` | Comprehensive design, ADR, and operations documentation—start with `docs/project-status.md` for current milestones. |
| `internal/api` | gRPC/REST server implementations and transport adapters. |
| `internal/auth` | Authentication, RBAC policy enforcement, and API key management logic. |
| `internal/integrations` | External service connectors (indexers, download clients, list providers). |
| `internal/library` | Core library domain services: catalog state, scheduling, and job orchestration. |
| `internal/metadata` | Metadata ingestion pipelines with caching, rate limiting, and provider backoffs. |
| `internal/observability` | Shared logging, metrics, and tracing helpers. |
| `internal/replication` | Media replication workflows, storage policies, and transfer orchestration. |
| `internal/worker` | Worker registration, heartbeat coordination, and queue consumers. |
| `sdk/downloadclient` | Go SDK for building download client plugins, plus contract tests. |
| `sdk/listingester` | SDK for external list ingestion modules (e.g., Trakt, IMDb, Last.fm). |
| `web/dashboard` | Next.js dashboard application for cluster visibility and manual job controls. |
| `.github/workflows` | CI pipelines for linting, testing, and container publication. |

## Development Workflow
- Target Go 1.22+ for all backend services.
- Format Go sources with `gofmt` (or `go fmt ./...`) before committing.
- Run `go test ./...` and any additional project-specific checks (e.g., `golangci-lint run`, front-end linters) when touching executable code.
- Keep commit messages and pull request summaries actionable and reference affected subsystems when possible.
- Use feature branches; never commit directly to `main`.

## Architectural Expectations
- The platform uses a master/worker container model. Any changes that alter scheduling, workload distribution, or service boundaries must be reflected in the README and architecture diagrams.
- PostgreSQL is the system of record, Redis is the cache/event backbone, and Elasticsearch/OpenSearch is optional search. Do not introduce alternative defaults without prior discussion.
- Prefer gRPC and REST for service communication; document any new protocols or message formats.

## Documentation Standards
- Use Markdown with descriptive headings, ordered/unordered lists, and tables for clarity.
- Update the README and related docs whenever new features, deployment requirements, or operational procedures are introduced.
- Maintain the Roadmap and TODO sections so newcomers understand project status.

## Best Practices
- Enforce observability: instrument new code with metrics, tracing, and structured logging.
- Secure secrets through environment variables or secret managers; never commit credentials.
- When adding third-party dependencies, justify them in commit/PR descriptions and ensure licenses are compatible.
- Keep Docker, Compose, and Helm artifacts consistent with documented defaults.

## Coordination
- Tag issues or TODO items with ownership when possible to aid distribution across master/worker teams.
- Raise architecture decision records (ADRs) for significant platform choices.
- If unsure about instructions in this file, update it with clarifications as part of your change.
