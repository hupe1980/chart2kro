package filter

import (
	"fmt"
	"strings"

	"github.com/hupe1980/chart2kro/internal/helm/chartmeta"
)

// knownExternalPatterns maps subchart names to their conventional
// external values key prefixes. These patterns are used by Helm community
// charts (e.g., Bitnami) to switch between a bundled subchart and an
// external managed service.
var knownExternalPatterns = map[string][]string{
	"postgresql": {"externalDatabase"},
	"mysql":      {"externalDatabase", "externalMysql"},
	"mariadb":    {"externalDatabase", "externalMariadb"},
	"mongodb":    {"externalMongodb", "externalDatabase"},
	"redis":      {"externalRedis"},
	"memcached":  {"externalMemcached"},
	"rabbitmq":   {"externalRabbitmq"},
	"kafka":      {"externalKafka"},
}

// DetectedPattern describes an auto-detected subchart/external pattern.
type DetectedPattern struct {
	// SubchartName is the subchart to exclude (e.g., "postgresql").
	SubchartName string
	// Condition is the enable/disable condition from Chart.yaml (e.g., "postgresql.enabled").
	Condition string
	// ExternalValuesKey is the values key prefix for external config (e.g., "externalDatabase").
	ExternalValuesKey string
	// DetectedFields lists the specific external value fields found (e.g., "host", "port").
	DetectedFields []string
}

// DetectExternalPatterns inspects chart metadata and values for the common
// pattern of subchart + externalFoo values. Returns all detected patterns.
func DetectExternalPatterns(meta *chartmeta.ChartMeta, values map[string]interface{}) []DetectedPattern {
	var detected []DetectedPattern

	for _, dep := range meta.Dependencies {
		// Check if this dependency has a condition like "postgresql.enabled".
		if dep.Condition == "" {
			continue
		}

		// Look for known external patterns for this subchart name.
		candidates, known := knownExternalPatterns[strings.ToLower(dep.Name)]
		if !known {
			// Try the generic pattern: "external" + PascalCase(name).
			candidates = []string{"external" + pascalCase(dep.Name)}
		}

		for _, candidate := range candidates {
			if extVals, ok := values[candidate]; ok {
				extMap, isMap := extVals.(map[string]interface{})
				if !isMap {
					continue
				}

				fields := make([]string, 0, len(extMap))
				for k := range extMap {
					fields = append(fields, k)
				}

				detected = append(detected, DetectedPattern{
					SubchartName:      dep.Name,
					Condition:         dep.Condition,
					ExternalValuesKey: candidate,
					DetectedFields:    fields,
				})

				break // use first matching candidate
			}
		}
	}

	return detected
}

// DetectExternalPatternsForSubchart detects the pattern for a specific subchart name.
// Returns the pattern and true if found, or zero value and false otherwise.
func DetectExternalPatternsForSubchart(
	meta *chartmeta.ChartMeta,
	values map[string]interface{},
	subchartName string,
) (DetectedPattern, bool) {
	patterns := DetectExternalPatterns(meta, values)
	for _, p := range patterns {
		if strings.EqualFold(p.SubchartName, subchartName) {
			return p, true
		}
	}

	return DetectedPattern{}, false
}

// BuildFiltersFromPattern creates exclusion and externalization filters
// from a detected pattern. It excludes the subchart and generates external
// mappings for Secret and Service resources that the subchart typically creates.
func BuildFiltersFromPattern(pattern DetectedPattern) []Filter {
	filters := []Filter{
		NewSubchartFilter([]string{pattern.SubchartName}),
	}

	// Generate external mappings based on detected fields.
	var mappings []ExternalMapping

	// Common mapping: if the external values have "host" or "secretName",
	// create appropriate externalizations.
	for _, field := range pattern.DetectedFields {
		switch {
		case strings.EqualFold(field, "secretName") || strings.EqualFold(field, "existingSecret"):
			mappings = append(mappings, ExternalMapping{
				ResourceName: pattern.SubchartName,
				ResourceKind: "Secret",
				SchemaField:  fmt.Sprintf("%s.%s", pattern.ExternalValuesKey, field),
			})
		}
	}

	if len(mappings) > 0 {
		filters = append(filters, NewExternalRefFilter(mappings))
	}

	return filters
}

// pascalCase converts a string to PascalCase (simple: capitalize first letter).
func pascalCase(s string) string {
	if s == "" {
		return ""
	}

	return strings.ToUpper(s[:1]) + s[1:]
}
