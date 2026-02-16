// Package transform implements the core transformation pipeline that converts
// parsed Kubernetes resources and Helm values into KRO-compatible artifacts.
package transform

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// idSanitizer matches characters that are not alphanumeric or hyphens.
var idSanitizer = regexp.MustCompile(`[^a-z0-9-]`)

// AssignResourceIDs assigns stable, human-readable IDs to each resource.
// When multiple resources share the same kind, a disambiguating name segment
// is appended (e.g., "service-main", "service-headless").
// The overrides map allows manual ID assignment by qualified name ("Kind/name").
// Returns an error if two resources would receive the same ID after sanitization.
func AssignResourceIDs(resources []*k8s.Resource, overrides map[string]string) (map[*k8s.Resource]string, error) {
	ids := make(map[*k8s.Resource]string, len(resources))

	// Apply overrides first.
	overridden := make(map[*k8s.Resource]bool)

	for _, r := range resources {
		if id, ok := overrides[r.QualifiedName()]; ok {
			ids[r] = sanitizeID(id)
			overridden[r] = true
		}
	}

	// Group non-overridden resources by lowercase kind for deduplication.
	kindGroups := make(map[string][]*k8s.Resource)

	for _, r := range resources {
		if overridden[r] {
			continue
		}

		kind := strings.ToLower(r.GVK.Kind)
		kindGroups[kind] = append(kindGroups[kind], r)
	}

	// Assign IDs.
	for kind, group := range kindGroups {
		if len(group) == 1 {
			ids[group[0]] = sanitizeID(kind)
			continue
		}

		// Multiple resources of the same kind: append name segments.
		// Start with the last segment; if there are collisions, progressively
		// include more segments from the right until all IDs are unique.
		suffixes := disambiguateSuffixes(group)
		for i, r := range group {
			ids[r] = sanitizeID(kind + "-" + suffixes[i])
		}
	}

	// Check for collisions after sanitization.
	seen := make(map[string]*k8s.Resource, len(ids))

	for r, id := range ids {
		if existing, ok := seen[id]; ok {
			return nil, fmt.Errorf(
				"resource ID collision: %q and %q both resolve to ID %q",
				existing.QualifiedName(), r.QualifiedName(), id,
			)
		}

		seen[id] = r
	}

	return ids, nil
}

// sanitizeID lowercases and replaces invalid characters with hyphens,
// collapsing consecutive hyphens and trimming leading/trailing ones.
func sanitizeID(s string) string {
	s = strings.ToLower(s)
	s = idSanitizer.ReplaceAllString(s, "-")

	// Collapse consecutive hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}

	return strings.Trim(s, "-")
}

// disambiguateSuffixes computes the shortest unique suffix for each resource
// in a group that share the same Kind. It starts with the last hyphen-separated
// segment of each name and progressively includes more segments from the right
// until every suffix in the group is unique.
func disambiguateSuffixes(group []*k8s.Resource) []string {
	// Split each name into parts.
	allParts := make([][]string, len(group))
	for i, r := range group {
		allParts[i] = strings.Split(r.Name, "-")
	}

	// Start with 1 segment from the right, increase until unique.
	maxParts := 0
	for _, parts := range allParts {
		if len(parts) > maxParts {
			maxParts = len(parts)
		}
	}

	for depth := 1; depth <= maxParts; depth++ {
		suffixes := make([]string, len(group))
		for i, parts := range allParts {
			start := len(parts) - depth
			if start < 0 {
				start = 0
			}

			suffixes[i] = strings.Join(parts[start:], "-")
		}

		if allUnique(suffixes) {
			return suffixes
		}
	}

	// Fallback: use the full name (should always be unique).
	suffixes := make([]string, len(group))
	for i, r := range group {
		suffixes[i] = r.Name
	}

	return suffixes
}

// allUnique returns true if all strings in the slice are distinct.
func allUnique(ss []string) bool {
	seen := make(map[string]bool, len(ss))
	for _, s := range ss {
		if seen[s] {
			return false
		}

		seen[s] = true
	}

	return true
}

// ToCamelCase converts a dot-separated or hyphenated path to camelCase.
func ToCamelCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})

	if len(parts) == 0 {
		return s
	}

	var b strings.Builder

	for i, part := range parts {
		if i == 0 {
			b.WriteString(strings.ToLower(part))
		} else {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			b.WriteString(string(runes))
		}
	}

	return b.String()
}

// ToPascalCase converts a string to PascalCase.
func ToPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '-' || r == '_' || r == ' '
	})

	var b strings.Builder

	for _, part := range parts {
		if len(part) > 0 {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			b.WriteString(string(runes))
		}
	}

	return b.String()
}
