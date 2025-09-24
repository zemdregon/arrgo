package downloadclient

import "context"

// Client models an individual download client integration that can accept work.
type Client interface {
	// Name is a human readable identifier that must be unique within a router.
	Name() string
	// Priority determines the routing order (lower numbers are preferred).
	Priority() int
	// Supports determines whether the client can process the provided request.
	Supports(Request) bool
	// CheckHealth validates the current health of the client.
	CheckHealth(context.Context) error
	// Enqueue submits the download request to the backing client implementation.
	Enqueue(context.Context, Request) error
}
