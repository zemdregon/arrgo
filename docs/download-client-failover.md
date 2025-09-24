# Download Client Failover and Health-Aware Routing

arrgo now routes download requests through a prioritized failover stack so that torrents, Usenet jobs, and future transport plug-ins continue processing even when an individual client becomes unavailable. The integration lives in [`internal/integrations/download`](../internal/integrations/download) and exposes a router capable of inspecting capabilities, verifying health, and dispatching work to the highest-priority healthy client.

## Router Overview

The download router accepts one or more client adapters that implement the following interface (exported for third-party use in [`sdk/downloadclient`](../sdk/downloadclient)):

```go
// Client models an individual download client integration that can accept work.
type Client interface {
        Name() string
        Priority() int
        Supports(download.Request) bool
        CheckHealth(context.Context) error
        Enqueue(context.Context, download.Request) error
}
```

- **Priority** drives deterministic routing. Lower numbers are preferred over higher numbers.
- **Supports** allows an adapter to indicate whether it can handle a specific request (e.g., only torrent jobs).
- **CheckHealth** provides active health probing and is invoked whenever a cached health decision is stale or missing.
- **Enqueue** submits the work to the underlying download client.

Clients are registered via `Router.RegisterClient`. Registration enforces unique names and keeps the internal ordering stable even as new adapters are added.

## Health Probing and TTLs

The router maintains per-client health state and records the timestamp of the last successful probe. When a request arrives, it refreshes the health of the highest-priority client if the cached result is older than the configured TTL. Failing probes mark the client unhealthy and capture the error for observability. Healthy results reset the TTL window.

`RouterConfig` exposes a `HealthTTL` option that controls how frequently health checks run. Setting the TTL to zero forces a health probe on every dispatch, which is useful during adapter development or for clients that require near-real-time availability signals.

## Failover Semantics

When dispatching a request, the router evaluates every registered client that supports the request, ordered by priority. The first healthy client to successfully accept the request wins. If a client reports `downloadclient.ErrClientUnavailable` from `Enqueue`, the router immediately marks it unhealthy, logs the failure, and retries the next candidate in priority order. Other errors are surfaced to the caller to preserve adapter-specific error reporting.

If all candidates are unhealthy, the router returns `download.ErrNoHealthyClients` (joined with the most recent probe failure) so that upstream workflows can surface actionable telemetry. When no registered client supports the request, `download.ErrNoSupportingClients` is returned.

## Example Adapter Skeleton

```go
router := download.NewRouter(download.RouterConfig{HealthTTL: 30 * time.Second})

client := &qbittorrentAdapter{ /* ... */ }
if err := router.RegisterClient(client); err != nil {
        log.Fatalf("register client: %v", err)
}

result, err := router.Dispatch(ctx, download.Request{Protocol: "torrent", MediaType: "series"})
if err != nil {
        return fmt.Errorf("dispatch download: %w", err)
}
log.Printf("queued download via %s after %d attempt(s)", result.ClientName, result.Attempts)
```

Adapters should wrap transient failures with `downloadclient.ErrClientUnavailable` to opt into the automatic failover behavior. Persistent errors can be returned directly and will bubble up to the caller.

## Observability

The router logs client registrations, health probe failures, and successful dispatches. Integrations should enrich errors returned from `CheckHealth` and `Enqueue` with contextual metadata (e.g., remote host, HTTP status, RPC code) to aid troubleshooting. Future work will add Prometheus metrics around dispatch latency, failover frequency, and health probe outcomes.
