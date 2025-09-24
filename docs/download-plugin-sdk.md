# Download Client Plugin SDK

> **Last reviewed:** 2025-03-08  
> **Status:** Contract tests available and enforced via `sdk/downloadclient/contract`

The download client SDK exposes a stable contract for third-party maintainers who want to ship adapters alongside arrgo releases. The SDK lives in [`sdk/downloadclient`](../sdk/downloadclient) and mirrors the semantics used internally by the download router while remaining importable from external Go modules.

## Adapter Interface

Adapters implement the [`downloadclient.Client`](../sdk/downloadclient/client.go) interface. The router consumes the resulting instance just like any built-in integration:

```go
import (
        "context"

        "github.com/arrgo/arrgo/sdk/downloadclient"
)

type qbittorrentAdapter struct {
        // ... configuration fields ...
}

func (a *qbittorrentAdapter) Name() string                { return "qbittorrent" }
func (a *qbittorrentAdapter) Priority() int               { return 10 }
func (a *qbittorrentAdapter) Supports(r downloadclient.Request) bool {
        return r.Protocol == "torrent"
}

func (a *qbittorrentAdapter) CheckHealth(ctx context.Context) error {
        // perform adapter-specific probe logic
        return nil
}

func (a *qbittorrentAdapter) Enqueue(ctx context.Context, r downloadclient.Request) error {
        // submit the request to the backing client
        return nil
}
```

Adapters should return `downloadclient.ErrClientUnavailable` from `Enqueue` when a transient failure occurs so the router can automatically fail over to the next candidate without surfacing an error to the caller.

## Contract Tests

The [`sdk/downloadclient/contract`](../sdk/downloadclient/contract) package provides reusable tests so that adapter authors can validate behaviour prior to publishing. The contract verifies metadata, supported request handling, health probing, and enqueue semantics based on scenarios supplied by the adapter under test.

Example usage inside an adapter repository:

```go
package myadapter_test

import (
        "errors"
        "testing"

        "github.com/arrgo/arrgo/sdk/downloadclient"
        "github.com/arrgo/arrgo/sdk/downloadclient/contract"
        "github.com/your/module/myadapter"
)

func TestAdapterContract(t *testing.T) {
        suite := contract.Suite{
                NewClient: func(t *testing.T) downloadclient.Client {
                        cfg := myadapter.Config{ /* ... */ }
                        client, err := myadapter.New(cfg)
                        if err != nil {
                                t.Fatalf("new adapter: %v", err)
                        }
                        return client
                },
                SupportedRequests: []downloadclient.Request{{Protocol: "torrent"}},
                UnsupportedRequests: []downloadclient.Request{{Protocol: "usenet"}},
                HealthCases: []contract.HealthCase{
                        { Name: "healthy" },
                        {
                                Name: "unhealthy",
                                Setup: func(t *testing.T, client downloadclient.Client) {
                                        client.(*myadapter.Client).SetHealthError(errors.New("down"))
                                },
                                Assert: contract.AssertError,
                        },
                },
                EnqueueCases: []contract.EnqueueCase{
                        {
                                Name:    "transient",
                                Request: downloadclient.Request{Protocol: "torrent"},
                                Setup: func(t *testing.T, client downloadclient.Client) {
                                        client.(*myadapter.Client).SetEnqueueError(downloadclient.ErrClientUnavailable)
                                },
                                Assert: contract.AssertErrorIs(downloadclient.ErrClientUnavailable),
                        },
                },
        }

        contract.Run(t, suite)
}
```

The suite is intentionally flexible: adapters can provide any number of health or enqueue scenarios, inject contexts, and reuse the exported assertion helpers to describe the expected outcomes.

## Operational Guidance
- Keep adapter names stable so that arrgo operators can reference them in
  configuration files and dashboards.
- Emit structured logs for health probes and enqueue attempts to make
  troubleshooting easier.
- Ship adapters with semantic versioning and changelogs so operators can pin
  compatible versions across clusters.

## Future Enhancements
- Expand the contract suite with performance benchmarks to validate adapter
  throughput under parallel dispatch.
- Publish packaging templates (Go modules, Docker images) that demonstrate how
  to distribute adapters independently from arrgo releases.
- Provide SDK shims for non-Go languages if the community demands alternative
  runtimes for adapters.
