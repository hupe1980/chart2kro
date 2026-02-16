package transform

import "github.com/hupe1980/chart2kro/internal/k8s"

// PathPart is the exported version of pathPart for testing.
type PathPart = pathPart

// ParseFieldPath is the exported version of parseFieldPath for testing.
func ParseFieldPath(path string) []PathPart {
	return parseFieldPath(path)
}

// SetNestedField is the exported version of setNestedField for testing.
func SetNestedField(obj map[string]interface{}, path string, value interface{}) {
	setNestedField(obj, path, value)
}

// ResourceMatchKey is the exported version of resourceMatchKey for testing.
func ResourceMatchKey(r *k8s.Resource) string {
	return resourceMatchKey(r)
}

// ExtractSentinelMappings is the exported version of extractSentinelMappings for testing.
func ExtractSentinelMappings(val interface{}, resourceID, fieldPath string) []FieldMapping {
	return extractSentinelMappings(val, resourceID, fieldPath)
}
