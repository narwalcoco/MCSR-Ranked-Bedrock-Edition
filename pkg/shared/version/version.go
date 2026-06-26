// Package version provides build-time version info shared by all services.
package version

import "runtime"

// These can be overridden at build time:
//
//	go build -ldflags "-X github.com/mcsr-ranked-bedrock/pkg/shared/version.Version=v1.2.3"
const (
	// Version is the semantic version of the build.
	Version = "0.1.0-dev"

	// Commit is the git commit hash the build was produced from.
	Commit = "unknown"

	// BuildTime is the RFC3339 timestamp of the build.
	BuildTime = "unknown"
)

// Info describes a binary for diagnostics.
type Info struct {
	Service   string `json:"service"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Get returns the version info for the given service name.
func Get(service string) Info {
	return Info{
		Service:   service,
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
}
