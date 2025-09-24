package buildinfo

// Package buildinfo exposes build-time metadata injected by the Docker image
// build. Default values are overridden via -ldflags when the binaries are
// compiled.
var (
	// Version describes the semantic version or git ref of the build.
	Version = "dev"
	// Commit captures the git commit SHA associated with the build.
	Commit = "unknown"
	// BuildTime records when the binary was produced.
	BuildTime = "unknown"
)

// Fields returns structured logging key/value pairs for the build metadata.
func Fields() []any {
	return []any{
		"version", Version,
		"commit", Commit,
		"build_time", BuildTime,
	}
}
