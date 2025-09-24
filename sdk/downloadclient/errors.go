package downloadclient

import "errors"

var (
	// ErrClientUnavailable should be returned from Enqueue to signal a temporary failure.
	ErrClientUnavailable = errors.New("downloadclient: client temporarily unavailable")
)
