package metadata

import "errors"

var (
	// ErrNoProviders indicates that the pipeline was constructed without any
	// metadata providers.
	ErrNoProviders = errors.New("metadata: no providers configured")
	// ErrMetadataNotFound is returned when no provider produced metadata for the
	// requested entity.
	ErrMetadataNotFound = errors.New("metadata: no metadata found for request")
)
