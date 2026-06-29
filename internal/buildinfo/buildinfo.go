// Package buildinfo holds version metadata stamped in at build time via -ldflags.
package buildinfo

// These values are overridden at build time, e.g.:
//
//	go build -ldflags "-X save-drive/internal/buildinfo.Version=1.2.0" ./cmd/save-drive
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
