# Performance and Regression Testing

Automated suites exercise library orchestration behavior against large catalogs so regressions are caught before they reach production-scale deployments.

## Scheduler fan-out regression
- `TestSchedulerLargeLibraryFanout` provisions a library with 10,000 entities, runs the cron-backed scheduler, and asserts that the run completes within five seconds while honoring the configured concurrency ceiling of 512 queued jobs.
- The test also verifies that each queued job targets a unique entity so no duplicates slip through as the scheduler loops over the catalog.

Run the fan-out regression by executing:

```bash
go test ./internal/library -run TestSchedulerLargeLibraryFanout -count=1 -v
```

## Queue drain regression
- `TestQueueHighVolumeDrain` seeds 6,000 queued jobs and drains them in 256-job batches, ensuring that every job is acknowledged, all Redis stream IDs remain unique, and the full drain completes within the defined timeout.
- Final state assertions guarantee that the in-memory job log records each job as completed once the backlog is processed.

Run the drain regression with:

```bash
go test ./internal/library -run TestQueueHighVolumeDrain -count=1 -v
```

## Scheduler benchmark
- `BenchmarkSchedulerRunLargeLibrary` repeatedly fans out work for a 4,000-entity library to provide a reproducible baseline for scheduler throughput. The benchmark resets the Redis queue and in-memory job log between iterations to isolate scheduler cost.

Execute the benchmark locally via:

```bash
go test ./internal/library -bench BenchmarkSchedulerRunLargeLibrary -run '^$'
```

## Integrating with CI
- Incorporate the regression tests into nightly or pre-release workflows to surface scaling issues early while keeping routine developer runs fast.
- Capture benchmark output across releases to spot throughput regressions; persisting historical results makes performance trends easy to visualize.
- For deeper profiling, wrap benchmark executions with `go test -benchmem` and `pprof` captures so flame graphs highlight new bottlenecks.
