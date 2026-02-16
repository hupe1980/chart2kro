// Package version provides build-time metadata for the chart2kro binary.
// Version, GitCommit, and BuildDate are injected at compile time via -ldflags.
package version

import (
	"encoding/json"
	"fmt"
	"runtime"
)

// Build-time values injected via -ldflags.
var (
	version   = "dev"
	gitCommit = "none"
	buildDate = "unknown"
)

// Info holds the build metadata for the binary.
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// GetInfo returns the current build information.
func GetInfo() Info {
	return Info{
		Version:   version,
		GitCommit: shortCommit(gitCommit),
		BuildDate: buildDate,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a human-readable single-line version string.
func (i Info) String() string {
	return fmt.Sprintf("chart2kro %s (commit: %s, built: %s, %s %s)",
		i.Version, i.GitCommit, i.BuildDate, i.GoVersion, i.Platform)
}

// JSON returns the version info as indented JSON.
func (i Info) JSON() (string, error) {
	data, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling version info: %w", err)
	}

	return string(data), nil
}

// shortCommit truncates a commit SHA to 7 characters.
func shortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}

	return commit
}
