package filter

import (
	"context"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// ProfileConfig describes a reusable set of filter rules that can be
// applied by name via --profile.
type ProfileConfig struct {
	// ExcludeSubcharts lists subchart names to exclude.
	ExcludeSubcharts []string `json:"excludeSubcharts,omitempty"`
	// ExcludeKinds lists Kubernetes kinds to exclude.
	ExcludeKinds []string `json:"excludeKinds,omitempty"`
	// ExternalResources contains externalization mappings.
	// Each entry is "kind:name=schemaField" (e.g., "Secret:postgresql=ext.secretName").
	ExternalResources []string `json:"externalResources,omitempty"`
	// Extends names a built-in profile to extend with these additional rules.
	Extends string `json:"extends,omitempty"`
}

// builtinProfiles contains the built-in profile definitions.
var builtinProfiles = map[string]ProfileConfig{
	"enterprise": {
		ExcludeSubcharts: []string{
			"postgresql", "mysql", "mariadb", "mongodb",
			"redis", "memcached",
			"rabbitmq", "kafka",
		},
	},
	"minimal": {
		// Minimal: exclude ALL subcharts. This is handled specially
		// because we don't know subchart names in advance — all resources
		// with a non-empty SourceChart() are excluded.
	},
	"app-only": {
		ExcludeKinds: []string{
			"StatefulSet",
			"PersistentVolumeClaim",
		},
	},
}

// BuiltinProfileNames returns the names of all built-in profiles.
func BuiltinProfileNames() []string {
	names := make([]string, 0, len(builtinProfiles))
	for name := range builtinProfiles {
		names = append(names, name)
	}

	return names
}

// ResolveProfile resolves a profile name to its configuration by checking
// built-in profiles first, then custom profiles. Returns an error if the
// profile name is not found in either source.
func ResolveProfile(name string, custom map[string]ProfileConfig) (ProfileConfig, error) {
	// Check built-in profiles.
	if p, ok := builtinProfiles[name]; ok {
		return p, nil
	}

	// Check custom profiles.
	if custom != nil {
		if p, ok := custom[name]; ok {
			// If the custom profile extends a built-in, merge them.
			if p.Extends != "" {
				base, err := ResolveProfile(p.Extends, nil)
				if err != nil {
					return ProfileConfig{}, fmt.Errorf("profile %q extends unknown profile %q", name, p.Extends)
				}

				return mergeProfiles(base, p), nil
			}

			return p, nil
		}
	}

	return ProfileConfig{}, fmt.Errorf("unknown profile %q", name)
}

// mergeProfiles merges an extension profile on top of a base profile.
func mergeProfiles(base, ext ProfileConfig) ProfileConfig {
	merged := ProfileConfig{
		ExcludeSubcharts:  append(append([]string{}, base.ExcludeSubcharts...), ext.ExcludeSubcharts...),
		ExcludeKinds:      append(append([]string{}, base.ExcludeKinds...), ext.ExcludeKinds...),
		ExternalResources: append(append([]string{}, base.ExternalResources...), ext.ExternalResources...),
	}

	return merged
}

// BuildFiltersFromProfile creates the filter chain for a resolved profile.
func BuildFiltersFromProfile(p ProfileConfig) ([]Filter, error) {
	var filters []Filter

	if len(p.ExcludeSubcharts) > 0 {
		filters = append(filters, NewSubchartFilter(p.ExcludeSubcharts))
	}

	if len(p.ExcludeKinds) > 0 {
		filters = append(filters, NewKindFilter(p.ExcludeKinds))
	}

	if len(p.ExternalResources) > 0 {
		mappings, err := parseExternalResources(p.ExternalResources)
		if err != nil {
			return nil, err
		}

		filters = append(filters, NewExternalRefFilter(mappings))
	}

	return filters, nil
}

// parseExternalResources parses "kind:name=schemaField" entries.
func parseExternalResources(entries []string) ([]ExternalMapping, error) {
	var mappings []ExternalMapping

	for _, entry := range entries {
		colonIdx := strings.Index(entry, ":")
		if colonIdx <= 0 {
			return nil, fmt.Errorf("invalid external resource %q: expected kind:name=schemaField", entry)
		}

		kind := entry[:colonIdx]
		rest := entry[colonIdx+1:]

		m, err := ParseExternalMapping(kind, rest)
		if err != nil {
			return nil, err
		}

		mappings = append(mappings, m)
	}

	return mappings, nil
}

// IsMinimalProfile returns true if the named profile is the "minimal" profile
// which has special behavior (excludes ALL subchart resources).
func IsMinimalProfile(name string) bool {
	return name == "minimal"
}

// MinimalFilter excludes all resources that originate from any subchart.
type MinimalFilter struct{}

// NewMinimalFilter creates a filter that excludes ALL subchart resources.
func NewMinimalFilter() *MinimalFilter {
	return &MinimalFilter{}
}

// Apply excludes every resource with a non-empty SourceChart.
func (f *MinimalFilter) Apply(_ context.Context, resources []*k8s.Resource) (*Result, error) {
	r := NewResult()

	for _, res := range resources {
		if sc := res.SourceChart(); sc != "" {
			r.Excluded = append(r.Excluded, ExcludedResource{
				Resource: res,
				Reason:   fmt.Sprintf("excluded by minimal profile (subchart: %s)", sc),
			})
		} else {
			r.Included = append(r.Included, res)
		}
	}

	return r, nil
}

// LoadCustomProfiles loads custom profile definitions from a YAML file.
// The file should contain a top-level "profiles" key.
func LoadCustomProfiles(path string) (map[string]ProfileConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is user-provided config file
	if err != nil {
		return nil, fmt.Errorf("reading profiles file: %w", err)
	}

	return ParseCustomProfiles(data)
}

// ParseCustomProfiles parses profile definitions from YAML bytes.
func ParseCustomProfiles(data []byte) (map[string]ProfileConfig, error) {
	var raw struct {
		Profiles map[string]ProfileConfig `json:"profiles"`
	}

	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing profiles: %w", err)
	}

	if raw.Profiles == nil {
		return make(map[string]ProfileConfig), nil
	}

	return raw.Profiles, nil
}
