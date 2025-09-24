// Package downloadclient defines the public SDK for authoring download client
// adapters that can be consumed by arrgo's download router. The types exported
// here intentionally mirror the internal router contract so that third-party
// maintainers can implement integrations without depending on internal
// packages.
package downloadclient

// Request captures the information required to enqueue a download operation.
type Request struct {
	// MediaType distinguishes between movies, series, music, etc.
	MediaType string
	// Protocol identifies the transport expected by the client (e.g. torrent, usenet).
	Protocol string
	// Tags allow routing hints such as preferred regions or capabilities.
	Tags []string
}
