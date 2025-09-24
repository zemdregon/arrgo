// Package listingester defines the public SDK for authoring external list
// ingestion providers that plug into arrgo's list manager. The types exported
// here mirror the internal contracts so that third-party maintainers can build
// adapters without importing internal packages.
package listingester

import "time"

// ListKind distinguishes between common upstream list types.
type ListKind string

const (
	// ListKindCollection represents a user's collected media library.
	ListKindCollection ListKind = "collection"
	// ListKindWatchlist represents a queue of media to acquire next.
	ListKindWatchlist ListKind = "watchlist"
	// ListKindHistory represents a history or scrobble log (e.g. Last.fm).
	ListKindHistory ListKind = "history"
	// ListKindCustom represents a provider-specific custom list type.
	ListKindCustom ListKind = "custom"
)

// ListRequest describes the upstream list to be synchronized.
type ListRequest struct {
	// ListID identifies the upstream list, playlist, or watchlist slug.
	ListID string
	// MediaType distinguishes movies, series, albums, tracks, etc.
	MediaType string
	// Kind captures the upstream list classification.
	Kind ListKind
	// Tags provide provider-specific hints (e.g. privacy level, owner).
	Tags []string
}

// SyncMode controls how a provider should process a synchronization cycle.
type SyncMode string

const (
	// SyncModeDelta requests only changes since the previous cursor.
	SyncModeDelta SyncMode = "delta"
	// SyncModeFull requests a full snapshot irrespective of prior cursor.
	SyncModeFull SyncMode = "full"
)

// SyncRequest is supplied to providers when executing a synchronization.
type SyncRequest struct {
	// List captures the target list context.
	List ListRequest
	// Mode instructs the provider whether a delta or full sync is desired.
	Mode SyncMode
	// Cursor represents provider-defined state used for incremental syncs.
	Cursor string
	// Limit allows callers to bound the number of items returned in a cycle.
	Limit int
}

// Item identifies an individual media entry discovered in an upstream list.
type Item struct {
	MediaType   string
	Title       string
	ExternalIDs map[string]string
	AddedAt     time.Time
	Metadata    map[string]any
}

// ItemRef references an item when conveying removals from a list.
type ItemRef struct {
	MediaType   string
	Title       string
	ExternalIDs map[string]string
}

// SyncResult captures the outcome of a synchronization attempt.
type SyncResult struct {
	Items      []Item
	Removed    []ItemRef
	NextCursor string
	HasMore    bool
	Reset      bool
	Stats      map[string]int
}
