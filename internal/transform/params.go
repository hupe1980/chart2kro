// Package transform - params.go defines parameter mapping types and CEL
// expression generation for sentinel-based parameter detection.
package transform

import (
	"fmt"
	"strings"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// FieldMapping represents a mapping from a Helm values path to affected resource fields.
type FieldMapping struct {
	// ValuesPath is the dot-separated Helm values path (e.g., "image.tag").
	ValuesPath string

	// ResourceID is the ID of the affected resource.
	ResourceID string

	// FieldPath is the dot-separated field path within the resource
	// (e.g., "spec.template.spec.containers[0].image").
	FieldPath string

	// MatchType indicates how the sentinel was found.
	MatchType MatchType

	// SentinelRendered holds the sentinel-rendered field value for substring matches.
	// Used by BuildCELExpression to produce interpolated CEL expressions.
	SentinelRendered string
}

// MatchType indicates how a sentinel value was matched.
type MatchType int

const (
	// MatchExact means the entire field value was the sentinel.
	MatchExact MatchType = iota

	// MatchSubstring means the sentinel appeared as part of the field value.
	// This indicates string interpolation.
	MatchSubstring
)

// String returns the string representation of a MatchType.
func (m MatchType) String() string {
	switch m {
	case MatchExact:
		return "exact"
	case MatchSubstring:
		return "substring"
	default:
		return "unknown"
	}
}

// BuildCELExpression generates a CEL expression for a field mapping.
// For exact matches: ${schema.spec.<path>}
// For substring matches: "${schema.spec.<path1>}...<literal>...${schema.spec.<path2>}"
func BuildCELExpression(mapping FieldMapping, sentinelRendered string) string {
	if mapping.MatchType == MatchExact {
		return SchemaRef("spec", mapping.ValuesPath)
	}

	// Substring match — if we have the sentinel-rendered string, parse it.
	if sentinelRendered != "" {
		return BuildInterpolatedCELFromSentinel(sentinelRendered)
	}

	// Fallback — produce a simple reference.
	return fmt.Sprintf("${schema.spec.%s}", mapping.ValuesPath)
}

// BuildInterpolatedCELFromSentinel parses a sentinel-rendered string with
// one or more sentinels embedded, and produces a CEL interpolation expression.
func BuildInterpolatedCELFromSentinel(sentinelRendered string) string {
	var parts []string

	remaining := sentinelRendered
	for {
		start := strings.Index(remaining, SentinelPrefix)
		if start < 0 {
			if remaining != "" {
				parts = append(parts, remaining)
			}

			break
		}

		// Add literal prefix.
		if start > 0 {
			parts = append(parts, remaining[:start])
		}

		after := remaining[start+len(SentinelPrefix):]
		end := strings.Index(after, SentinelSuffix)

		if end < 0 {
			// Malformed sentinel — keep as-is.
			parts = append(parts, remaining)

			break
		}

		path := after[:end]
		parts = append(parts, SchemaRef("spec", path))

		remaining = after[end+len(SentinelSuffix):]
	}

	return strings.Join(parts, "")
}

// ApplyFieldMappings applies field mappings to resource templates, replacing
// hardcoded values with CEL expressions (e.g., ${schema.spec.replicaCount}).
// For substring matches (string interpolation), it uses the full sentinel-rendered
// value to produce a composite CEL expression.
func ApplyFieldMappings(
	resources []*k8s.Resource,
	resourceIDs map[*k8s.Resource]string,
	mappings []FieldMapping,
) {
	// Index mappings by resourceID for efficient lookup.
	byResource := make(map[string][]FieldMapping)
	for _, m := range mappings {
		byResource[m.ResourceID] = append(byResource[m.ResourceID], m)
	}

	for _, r := range resources {
		if r.Object == nil {
			continue
		}

		id := resourceIDs[r]
		rMappings := byResource[id]

		if len(rMappings) == 0 {
			continue
		}

		// Deduplicate by field path — for substring matches, multiple mappings
		// point to the same field. We only need to apply the CEL expression once.
		applied := make(map[string]bool)

		for _, m := range rMappings {
			if applied[m.FieldPath] {
				continue
			}

			applied[m.FieldPath] = true

			var expr string
			if m.MatchType == MatchExact {
				expr = BuildCELExpression(m, "")
			} else {
				// Substring match — use the sentinel-rendered value to build
				// the full interpolated CEL expression.
				expr = BuildInterpolatedCELFromSentinel(m.SentinelRendered)
			}

			setNestedField(r.Object.Object, m.FieldPath, expr)
		}
	}
}
