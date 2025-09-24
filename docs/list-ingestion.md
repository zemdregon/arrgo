# External List Ingestion

arrgo synchronizes external watchlists, custom collections, and listening history through a
pluggable list ingestion framework. The goal is to mirror Sonarr/Radarr/Lidarr behaviour so
operators can import Trakt, IMDb, and Last.fm feeds without bespoke automation while keeping
the system open for additional providers.

## Architecture Overview

The ingestion pipeline is anchored by the [`list.Manager`](../internal/integrations/list/manager.go)
which coordinates incremental syncs across registered providers. Each provider implements the
[`listingester.Provider`](../sdk/listingester/provider.go) interface, exposing support checks,
health probing, and sync semantics that the manager consumes. Providers are dynamically
registered so new integrations can be added without touching the manager core.

```
+----------------+       +----------------------+       +-------------------------+
|  list.Manager  | <---> | listingester.Provider | <---> | External Service (API) |
+----------------+       +----------------------+       +-------------------------+
        |                            ^                                ^
        |                            |                                |
        |                  Provider implementations         Trakt / IMDb / Last.fm
        v
  Library services consume sync outcomes to queue imports, reconciliation jobs, and
  dashboards while preserving provider-specific metadata.
```

The manager tracks subscription state (cursors, identity fields, snapshots) so delta syncs only
process new or removed items. Health checks are cached with a configurable TTL so transient API
outages automatically pause syncs until the provider reports healthy again.

## Provider SDK

Provider authors build against the [`sdk/listingester`](../sdk/listingester) package which
exports the stable contract used by arrgo:

- [`Provider`](../sdk/listingester/provider.go) exposes `Name`, `Supports`, `CheckHealth`, and
  `Sync` methods. Returning `listingester.ErrProviderUnavailable` signals transient failures and
  triggers automatic backoff.
- [`ListRequest`, `SyncRequest`, and `SyncResult`](../sdk/listingester/types.go) model upstream
  list metadata, incremental cursors, and item payloads. Items include arbitrary metadata maps so
  providers can surface provider-specific attributes without schema changes.
- [`sdk/listingester/contract`](../sdk/listingester/contract/contract.go) ships reusable tests so
  third-party maintainers can validate behaviour prior to publishing a plugin.

Adapters bundle with arrgo or ship independently (e.g. as Go modules). Operators simply register
provider instances with the manager during bootstrap.

## Manager Usage

The manager API mirrors the download router to keep integrations familiar:

```go
manager := list.NewManager(list.Config{})
manager.RegisterProvider(traktProvider)
manager.RegisterProvider(imdbProvider)

outcome, err := manager.Sync(ctx, list.Subscription{
        ID:       "my-watchlist",
        Provider: traktProvider.Name(),
        List: listingester.ListRequest{
                ListID:    "watchlist",
                MediaType: "movie",
                Kind:      listingester.ListKindWatchlist,
        },
        Mode: listingester.SyncModeDelta,
})
if err != nil {
        // handle health errors or transient provider outages
}
for _, item := range outcome.Added {
        // enqueue library imports or update dashboards
}
```

The manager deduplicates items based on identity fields (Trakt IDs, IMDb IDs, MusicBrainz IDs,
etc.) and emits both incremental deltas and the full snapshot for observability dashboards.

## Built-in Providers

arrgo ships static provider implementations that mirror upstream service semantics. Each
provider wraps the shared [`static.Provider`](../internal/integrations/list/providers/static/provider.go)
for deterministic behaviour while leaving room for HTTP-backed implementations later.

### Trakt

[`internal/integrations/list/providers/trakt`](../internal/integrations/list/providers/trakt)
models Trakt watchlists and custom lists. Entries surface Trakt, IMDb, TMDB, and slug identifiers
alongside year metadata so arrgo can reconcile against existing libraries. The provider name
includes the Trakt username (e.g. `trakt:exampleuser`) to support multiple accounts.

### IMDb

[`internal/integrations/list/providers/imdb`](../internal/integrations/list/providers/imdb)
mirrors IMDb watchlists and top charts. Items expose IMDb/TMDB identifiers plus rank metadata so
existing custom-format rules can weigh releases consistently with the legacy *arr ecosystem.

### Last.fm

[`internal/integrations/list/providers/lastfm`](../internal/integrations/list/providers/lastfm)
handles listening history and curated music lists. It records MusicBrainz, Last.fm, and optional
Spotify identifiers while carrying artist/album metadata for downstream tagging.

## Extending the Ecosystem

To add a new provider:

1. Implement the `listingester.Provider` interface in a new package.
2. Supply configuration structs that translate upstream payloads into `listingester.Item`
   instances. Reuse the static provider helper if you do not need live HTTP calls yet.
3. Validate behaviour using the contract suite and dedicated unit tests.
4. Register the provider with `list.Manager` during service initialization.

The list ingestion framework keeps the manager agnostic of upstream APIs, allowing arrgo to grow
its ecosystem without disrupting existing deployments.
