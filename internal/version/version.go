// Package version carries build-time metadata injected via -ldflags.
package version

var (
	// Version is the release version, e.g. v0.1.0 or dev.
	Version = "dev"
	// Commit is the short git commit hash.
	Commit = "none"
	// BuildDate is the UTC build timestamp.
	BuildDate = "unknown"
)

// ci-scope probe
