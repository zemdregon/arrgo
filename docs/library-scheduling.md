# Library Scheduling

The library scheduler automates recurring scan and import workflows by wiring cron-based
triggers directly to the orchestration queue. It ensures that large media libraries stay
current without manual intervention and keeps worker utilization predictable via explicit
concurrency guards.

## Components

- **Scheduler (`internal/library/scheduler.go`):** Parses configured schedules and uses
  [`github.com/robfig/cron`](https://pkg.go.dev/github.com/robfig/cron/v3) to execute them on time.
- **Library service (`internal/library/service.go`):** Provides entity discovery, job
  orchestration, and audit fan-out once jobs complete.
- **Audit sink (`internal/library/audit.go`):** Emits structured events for downstream
  notification or analytics pipelines whenever an import completes.

## Configuring Schedules

Construct a `library.Scheduler` with one or more `Schedule` definitions:

```go
scheduler, err := library.NewScheduler(library.SchedulerConfig{
    Service: libraryService,
    Schedules: []library.Schedule{
        {
            EntityType:    "series",
            JobType:       library.JobTypeScan,
            Cron:          "0 */6 * * *", // every six hours
            MaxConcurrent: 4,
        },
        {
            EntityType:    "movie",
            JobType:       library.JobTypeImport,
            Cron:          "15 * * * *", // at minute 15 every hour
            MaxConcurrent: 2,
            PayloadURI:    "s3://arrgo/imports/movies/",
        },
    },
})
if err != nil {
    log.Fatal(err)
}
ctx, cancel := context.WithCancel(context.Background())
var wg sync.WaitGroup
wg.Add(1)
go func() {
    defer wg.Done()
    scheduler.Start(ctx)
}()
// … later, when shutting down:
cancel()
scheduler.Stop()
wg.Wait()
```

Each schedule enforces a `MaxConcurrent` ceiling per media type. The scheduler will only
queue new jobs when fewer than the configured number are currently in the queued or running
states, preventing worker pools from being overwhelmed by large bursts.

## How Dispatching Works

1. The scheduler lists all entities that match the configured `EntityType`.
2. It counts existing jobs of the same `JobType` that target those entities and are still
   queued or running.
3. If the concurrency window allows, it triggers new jobs through `Service.TriggerJob`, which
   persists them and enqueues them on the Redis-backed orchestration stream.
4. Workers claim jobs as usual via `Service.ClaimJobs`; when they acknowledge completion, the
   scheduler's audit integration fires.

## Audit Events

Import completions raise a `library.import.completed` audit event with the job, entity, and
completion timestamp. Downstream systems can subscribe to these events to power
notifications, dashboards, or other side effects.

```go
type AuditEvent struct {
    EventType   string
    JobID       string
    JobType     string
    TargetID    string
    EntityType  string
    CompletedAt time.Time
}
```

Provide a custom `AuditSink` via `library.Config{AuditSink: mySink}` to persist events in a
message bus or metrics backend. A default no-op sink keeps the service lightweight when
additional plumbing is not yet required.

## Operational Notes

- Cron expressions are parsed using the "with seconds" format, enabling high-frequency scans
  such as `@every 30s` during development.
- Scheduling is idempotent with respect to in-flight jobs: entities already queued or running
  are skipped until their job transitions to `completed`.
- Audit publication is best-effort. Failures are logged but do not fail job acknowledgement
  to keep the worker pipeline flowing.
- Ensure the library service is constructed with a Redis client so jobs can be enqueued. The
  scheduler returns `ErrQueueUnavailable` if the orchestration queue is not configured.
