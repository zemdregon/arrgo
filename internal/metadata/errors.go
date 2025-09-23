package metadata

import (
	"errors"
	"time"
)

var (
	// ErrNoProviders indicates that the pipeline was constructed without any
	// metadata providers.
	ErrNoProviders = errors.New("metadata: no providers configured")
	// ErrMetadataNotFound is returned when no provider produced metadata for the
	// requested entity.
	ErrMetadataNotFound = errors.New("metadata: no metadata found for request")
)

// RateLimitError conveys provider throttling responses to the pipeline.
type RateLimitError interface {
	error
	RetryAfter() time.Duration
}

type rateLimitError struct {
	msg        string
	retryAfter time.Duration
	cause      error
}

// NewRateLimitError constructs an error signalling that the caller was rate limited.
func NewRateLimitError(message string, retryAfter time.Duration) error {
	return &rateLimitError{msg: message, retryAfter: retryAfter}
}

// WrapRateLimitError annotates an existing error with rate limiting metadata.
func WrapRateLimitError(err error, retryAfter time.Duration) error {
	if err == nil {
		return nil
	}
	return &rateLimitError{msg: err.Error(), retryAfter: retryAfter, cause: err}
}

func (e *rateLimitError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return "metadata: provider rate limited"
}

func (e *rateLimitError) Unwrap() error {
	return e.cause
}

func (e *rateLimitError) RetryAfter() time.Duration {
	return e.retryAfter
}

// AsRateLimitError extracts rate limiting metadata from an error value.
func AsRateLimitError(err error) (RateLimitError, bool) {
	var rl RateLimitError
	if errors.As(err, &rl) {
		return rl, true
	}
	return nil, false
}
