package listingester

import "context"

// Provider models an external list integration capable of syncing media items.
type Provider interface {
	// Name returns a stable identifier for the provider instance. Names must
	// be unique within a manager to avoid collisions when registering
	// multiple accounts for the same upstream service.
	Name() string
	// Supports reports whether the provider can service the supplied list
	// request. Providers should return false when the media type, list kind,
	// or other attributes are unsupported so the manager can skip the sync.
	Supports(ListRequest) bool
	// CheckHealth validates the current health of the provider. Providers
	// should perform lightweight probes and return ErrProviderUnavailable
	// when upstream APIs are temporarily unreachable so the manager can
	// back off automatically.
	CheckHealth(context.Context) error
	// Sync performs a synchronization cycle for the requested list. When a
	// provider cannot service the request due to a transient issue it
	// should return ErrProviderUnavailable to trigger failover and retry
	// semantics in the manager.
	Sync(context.Context, SyncRequest) (*SyncResult, error)
}
