package filter

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// ExternalMapping describes how an excluded resource should be replaced
// with field references in the generated RGD schema.
type ExternalMapping struct {
	// ResourceName is the metadata.name of the resource to externalize.
	ResourceName string
	// ResourceKind is the kind of resource (e.g., "Secret", "Service").
	ResourceKind string
	// SchemaField is the dot-separated schema path (e.g., "externalDatabase.secretName").
	SchemaField string
}

// ParseExternalMapping parses "name=schemaField" into an ExternalMapping with
// the given kind.
func ParseExternalMapping(kind, expr string) (ExternalMapping, error) {
	parts := strings.SplitN(expr, "=", 2) //nolint:mnd // splitting on '='
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ExternalMapping{}, fmt.Errorf("invalid external mapping %q: expected name=schemaField", expr)
	}

	return ExternalMapping{
		ResourceName: parts[0],
		ResourceKind: kind,
		SchemaField:  parts[1],
	}, nil
}

// ExternalRefFilter excludes resources and promotes them to external references.
// It records the schema fields needed for the externalized resources and
// rewires references in the remaining resources.
type ExternalRefFilter struct {
	mappings []ExternalMapping
}

// NewExternalRefFilter creates a filter that externalizes the named resources.
func NewExternalRefFilter(mappings []ExternalMapping) *ExternalRefFilter {
	return &ExternalRefFilter{mappings: mappings}
}

// Apply finds resources matching the mappings, excludes them, and records
// externalization metadata including schema additions and rewirings.
func (f *ExternalRefFilter) Apply(_ context.Context, resources []*k8s.Resource) (*Result, error) {
	r := NewResult()

	// Index mappings by (kind, name) for lookup.
	type mappingKey struct{ kind, name string }

	byKey := make(map[mappingKey]ExternalMapping, len(f.mappings))
	for _, m := range f.mappings {
		byKey[mappingKey{
			kind: strings.ToLower(m.ResourceKind),
			name: m.ResourceName,
		}] = m
	}

	// Partition resources.
	for _, res := range resources {
		key := mappingKey{
			kind: strings.ToLower(res.Kind()),
			name: res.Name,
		}

		if m, ok := byKey[key]; ok {
			ext := buildExternalizedResource(res, m)
			r.Externalized = append(r.Externalized, ext)

			// Add schema fields.
			for k, v := range ext.SchemaFields {
				r.SchemaAdditions[k] = v
			}
		} else {
			r.Included = append(r.Included, res)
		}
	}

	// Rewire references in remaining resources.
	if len(r.Externalized) > 0 {
		rewireResources(r.Included, r.Externalized)
	}

	return r, nil
}

// buildExternalizedResource creates the externalization metadata for a resource.
func buildExternalizedResource(res *k8s.Resource, m ExternalMapping) ExternalizedResource {
	schemaFields := make(map[string]string)
	rewirings := make(map[string]string)

	celRef := fmt.Sprintf("${schema.spec.%s}", m.SchemaField)

	// The primary schema field holds the resource name.
	schemaFields[m.SchemaField] = "string"

	// Add the old→new value rewiring.
	rewirings[res.Name] = celRef

	// Build the externalRef template.
	externalRef := map[string]interface{}{
		"apiVersion": res.APIVersion(),
		"kind":       res.Kind(),
		"metadata": map[string]interface{}{
			"name": celRef,
		},
	}

	return ExternalizedResource{
		Resource:     res,
		ExternalRef:  externalRef,
		SchemaFields: schemaFields,
		Rewirings:    rewirings,
	}
}

// rewireResources walks the remaining resources and replaces string values
// that match externalized resource names with their CEL expression replacements.
func rewireResources(resources []*k8s.Resource, externalized []ExternalizedResource) {
	// Build a combined rewiring map from all externalized resources.
	rewirings := make(map[string]string)
	for _, ext := range externalized {
		for old, repl := range ext.Rewirings {
			rewirings[old] = repl
		}
	}

	if len(rewirings) == 0 {
		return
	}

	for _, res := range resources {
		if res.Object == nil {
			continue
		}

		res.Object.Object = rewireMap(res.Object.Object, rewirings).(map[string]interface{})
	}
}

// rewireMap recursively walks a map/slice tree and replaces matching string values.
func rewireMap(val interface{}, rewirings map[string]string) interface{} {
	switch v := val.(type) {
	case map[string]interface{}:
		for key, child := range v {
			v[key] = rewireMap(child, rewirings)
		}

		return v
	case []interface{}:
		for i, child := range v {
			v[i] = rewireMap(child, rewirings)
		}

		return v
	case string:
		if repl, ok := rewirings[v]; ok {
			return repl
		}

		// Handle substring matches at segment boundaries only (e.g.,
		// "postgresql.default.svc.cluster.local" contains the segment
		// "postgresql" delimited by "."). This avoids false rewirings
		// where short resource names match inside unrelated strings
		// (e.g., "redis" inside "redis-commander").
		for old, repl := range rewirings {
			v = replaceAtSegmentBoundaries(v, old, repl)
		}

		return v
	default:
		return val
	}
}

// segmentDelimiters defines the characters that act as segment boundaries
// for substring replacement. Resource names in Kubernetes manifests are
// typically delimited by these characters in DNS names, paths, and URIs.
const segmentDelimiters = "./:@"

// isSegmentDelimiter returns true if the byte is a segment boundary character.
func isSegmentDelimiter(b byte) bool {
	return strings.ContainsRune(segmentDelimiters, rune(b))
}

// replaceAtSegmentBoundaries replaces occurrences of old with repl in s,
// but only when the match is bounded by segment delimiters (., /, :, @)
// or the start/end of the string. This prevents greedy replacement of
// short resource names inside unrelated compound strings.
func replaceAtSegmentBoundaries(s, old, repl string) string {
	if !strings.Contains(s, old) {
		return s
	}

	var result strings.Builder

	result.Grow(len(s))

	idx := 0

	for idx < len(s) {
		pos := strings.Index(s[idx:], old)
		if pos == -1 {
			result.WriteString(s[idx:])
			break
		}

		absPos := idx + pos
		endPos := absPos + len(old)

		leftOK := absPos == 0 || isSegmentDelimiter(s[absPos-1])
		rightOK := endPos == len(s) || isSegmentDelimiter(s[endPos])

		if leftOK && rightOK {
			result.WriteString(s[idx:absPos])
			result.WriteString(repl)
			idx = endPos
		} else {
			result.WriteString(s[idx:endPos])
			idx = endPos
		}
	}

	return result.String()
}
