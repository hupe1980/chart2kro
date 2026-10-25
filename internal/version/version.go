// Package version holds build information set via -ldflags.
package version

import "runtime/debug"

var (
	version   = "dev"
	gitCommit = ""
	buildDate = ""
)

// Version is the release version (e.g. v0.1.0) or "dev".
func Version() string {
	if version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
	}

	return version
}

// Commit is the short git commit, if known.
func Commit() string {
	if gitCommit != "" {
		return gitCommit
	}

	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return s.Value[:7]
			}
		}
	}

	return "unknown"
}

// BuildDate is the build timestamp, if set.
func BuildDate() string {
	if buildDate == "" {
		return "unknown"
	}

	return buildDate
}
