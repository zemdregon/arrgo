# Functional Readiness TODO
> **Goal:** Drive arrgo from alpha hardening to a day-one functional release that new operators can deploy with confidence.

The items below consolidate cross-team follow-ups required to ship a cohesive, production-ready experience. Each section highlights the expected owner and the success criteria that mark the task as complete.

## Backend & Data Plane
- [x] Finalize PostgreSQL schema migrations with rollback coverage and seed data for demo environments.
  - Added a shared migration runner (`internal/postgres/migrate`) that tracks checksums, supports rollbacks, and seeds demo catalog content for quick-start clusters.
- [x] Persist job execution history with retention policies and expose it through `/api/v1/jobs` pagination.
  - Job lifecycle events now flow into a dedicated history store with automatic retention, and `/api/v1/jobs` supports cursor-based pagination with per-job history hydration.
- [x] Land request persistence models (request, approval event, notification) and wire them into the library service.
- [x] Harden Redis Streams configuration (key rotation, consumer group lag alerts, automated dead-letter handling).
  - Added stream keyspace rotation helpers, lag alert instrumentation, and automatic dead-letter routing for exhausted jobs.

## API Surface & Request Management
- [x] Implement the request endpoints documented in [`docs/api-first-iteration.md`](api-first-iteration.md) with RBAC enforcement.
 - [x] Add policy evaluation hooks so quotas and automation previews run before requests are approved.
   - Library service injects quota and automation preview evaluators that run prior to approvals, updating persisted quota state and surfacing preview telemetry.
- [ ] Publish OpenAPI/Buf descriptors for REST and gRPC services once the request APIs stabilize.
- [ ] Provide contract/integration tests that cover approve/deny/cancel flows and downstream job fan-out.

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
