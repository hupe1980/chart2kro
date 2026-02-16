// Package deps handles Helm subchart dependency resolution.
package deps

import (
	"log/slog"
	"strings"

	"github.com/Masterminds/semver/v3"
	"helm.sh/helm/v3/pkg/chart"
)

// Status represents the state of a dependency.
type Status string

const (
	// StatusOK means the dependency is vendored in charts/.
	StatusOK Status = "ok"
	// StatusMissing means the dependency is declared but not vendored.
	StatusMissing Status = "missing"
	// StatusVersionMismatch means the vendored version doesn't match.
	StatusVersionMismatch Status = "version-mismatch"
)

// DependencyInfo describes a chart dependency and its resolution status.
type DependencyInfo struct {
	Name       string
	Version    string
	Repository string
	Condition  string
	Tags       []string
	Status     Status
	Actual     string // Actual version found (if vendored).
}

// Result contains the outcome of dependency analysis.
type Result struct {
	Dependencies []DependencyInfo
	HasLock      bool
	AllResolved  bool
}

// Analyze inspects a chart's declared dependencies against what is vendored
// in the charts/ directory. It returns a Result describing each dependency's
// resolution status.
func Analyze(ch *chart.Chart, logger *slog.Logger) *Result {
	result := &Result{
		HasLock:     ch.Lock != nil,
		AllResolved: true,
	}

	if len(ch.Metadata.Dependencies) == 0 {
		return result
	}

	// Index vendored subcharts by name.
	vendored := make(map[string]*chart.Chart, len(ch.Dependencies()))
	for _, sub := range ch.Dependencies() {
		if sub.Metadata != nil {
			vendored[sub.Metadata.Name] = sub
		}
	}

	for _, dep := range ch.Metadata.Dependencies {
		info := DependencyInfo{
			Name:       dep.Name,
			Version:    dep.Version,
			Repository: dep.Repository,
			Condition:  dep.Condition,
			Tags:       dep.Tags,
		}

		sub, ok := vendored[dep.Name]
		if !ok {
			info.Status = StatusMissing
			result.AllResolved = false

			logger.Warn("subchart dependency not vendored",
				slog.String("dependency", dep.Name),
				slog.String("version", dep.Version),
				slog.String("repository", dep.Repository),
			)
		} else {
			info.Actual = sub.Metadata.Version
			if dep.Version != "" && !versionSatisfied(dep.Version, sub.Metadata.Version) {
				info.Status = StatusVersionMismatch
				logger.Warn("subchart version mismatch",
					slog.String("dependency", dep.Name),
					slog.String("expected", dep.Version),
					slog.String("actual", sub.Metadata.Version),
				)
			} else {
				info.Status = StatusOK
			}
		}

		result.Dependencies = append(result.Dependencies, info)
	}

	if !result.HasLock && len(ch.Metadata.Dependencies) > 0 {
		logger.Warn("Chart.lock is missing; dependency versions are not pinned (non-reproducible)")
	}

	return result
}

// IsEnabled checks whether a dependency is enabled based on its condition
// and tags evaluated against the provided values.
func IsEnabled(dep *chart.Dependency, vals map[string]interface{}) bool {
	// Check condition first (takes precedence over tags).
	if dep.Condition != "" {
		for _, cond := range strings.Split(dep.Condition, ",") {
			cond = strings.TrimSpace(cond)
			if v, found := lookupValue(vals, cond); found {
				if b, ok := v.(bool); ok {
					return b
				}
			}
		}
	}

	// Check tags.
	if len(dep.Tags) > 0 {
		tags, _ := vals["tags"].(map[string]interface{})
		for _, tag := range dep.Tags {
			if v, found := tags[tag]; found {
				if b, ok := v.(bool); ok && !b {
					return false
				}
			}
		}
	}

	// Default: enabled.
	return true
}

// versionSatisfied checks if actual version satisfies the declared constraint
// using the Masterminds/semver library (the same one used by Helm itself).
func versionSatisfied(constraint, actual string) bool {
	// Exact string match is always satisfied.
	if constraint == actual {
		return true
	}

	c, err := semver.NewConstraint(constraint)
	if err != nil {
		// Unparseable constraint — cannot satisfy.
		return false
	}

	v, err := semver.NewVersion(actual)
	if err != nil {
		// Unparseable version — cannot satisfy.
		return false
	}

	return c.Check(v)
}

// lookupValue resolves a dotted path in a values map.
func lookupValue(vals map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	current := vals

	for i, p := range parts {
		v, ok := current[p]
		if !ok {
			return nil, false
		}

		if i == len(parts)-1 {
			return v, true
		}

		next, ok := v.(map[string]interface{})
		if !ok {
			return nil, false
		}

		current = next
	}

	return nil, false
}

// MissingDependencies returns the names of dependencies that are not vendored.
func MissingDependencies(result *Result) []string {
	var missing []string

	for _, d := range result.Dependencies {
		if d.Status == StatusMissing {
			missing = append(missing, d.Name)
		}
	}

	return missing
}

// PrintSummary writes a dependency resolution summary to the logger.
func PrintSummary(result *Result, logger *slog.Logger) {
	if len(result.Dependencies) == 0 {
		return
	}

	for _, d := range result.Dependencies {
		switch d.Status {
		case StatusOK:
			logger.Info("dependency resolved",
				slog.String("name", d.Name),
				slog.String("version", d.Actual),
			)
		case StatusMissing:
			logger.Error("dependency missing",
				slog.String("name", d.Name),
				slog.String("version", d.Version),
				slog.String("repository", d.Repository),
			)
		case StatusVersionMismatch:
			logger.Warn("dependency version mismatch",
				slog.String("name", d.Name),
				slog.String("expected", d.Version),
				slog.String("actual", d.Actual),
			)
		}
	}

	if !result.HasLock {
		logger.Warn("no Chart.lock found",
			slog.String("hint", "run 'helm dependency update' to generate Chart.lock"),
		)
	}
}
