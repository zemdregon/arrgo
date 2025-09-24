# Frontend Implementation TODO

> **Last updated:** 2025-09-24
> **Scope:** Next.js dashboard that replaces legacy Overseerr-style workflows and surfaces arrgo automation controls.

## Objectives
- Deliver an operator-focused dashboard that surfaces health, automation activity, and manual controls without depending on legacy *arr frontends.
- Provide a request experience that supersedes Overseerr, integrates with arrgo policies, and exposes quota/approval flows.
- Ensure the UI scales alongside roadmap milestones, including the Readarr-class book experience and future plugin ecosystem extensions.

## Work Breakdown Structure

### Core Application Foundation
- [x] Establish Next.js app router structure, TypeScript strict mode, and shared layout primitives.
- [x] Implement global design system (theme tokens, typography, color, spacing) with light/dark mode support.
- [x] Configure authentication guards, API key selection, and RBAC-aware route protection.
- [x] Build REST/gRPC client adapters with SWR/React Query hooks and request tracing instrumentation.
- [x] Set up error boundaries, toast/alert primitives, and structured logging for client-side diagnostics.

### Navigation & Layout
- [x] Create persistent shell with navigation, environment context switcher, and worker status indicator.
- [x] Implement responsive grid/layout rules for widescreen dashboards and compact tablet views.
- [ ] Add global search and quick action palette for navigating entities and triggering automation workflows.

### Library & Automation Views
- [ ] Library overview with filterable status cards (pending imports, quality upgrades, failures).
- [ ] Detail pages for shows, movies, albums, and books that expose metadata, history, and manual job triggers.
- [x] Queue analytics dashboard with stream lag, retry counts, and SLA indicators.
- [ ] Policy management editor for release profiles, retention rules, and media-specific overrides.
- [x] Readarr replacement UI: reading lists, audiobook/ebook metadata comparisons, and download history timelines.

### Request Management (Overseerr Replacement)
- [ ] Public/private request form with media search, availability indicators, and quota display.
- [ ] Approval workflow view with bulk actions, automation previews, and status history.
- [ ] Notification settings for request outcomes (email, webhook, push integrations).
- [ ] SLA metrics for request turnaround, backlog aging, and policy violations.

### Worker & Infrastructure Operations
- [ ] Worker pool list with health indicators, capability tags, and heartbeat timelines.
- [ ] Job drill-down views showing execution logs, retries, and related media entities.
- [ ] Observability suite integration: charts for metrics, trace sampling, and log streaming.
- [ ] Configuration UI for secrets management pointers and infrastructure environment variables.

### Quality, Testing, and Delivery
- [ ] Storybook (or equivalent) coverage for core components with visual regression testing.
- [ ] Playwright end-to-end flows for critical journeys (library ingest, request approval, worker remediation).
- [ ] Accessibility audit and keyboard navigation coverage for high-traffic interactions.
- [ ] Performance budgets for initial load, navigation latency, and API response hydration.
- [ ] Deployment pipeline validation (linting, type checks, tests) integrated into CI and preview environments.

## Dependencies & Follow-Ups
- Align API contract coverage with [`docs/api-first-iteration.md`](api-first-iteration.md) and expand endpoints for request management as backend stories land.
- Coordinate with observability workstream to surface metrics and traces exposed by the master/worker services.
- Update the roadmap and status reports as frontend milestones reach completion to maintain cross-team visibility.
