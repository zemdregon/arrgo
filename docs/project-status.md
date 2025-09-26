# arrgo Project Status

> **Last reviewed:** 2025-03-10
> **Current phase:** Alpha hardening (foundational infrastructure feature complete, polish in progress)

This document aggregates milestone tracking, delivery confidence, and near-term
focus areas. It should be read alongside the [README](../README.md) roadmap and
referenced before planning new workstreams.

## Platform Snapshot
- Core services (master, worker pools) compile and run locally with Docker
  Compose, Helm, and Terraform deployment assets available for cluster rollout.
- Redis Streams powers the orchestration queue, worker registration, and
  heartbeat management. CI covers service-level unit tests for queue semantics
  and worker lifecycle flows.
- Multi-stage Docker image packages the production binaries with embedded build
  metadata so Compose, Helm, and Terraform deployments all execute the same
  signed artifacts.
- Metadata ingestion, release profile scoring, download client failover, and
  list ingestion are implemented with accompanying SDKs and documentation.
- The Next.js dashboard exposes cluster telemetry, job analytics, worker pool
  editing, and manual intervention workflows.

## Completed Milestones
- **Control plane foundation** — worker registration/heartbeat, Redis Streams
  orchestration queue, cron-backed scheduler, and Postgres data model
  definitions. Documented in
  [`docs/library-scheduling.md`](library-scheduling.md) and
  [`docs/worker-coordination.md`](worker-coordination.md).
- **Postgres migrations & demo seed** — repeatable schema migrations with
  checksum enforcement, rollback coverage, and demo catalog content to
  accelerate sandbox environments. Backed by
  [`internal/postgres/migrate`](../internal/postgres/migrate) and documented in
  [`docs/project-functional-todo.md`](project-functional-todo.md).
- **Metadata & catalog** — ingestion pipeline with caching, overrides, and the
  unified domain model. See [`docs/metadata-ingestion.md`](metadata-ingestion.md),
  [`docs/metadata-overrides.md`](metadata-overrides.md), and
  [`docs/domain-model.md`](domain-model.md).
- **Automation tooling** — release profile scoring, download client failover,
  indexer health scoring, and list ingestion SDK. See
  [`docs/release-profile-scoring.md`](release-profile-scoring.md),
  [`docs/download-client-failover.md`](download-client-failover.md), and
  [`docs/list-ingestion.md`](list-ingestion.md).
- **Operations & UX** — dashboard drill-downs, queue analytics, worker pool
  editing, API key management, observability baselines, and ADRs documenting key
  choices. See [`docs/dashboard-operations.md`](dashboard-operations.md),
  [`docs/access-control.md`](access-control.md),
  [`docs/observability-baseline.md`](observability-baseline.md), and
  [`docs/adrs/0001-job-queue-selection.md`](adrs/0001-job-queue-selection.md).
- **Request portal UI** — Next.js request intake, approval workflows, and
  notification settings shipped behind feature flags pending backend request
  persistence. Tracked in [`docs/project-functional-todo.md`](project-functional-todo.md).
- **Deployment assets** — Docker Compose, Helm charts, Terraform modules, and
  GHCR image publication workflows. See [`docs/deployment/production.md`](deployment/production.md)
  and the repository `.github/workflows` directory.

## In-Flight Priorities (Alpha Polishing)
- Establish automated Postgres backup verification and validation tooling to
  complement the new migration framework.
- Harden worker lifecycle management with grace periods, back-pressure
  heuristics, and alert-friendly metrics.
- Expand contract test coverage for plugin SDKs (download clients, list
  ingesters) to accelerate community contributions.
- Document incident response playbooks for queue stalls, ingestion failures, and
  metadata discrepancies.
- Deliver request persistence, approval events, and automation fan-out so the
  dashboard request portal can ship without feature flags.

## Upcoming Milestones
| Target | Focus | Exit Criteria |
| --- | --- | --- |
| **Alpha exit** | Postgres persistence, observability package, worker lifecycle polish. | CRUD backed by Postgres, dashboard reflects persisted data, alert runbooks published. |
| **Beta kickoff** | Ecosystem integrations and security hardening. | Indexer/download client integrations enabled end-to-end, SSO/RBAC stories validated, dashboard feature set feature-complete. |
| **GA preparation** | Reliability, scale tests, marketplace governance. | HA master rollout validated, regression suite green on scale scenarios, plugin distribution guidelines published. |

## Risk Register
- **Observability follow-through** — tracing propagation and dashboard bundles
  still need final packaging; missing instrumentation could slow incident
  response. *Mitigation:* prioritize observability tasks in Alpha exit criteria
  and ensure docs include dashboard import instructions.
- **Stateful migration** — early adopters running pre-Postgres builds still need
  a controlled path for importing dev/test data into the database-backed
  catalog. *Mitigation:* document migration tooling in
  [`docs/migration-guidelines.md`](migration-guidelines.md) and deliver upgrade
  scripts alongside schema changes.
- **Plugin ecosystem readiness** — third-party connectors rely on contract tests
  and packaging guidance that are partially drafted. *Mitigation:* continue to
  grow the SDK docs and publish sample adapters.

## Communication Cadence
- **Weekly**: Update this status report and the README roadmap with progress and
  blockers.
- **Bi-weekly**: Review ADR backlog, triage open design questions, and ensure
  documentation matches shipped behavior.
- **Release gates**: Alpha exit review focuses on persistence and observability;
  beta kickoff emphasizes integrations and security; GA prep centers on scale
  validations and marketplace operations.
- **Cross-team visibility**: Keep [`docs/project-functional-todo.md`](project-functional-todo.md)
  current so backend, frontend, and observability owners can coordinate releases.

## How to Contribute
- Review the [development setup guide](development-setup.md) to bootstrap your
  environment.
- Pick work items from the milestone tables above, confirm ownership in issues,
  and document changes by updating the relevant subsystem guides.
- Capture architecture-impacting changes as ADRs and link them from this report
  when adopted.
