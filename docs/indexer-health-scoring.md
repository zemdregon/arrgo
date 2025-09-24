# Indexer Health Scoring and Discovery

> **Last reviewed:** 2025-03-08  
> **Status:** Implemented with monitoring hooks in `internal/integrations/indexer`

arrgo now tracks indexer reliability automatically so that operators can set failover thresholds based on live traffic instead of manual guesswork. The monitoring logic lives in [`internal/integrations/indexer`](../internal/integrations/indexer) and pairs rolling health scores with discovery hooks so that new indexers are incorporated as soon as they respond to requests or are surfaced by pluggable discovery sources.

## Monitor Overview

The package exposes a `Monitor` that ingests observations and keeps per-indexer health state:

```go
monitor := indexer.NewMonitor(indexer.Config{})
monitor.Record(indexer.Observation{Indexer: "Alpha", Outcome: indexer.OutcomeSuccess, Latency: 250 * time.Millisecond})
monitor.Record(indexer.Observation{Indexer: "Alpha", Outcome: indexer.OutcomeFailure, Error: errors.New("http 502")})
reports := monitor.Snapshot()
```

Each `Observation` captures the indexer identifier, request outcome, latency, and optional error. Outcomes may be:

- `indexer.OutcomeSuccess`
- `indexer.OutcomeFailure`
- `indexer.OutcomeTimeout`

A new indexer is discovered automatically the first time it generates an observation. For proactive registration, discovery sources implementing `indexer.Discoverer` can be attached during construction or via `RegisterDiscoverer`. Invoking `Monitor.Discover(ctx)` queries each source, updates metadata, and returns the unique set of indexers that were found.

## Health Scoring

The monitor maintains a rolling sample window (default 50 observations) per indexer and applies weighted penalties to derive a 0–100 score:

- Successes contribute positively to the score.
- Failures subtract the configured failure penalty (default 40) multiplied by the failure rate in the window.
- Timeouts subtract the timeout penalty (default 70) multiplied by the timeout rate.
- Slow responses above the latency target (default 500ms) subtract up to the latency penalty (default 15) based on how far they exceed the target.

Scores are clamped between 0 and 100 and marked healthy when they meet or exceed the configured healthy threshold (default 80). Reports produced by `Snapshot` include:

- `Score`, `Availability`, `FailureRate`, and `TimeoutRate`
- Average latency for successful requests
- Timestamps for the last observation and discovery
- The most recent error string (if any)
- Metadata gathered via discovery sources

Without any observations, an indexer is considered neutral and reported with the healthy baseline score so that onboarding does not penalize aggregate thresholds.

## Failover Recommendations

`Monitor.RecommendFailover()` inspects the current score distribution to suggest a threshold for routing decisions. Scores with at least five observations participate in the percentile calculation. By default, the monitor returns the 20th percentile score across participating indexers, rounded to a single decimal place, and clamps the result between the configured healthy score and a floor score (default equal to the degraded score of 60). A `Confidence` ratio indicates how many tracked indexers had sufficient data to influence the recommendation.

Operators can feed the suggested score directly into failover policies or use it as a starting point when defining degraded and critical alerting bands.

## Discovery Metadata

Discovery sources return lightweight `indexer.Metadata` records containing the indexer name, endpoint, supported categories, and optional tags. The monitor stores the latest metadata alongside health signals so that dashboards and APIs can show where the data came from without requiring separate bookkeeping.

## Configuration Highlights

Key tunables exposed via `indexer.Config`:

| Option | Purpose |
| --- | --- |
| `SampleWindow` | Number of recent observations retained per indexer (default 50). |
| `FailurePenalty` / `TimeoutPenalty` / `LatencyPenalty` | Weighting applied when computing scores. |
| `LatencyTarget` | Baseline expected latency for successful responses. |
| `HealthyScore` / `DegradedScore` | Thresholds for flagging healthy vs. degraded states. |
| `FailoverFloorScore` | Minimum score that `RecommendFailover` will return, preventing overly permissive thresholds. |
| `MinObservationsForRecommendation` | Minimum observations required for an indexer to influence the failover recommendation. |
| `Discoverers` | Optional list of discovery sources consulted by `Monitor.Discover`. |

These defaults match the requirements captured in the roadmap item: score indexer reliability automatically, surface context for each discovery, and deliver actionable recommendations that eliminate manual threshold tuning.

## Operational Monitoring
- Export monitor snapshots to Prometheus via gauges/histograms (`indexer_health_score`,
  `indexer_latency_seconds`) to trend reliability over time.
- Emit structured events when discovery sources add or remove indexers so that
  dashboards and audit logs stay synchronized.
- Surface failover recommendations in the dashboard with the underlying
  percentile, sample size, and number of participating indexers to build trust
  in the automation.

## Future Enhancements
- Incorporate request volume weighting so high-throughput indexers have more
  influence on failover thresholds than rarely used ones.
- Persist health history in Postgres to support long-term reporting and anomaly
  detection.
- Allow per-indexer maintenance windows that suppress alerts and avoid penalizing
  known downtime.
