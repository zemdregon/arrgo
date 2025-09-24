# Cross-Region Media Replication

> **Last reviewed:** 2025-03-08  
> **Status:** Implemented with workflow engine in `internal/replication`

## Overview
Arrgo now includes a dedicated replication service that mirrors library imports across
multiple S3-compatible regions. The service is implemented in [`internal/replication`](../internal/replication)
and exposes a workflow-driven API so the master can fan out artifacts to any number of
object storage targets after a worker finishes processing media. Each workflow captures
the source region, the destinations to fan out to, and the retention policy that governs
how many historical copies are retained per asset.

## Workflow Components
A replication workflow is composed of the following elements:

- **Name** – unique identifier referenced by the worker/master orchestration layer.
- **Source region** – optional indicator of the primary ingest region.
- **Targets** – one or more destinations, each with:
  - `name` and `region` metadata for observability.
  - `bucket`, optional `prefix`, and `storage_class` that map to the S3-compatible API.
  - `endpoint` for providers such as MinIO, Ceph, or Backblaze B2 that expose an S3
    interface.
  - `retention` override when a target needs stricter retention than the workflow default.
- **Retention policy** – default retention applied to every target unless overridden.

Workflows are registered through `replication.Service.RegisterWorkflow` or supplied during
service construction. Once registered, the service guarantees that the same object key is
mirrored to each target with an automatically generated version suffix so every artifact
is unique.

### Example: Registering a Workflow
```go
clock := systemClock{}
storageA := newS3Client(...) // wraps AWS S3, MinIO, etc.
storageB := newS3Client(...)

svc, _ := replication.NewService(replication.Config{Clock: clock})
_ = svc.RegisterWorkflow(replication.Workflow{
    Name:         "global-media",
    SourceRegion: "us-east-1",
    Retention:    replication.RetentionPolicy{MaxVersions: 3, MaxAge: 30 * 24 * time.Hour},
    Targets: []replication.Target{
        {
            Name:         "primary-us",
            Region:       "us-east-1",
            Bucket:       "arrgo-media-primary",
            Prefix:       "media",
            StorageClass: "STANDARD",
            Client:       storageA,
        },
        {
            Name:         "secondary-eu",
            Region:       "eu-central-1",
            Bucket:       "arrgo-media-secondary",
            Prefix:       "media",
            StorageClass: "STANDARD_IA",
            Client:       storageB,
        },
    },
})
```

### Replication Requests
When a job completes, the worker issues a `ReplicationRequest` containing the workflow
name, logical object key (e.g., `series/the-expanse/s01e01.mkv`), and an `io.ReadSeeker`
for the file contents. The service generates a version string (RFC 3339-like, safe for
S3 object keys), writes the payload to every target, and then enforces retention. Each
successful replication returns metadata describing the destination key, storage class, and
any objects deleted due to retention.

## Retention Policies
Retention is policy-driven and enforced per target after a successful upload:

- `MaxVersions` keeps only the newest N versions of an asset (per logical object key).
- `MaxAge` trims artifacts older than the configured duration.
- Target-level retention overrides allow colder tiers (e.g., Glacier Deep Archive) to
  retain more history than faster regions if desired.

The policy engine lists objects using the generated prefix (`<prefix>/<object-key>/`) and
removes any entries exceeding the configured limits while ensuring the just-uploaded
object is never deleted.

## S3 Compatibility
The abstraction accepts any client that implements the minimal `ObjectStorageClient`
interface (`PutObject`, `ListObjects`, and `DeleteObject`). Providers such as AWS S3,
MinIO, Ceph RGW, Wasabi, and Backblaze B2 can plug in so long as they expose compatible
APIs. The service passes through storage class hints and metadata; advanced features like
replication metrics or encryption envelopes can be layered in the concrete client.

## Operational Notes
- **Observability** – every replication attempt emits structured logs detailing the
  workflow, target, generated version, and retention actions. Hook the service into the
  existing OpenTelemetry pipeline by wrapping the storage client.
- **Failure handling** – errors are recorded per target and surfaced collectively. A
  single target failure does not prevent the remaining destinations from completing, and
  the caller receives the aggregated error list via `errors.Join` for requeue/backoff
  handling.
- **Future integration** – the next step is wiring the library import pipeline so the
  worker posts replication requests after successful imports, then exposing progress in
  the dashboard.

## Observability & Operations
- Track replication latency per target (`replication_duration_seconds`) and the
  size of queued replication jobs. Alert if lag exceeds retention windows.
- Emit events when retention policies prune historical versions so operators can
  audit deletion activity.
- Expose workflow status in the dashboard, including last success timestamp and
  outstanding retries per target.

## Future Enhancements
- Support checksum validation at the destination to detect silent data
  corruption.
- Integrate with cold storage tiers (Glacier, Deep Archive) and customizable
  lifecycle transitions.
- Allow conditional replication based on media attributes (e.g., only replicate
  high-value titles or specific quality tiers).
