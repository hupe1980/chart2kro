// Package transform - fieldpath.go provides utilities for navigating and
// mutating nested map[string]interface{} structures using dot-separated field
// paths with optional array index notation (e.g., "spec.containers[0].image").
package transform

import "strings"

// pathPart represents a single component of a field path.
type pathPart struct {
	Key   string
	Index int // -1 means not an array index
}

// parseFieldPath parses a dot-separated field path with optional array indices.
// e.g., "spec.containers[0].image" => [{Key:"spec", Index:-1}, {Key:"containers", Index:0}, {Key:"image", Index:-1}]
func parseFieldPath(path string) []pathPart {
	if path == "" {
		return nil
	}

	segments := strings.Split(path, ".")
	var parts []pathPart

	for _, seg := range segments {
		if seg == "" {
			continue // skip empty segments from leading/trailing/double dots
		}

		bracketIdx := strings.Index(seg, "[")
		if bracketIdx < 0 {
			parts = append(parts, pathPart{Key: seg, Index: -1})
			continue
		}

		closeBracket := strings.Index(seg, "]")
		if closeBracket < 0 || closeBracket <= bracketIdx+1 {
			// Malformed bracket expression — treat entire segment as key.
			parts = append(parts, pathPart{Key: seg, Index: -1})
			continue
		}

		key := seg[:bracketIdx]
		idxStr := seg[bracketIdx+1 : closeBracket]

		idx := 0
		validIndex := true

		for _, ch := range idxStr {
			if ch < '0' || ch > '9' {
				validIndex = false
				break
			}

			idx = idx*10 + int(ch-'0')
		}

		if !validIndex {
			// Non-numeric index — treat entire segment as key.
			parts = append(parts, pathPart{Key: seg, Index: -1})
			continue
		}

		// First add the key as a map access, then the index as an array access.
		if key != "" {
			parts = append(parts, pathPart{Key: key, Index: -1})
		}

		parts = append(parts, pathPart{Key: "", Index: idx})
	}

	return parts
}

// setNestedField sets a value at a dot-separated path in a nested map.
// Supports array index notation like "spec.containers[0].image".
func setNestedField(obj map[string]interface{}, path string, value interface{}) {
	parts := parseFieldPath(path)
	if len(parts) == 0 {
		return
	}

	current := interface{}(obj)

	for i := 0; i < len(parts)-1; i++ {
		p := parts[i]

		switch c := current.(type) {
		case map[string]interface{}:
			if p.Index >= 0 {
				continue // invalid: index on a map
			}

			next, ok := c[p.Key]
			if !ok {
				return // path doesn't exist
			}

			current = next
		case []interface{}:
			if p.Index < 0 || p.Index >= len(c) {
				return // index out of range
			}

			current = c[p.Index]
		default:
			return // can't traverse further
		}
	}

	last := parts[len(parts)-1]

	switch c := current.(type) {
	case map[string]interface{}:
		if last.Index >= 0 {
			return // invalid: index at leaf on a map
		}

		c[last.Key] = value
	case []interface{}:
		if last.Index >= 0 && last.Index < len(c) {
			c[last.Index] = value
		}
	}
}

// joinFieldPath joins a prefix and key with a dot separator for field paths.
func joinFieldPath(prefix, key string) string {
	if prefix == "" {
		return key
	}

	return prefix + "." + key
}
