package listingester

import "errors"

var (
	// ErrProviderUnavailable signals that the upstream provider is
	// temporarily unavailable and the caller should retry later or fail over
	// to an alternate provider.
	ErrProviderUnavailable = errors.New("listingester: provider unavailable")
	// ErrUnsupportedList indicates that the provider cannot service the
	// requested list and no retry should be attempted.
	ErrUnsupportedList = errors.New("listingester: unsupported list")
)
