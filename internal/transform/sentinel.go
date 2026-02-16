// Package transform - sentinel.go implements sentinel value injection and diffing
// for parameter detection. It replaces Helm values with unique markers, re-renders,
// and diffs resources to determine which fields are controlled by which values.
package transform

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// SentinelPrefix is the prefix used for string sentinel values.
const SentinelPrefix = "__CHART2KRO_SENTINEL_"

// SentinelSuffix is the suffix used for string sentinel values.
const SentinelSuffix = "__"

// SentinelForString returns the sentinel string for a given path.
func SentinelForString(path string) string {
	return SentinelPrefix + path + SentinelSuffix
}

// ExtractSentinelsFromString extracts all sentinel paths from a string that
// may contain multiple interpolated sentinels.
func ExtractSentinelsFromString(s string) []string {
	var paths []string

	remaining := s
	for {
		start := strings.Index(remaining, SentinelPrefix)
		if start < 0 {
			break
		}

		after := remaining[start+len(SentinelPrefix):]
		end := strings.Index(after, SentinelSuffix)

		if end < 0 {
			break
		}

		path := after[:end]
		paths = append(paths, path)

		remaining = after[end+len(SentinelSuffix):]
	}

	return paths
}

// SentinelizeAll creates a deep copy of values with ALL leaf values replaced
// by their sentinel equivalents. This enables detecting multi-value string
// interpolation patterns (e.g., "${image.repo}:${image.tag}") in a single render.
func SentinelizeAll(values map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(values))
	sentinelizeAllRecursive(values, "", result)

	return result
}

func sentinelizeAllRecursive(values map[string]interface{}, prefix string, result map[string]interface{}) {
	for key, val := range values {
		path := joinFieldPath(prefix, key)

		switch v := val.(type) {
		case map[string]interface{}:
			nested := make(map[string]interface{}, len(v))
			sentinelizeAllRecursive(v, path, nested)
			result[key] = nested
		case []interface{}:
			result[key] = val // preserve arrays as-is
		default:
			// Use string sentinels for ALL leaf types (string, int, float, bool).
			// This ensures extractSentinelMappings can always extract the path
			// from the sentinel marker. Helm templates generally handle string
			// values in place of int/bool for simple interpolation.
			result[key] = SentinelForString(path)
		}
	}
}

// DiffAllResources compares baseline resources against full-sentinel-rendered
// resources to detect all field mappings in a single pass. Resources are matched
// by GVK+name identity rather than positional index, making the diff robust
// against structural changes caused by sentinel values (e.g., conditional blocks).
func DiffAllResources(
	baseline []*k8s.Resource,
	sentinelRendered []*k8s.Resource,
	resourceIDs map[*k8s.Resource]string,
) []FieldMapping {
	if len(sentinelRendered) == 0 {
		return nil
	}

	// Index sentinel-rendered resources by GVK+name for O(1) lookup.
	sentinelByKey := make(map[string]*k8s.Resource, len(sentinelRendered))
	for _, r := range sentinelRendered {
		key := resourceMatchKey(r)
		if key != "" {
			sentinelByKey[key] = r
		}
	}

	var mappings []FieldMapping

	for _, baseRes := range baseline {
		if baseRes.Object == nil {
			continue
		}

		key := resourceMatchKey(baseRes)
		sentRes, ok := sentinelByKey[key]

		if !ok || sentRes.Object == nil {
			continue
		}

		id := resourceIDs[baseRes]
		resourceMappings := diffForSentinels(
			baseRes.Object.Object,
			sentRes.Object.Object,
			id, "",
		)

		mappings = append(mappings, resourceMappings...)
	}

	return mappings
}

// resourceMatchKey returns a stable identity key for a resource based on
// its apiVersion, kind, and name. This is used to match baseline resources
// against sentinel-rendered resources regardless of array position.
// Returns "" for nil resources or resources with empty Kind (zero-valued GVK).
func resourceMatchKey(r *k8s.Resource) string {
	if r == nil || r.GVK.Kind == "" {
		return ""
	}

	apiVersion := GVKToAPIVersion(r.GVK)

	return apiVersion + "/" + r.GVK.Kind + "/" + r.Name
}

// diffForSentinels recursively diffs two maps and produces FieldMappings
// by extracting sentinel markers from changed values.
func diffForSentinels(base, sent map[string]interface{}, resourceID, prefix string) []FieldMapping {
	var mappings []FieldMapping

	for key, sentVal := range sent {
		fieldPath := joinFieldPath(prefix, key)
		baseVal, exists := base[key]

		if !exists {
			// New field from sentinel — extract sentinel info.
			mappings = append(mappings, extractSentinelMappings(sentVal, resourceID, fieldPath)...)
			continue
		}

		switch sv := sentVal.(type) {
		case map[string]interface{}:
			if bv, ok := baseVal.(map[string]interface{}); ok {
				mappings = append(mappings, diffForSentinels(bv, sv, resourceID, fieldPath)...)
			} else {
				mappings = append(mappings, extractSentinelMappings(sentVal, resourceID, fieldPath)...)
			}
		case []interface{}:
			if bv, ok := baseVal.([]interface{}); ok {
				mappings = append(mappings, diffSlicesForSentinels(bv, sv, resourceID, fieldPath)...)
			} else {
				mappings = append(mappings, extractSentinelMappings(sentVal, resourceID, fieldPath)...)
			}
		default:
			if !reflect.DeepEqual(baseVal, sentVal) {
				mappings = append(mappings, extractSentinelMappings(sentVal, resourceID, fieldPath)...)
			}
		}
	}

	return mappings
}

func diffSlicesForSentinels(base, sent []interface{}, resourceID, prefix string) []FieldMapping {
	var mappings []FieldMapping

	for i := 0; i < len(sent); i++ {
		fieldPath := fmt.Sprintf("%s[%d]", prefix, i)

		if i >= len(base) {
			mappings = append(mappings, extractSentinelMappings(sent[i], resourceID, fieldPath)...)
			continue
		}

		switch sv := sent[i].(type) {
		case map[string]interface{}:
			if bv, ok := base[i].(map[string]interface{}); ok {
				mappings = append(mappings, diffForSentinels(bv, sv, resourceID, fieldPath)...)
			} else {
				mappings = append(mappings, extractSentinelMappings(sent[i], resourceID, fieldPath)...)
			}
		case []interface{}:
			if bv, ok := base[i].([]interface{}); ok {
				mappings = append(mappings, diffSlicesForSentinels(bv, sv, resourceID, fieldPath)...)
			} else {
				mappings = append(mappings, extractSentinelMappings(sent[i], resourceID, fieldPath)...)
			}
		default:
			if !reflect.DeepEqual(base[i], sent[i]) {
				mappings = append(mappings, extractSentinelMappings(sent[i], resourceID, fieldPath)...)
			}
		}
	}

	return mappings
}

// extractSentinelMappings examines a sentinel-rendered value and produces
// FieldMappings by finding embedded sentinel markers. Non-string values are
// stringified before inspection, since Helm templates may convert sentinel
// strings to other types (e.g., integer fields).
func extractSentinelMappings(val interface{}, resourceID, fieldPath string) []FieldMapping {
	s, ok := val.(string)
	if !ok {
		// Try stringifying non-string values — the template may have
		// converted a sentinel marker to an int/bool.
		if val == nil {
			return nil
		}

		s = fmt.Sprintf("%v", val)
		if !strings.Contains(s, SentinelPrefix) {
			return nil
		}
	}

	// Check for sentinel markers in the string.
	paths := ExtractSentinelsFromString(s)
	if len(paths) == 0 {
		return nil
	}

	if len(paths) == 1 && s == SentinelForString(paths[0]) {
		// Exact match — the entire value is a single sentinel.
		return []FieldMapping{{
			ValuesPath:       paths[0],
			ResourceID:       resourceID,
			FieldPath:        fieldPath,
			MatchType:        MatchExact,
			SentinelRendered: s,
		}}
	}

	// Substring/interpolation — the value contains sentinel(s) mixed with literals.
	var mappings []FieldMapping

	seen := make(map[string]bool)

	for _, path := range paths {
		if seen[path] {
			continue
		}

		seen[path] = true

		mappings = append(mappings, FieldMapping{
			ValuesPath:       path,
			ResourceID:       resourceID,
			FieldPath:        fieldPath,
			MatchType:        MatchSubstring,
			SentinelRendered: s,
		})
	}

	return mappings
}
