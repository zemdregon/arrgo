# Dashboard Operations Enhancements

The arrgo dashboard now surfaces deeper operational insights and direct
configuration controls so operators can steer orchestration without leaving the
UI.

## Queue analytics

The **Job queue** panel includes a metrics ribbon plus a dedicated analytics
grid that breaks down queued, running, and completed workloads by type. The
panel computes aggregate counts client-side from `/api/v1/jobs` responses and
also tracks the latest submission timestamp so operators can spot stalls or
bursts at a glance. Average attempt counts are surfaced to highlight retry
pressure, complementing the per-state gauges that were already present.

## Job drill-downs

Each job entry now exposes an **Inspect** action. Selecting a job opens a detail
drawer with the idempotency key, payload URI, timestamps, attempt history, and a
status pill sourced from the orchestration service. The drawer provides both the
absolute submission time and a relative age using the same formatting helpers as
the list view. This makes it easy to audit jobs without leaving the dashboard or
hitting the gRPC/REST APIs manually.

## Worker pool configuration editing

A new **Worker pool configuration** panel reads data from `GET /api/v1/worker-pools`
and allows updates through `PUT /api/v1/worker-pools/{pool}`. The form persists
description updates, queue stream/consumer group overrides, heartbeat cadence,
staleness thresholds, and optional concurrency limits. Submitted changes call
into the worker service, which now tracks per-pool overrides and immediately
reconciles metrics to reflect new settings. The panel also summarizes current
heartbeat and stale thresholds alongside live worker counts drawn from the
worker listing endpoint, giving administrators context while adjusting knobs.

## Manual indexer search overrides

Operators can now launch manual indexer searches directly from the dashboard via
the **Manual indexer search** panel. Selecting a library entity issues a
`GET /api/v1/indexer/search?entity_id=<id>` call to hydrate the most recent
results, including the last query, execution timestamp, and a per-release
history. Running a new search submits to `POST /api/v1/indexer/search`, which
returns a synthetic set of releases to review.

Each release row exposes the indexer, quality, publish time, and score along
with status pills so rejections are surfaced alongside the rejection reason. The
**Grab** action invokes `POST /api/v1/indexer/releases/grab` to override
automation and mark a candidate as manually grabbed. Results update in-place so
operators can track which releases have been queued while auditing additional
candidates.
