# Roadmap TODO

> **Last updated:** 2025-09-26
> **Source of truth:** Mirrors the functional release checklist in [`docs/project-functional-todo.md`](docs/project-functional-todo.md).

This file tracks the highest-priority roadmap items that still require work before the functional release milestone is complete.

## Backend & Data Plane
- [x] Finalize PostgreSQL schema migrations with rollback coverage and seed data for demo environments.
- [x] Persist job execution history with retention policies and expose it through `/api/v1/jobs` pagination.
  - Job lifecycle events now flow into the dedicated history store with automatic retention pruning, and `/api/v1/jobs` returns paginated listings hydrated with per-job history records.
- [x] Land request persistence models (request, approval event, notification) and wire them into the library service.
  - Request submissions, approvals, and notifications now persist through the library repository (in-memory and Postgres) with schema migrations, seeding hooks, and service methods that hydrate timelines on read.
- [x] Harden Redis Streams configuration (key rotation, consumer group lag alerts, automated dead-letter handling).
  - Stream namespaces now support version rotation, emit lag warnings, and funnel exhausted jobs into a managed dead-letter queue.

## API Surface & Request Management
- [ ] Implement the request endpoints documented in [`docs/api-first-iteration.md`](docs/api-first-iteration.md) with RBAC enforcement.
- [x] Add policy evaluation hooks so quotas and automation previews run before requests are approved.
  - Library service now evaluates quotas and automation previews ahead of approval to block exhausted policies and capture preview telemetry.
- [ ] Publish OpenAPI/Buf descriptors for REST and gRPC services once the request APIs stabilize.
  - **Do not do:** Blocked until the request APIs reach GA stability; revisit after the release candidate hardening window.
- [x] Provide contract/integration tests that cover approve/deny/cancel flows and downstream job fan-out.
  - Added service- and HTTP-level tests that assert approval fan-out enqueues automation jobs while denial/cancel flows skip queueing.

## Observability & Operations
- [ ] Export master HTTP metrics (`arrgo_http_requests_total`, `arrgo_http_request_duration_seconds`) via the OpenTelemetry Collector to Prometheus.
- [ ] Ensure worker metrics (`arrgo_workers_*`) are present in Grafana dashboards with pooled drill-downs.
- [ ] Propagate trace context into worker-executed jobs and surface span links in dashboard workflows.
- [ ] Document incident response runbooks for request backlogs, worker staleness, and metadata provider outages.

## Frontend & UX Polish
- [ ] Connect the Next.js dashboard to the new request endpoints (list, submit, approve/deny/cancel, timeline).
- [ ] Add live quota widgets driven by policy evaluation responses.
- [ ] Ship accessibility and performance regression checks for the request portal views.
- [ ] Provide environment toggles for observability backends (Prometheus, Tempo, Loki) in the dashboard settings UI.

## Delivery & Release Management
- [ ] Gate main-branch merges on `go test ./...`, Playwright smoke tests, and linting.
- [ ] Produce a release checklist covering configuration templates, secrets, and first-deploy validation steps.
- [ ] Publish sample Terraform/Helm overrides for enabling the request portal and observability integrations.
- [ ] Announce functional release readiness in `docs/project-status.md` with a cutover date and owner.
