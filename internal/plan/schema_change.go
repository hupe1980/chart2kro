package plan

import (
	"fmt"
	"sort"
	"strings"
)

// ChangeType represents the type of change detected.
type ChangeType string

const (
	ChangeAdded    ChangeType = "added"
	ChangeRemoved  ChangeType = "removed"
	ChangeModified ChangeType = "modified"
)

// SchemaChange represents a single schema field change between two RGD versions.
type SchemaChange struct {
	Type     ChangeType `json:"type"`
	Field    string     `json:"field"`
	Details  string     `json:"details"`
	Impact   string     `json:"impact,omitempty"`
	Breaking bool       `json:"breaking"`
}

// CompareSchemas compares the spec.schema.spec sections of two RGD maps and returns
// a list of schema changes, sorted with breaking changes first.
func CompareSchemas(oldSpec, newSpec map[string]interface{}) []SchemaChange {
	if oldSpec == nil {
		oldSpec = map[string]interface{}{}
	}

	if newSpec == nil {
		newSpec = map[string]interface{}{}
	}

	changes := compareSchemaFields("", oldSpec, newSpec)

	// Sort: breaking first, then by field name.
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Breaking != changes[j].Breaking {
			return changes[i].Breaking
		}

		return changes[i].Field < changes[j].Field
	})

	return changes
}

// compareSchemaFields recursively compares old and new schema field maps.
func compareSchemaFields(prefix string, old, new map[string]interface{}) []SchemaChange {
	var changes []SchemaChange

	// Check removed fields.
	for _, key := range sortedStringKeys(old) {
		fieldPath := key
		if prefix != "" {
			fieldPath = prefix + "." + key
		}

		if _, exists := new[key]; !exists {
			oldVal := old[key]
			if isSchemaGroup(oldVal) {
				changes = append(changes, SchemaChange{
					Type:     ChangeRemoved,
					Field:    fieldPath,
					Details:  "schema field group removed",
					Impact:   "Existing instances using this field group will fail validation",
					Breaking: true,
				})
			} else {
				changes = append(changes, SchemaChange{
					Type:     ChangeRemoved,
					Field:    fieldPath,
					Details:  fmt.Sprintf("field removed (was: %v)", oldVal),
					Impact:   "Existing instances using this field will fail validation",
					Breaking: true,
				})
			}
		}
	}

	// Check added and modified fields.
	for _, key := range sortedStringKeys(new) {
		fieldPath := key
		if prefix != "" {
			fieldPath = prefix + "." + key
		}

		newVal := new[key]
		oldVal, exists := old[key]

		if !exists {
			changes = append(changes, classifyAddedField(fieldPath, newVal)...)

			continue
		}

		// Both exist — compare.
		changes = append(changes, compareFieldValues(fieldPath, oldVal, newVal)...)
	}

	return changes
}

// classifyAddedField classifies an added field as breaking or non-breaking.
func classifyAddedField(fieldPath string, val interface{}) []SchemaChange {
	if isSchemaGroup(val) {
		groupMap, _ := val.(map[string]interface{})

		return []SchemaChange{{
			Type:     ChangeAdded,
			Field:    fieldPath,
			Details:  fmt.Sprintf("field group added with %d sub-fields", len(groupMap)),
			Breaking: false,
		}}
	}

	spec := fmt.Sprintf("%v", val)
	if hasDefault(spec) {
		return []SchemaChange{{
			Type:     ChangeAdded,
			Field:    fieldPath,
			Details:  fmt.Sprintf("field added: %s", spec),
			Breaking: false,
		}}
	}

	return []SchemaChange{{
		Type:     ChangeAdded,
		Field:    fieldPath,
		Details:  fmt.Sprintf("required field added: %s (no default)", spec),
		Impact:   "Existing instances will fail validation without this field",
		Breaking: true,
	}}
}

// compareFieldValues compares two field values and returns any changes.
func compareFieldValues(fieldPath string, oldVal, newVal interface{}) []SchemaChange {
	// Case 1: both are schema groups (nested objects).
	oldGroup, oldIsGroup := oldVal.(map[string]interface{})
	newGroup, newIsGroup := newVal.(map[string]interface{})

	if oldIsGroup && newIsGroup {
		return compareSchemaFields(fieldPath, oldGroup, newGroup)
	}

	// Case 2: type mismatch (scalar <-> object).
	if oldIsGroup != newIsGroup {
		return []SchemaChange{{
			Type:     ChangeModified,
			Field:    fieldPath,
			Details:  fmt.Sprintf("changed from %s to %s", describeKind(oldIsGroup), describeKind(newIsGroup)),
			Impact:   "Existing instances using this field will fail validation",
			Breaking: true,
		}}
	}

	// Case 3: both scalars — compare as strings.
	oldStr := fmt.Sprintf("%v", oldVal)
	newStr := fmt.Sprintf("%v", newVal)

	if oldStr == newStr {
		return nil
	}

	oldType := parseSchemaType(oldStr)
	newType := parseSchemaType(newStr)

	if oldType != newType {
		return []SchemaChange{{
			Type:     ChangeModified,
			Field:    fieldPath,
			Details:  fmt.Sprintf("type changed: %s -> %s", oldType, newType),
			Impact:   "Existing instances may fail validation with new type",
			Breaking: true,
		}}
	}

	// Same type but different spec (e.g., default changed).
	oldDefault := parseSchemaDefault(oldStr)
	newDefault := parseSchemaDefault(newStr)

	if oldDefault != newDefault {
		return []SchemaChange{{
			Type:     ChangeModified,
			Field:    fieldPath,
			Details:  fmt.Sprintf("default changed: %s -> %s", oldDefault, newDefault),
			Breaking: false,
		}}
	}

	return []SchemaChange{{
		Type:     ChangeModified,
		Field:    fieldPath,
		Details:  fmt.Sprintf("spec changed: %s -> %s", oldStr, newStr),
		Breaking: false,
	}}
}

// describeKind returns a human-readable kind description.
func describeKind(isGroup bool) string {
	if isGroup {
		return "object"
	}

	return "scalar"
}

// isSchemaGroup checks if a value represents a nested schema group (map).
func isSchemaGroup(val interface{}) bool {
	_, ok := val.(map[string]interface{})
	return ok
}

// parseSchemaType extracts the type portion from a schema spec like "string | default=\"foo\"".
func parseSchemaType(spec string) string {
	spec = strings.TrimSpace(spec)
	if idx := strings.Index(spec, "|"); idx >= 0 {
		return strings.TrimSpace(spec[:idx])
	}

	return spec
}

// parseSchemaDefault extracts the default value from a schema spec.
func parseSchemaDefault(spec string) string {
	spec = strings.TrimSpace(spec)
	if idx := strings.Index(spec, "default="); idx >= 0 {
		return strings.TrimSpace(spec[idx+len("default="):])
	}

	return ""
}

// hasDefault checks whether a schema spec contains a default value.
func hasDefault(spec string) bool {
	return strings.Contains(spec, "default=")
}

// sortedStringKeys returns the keys of a string-keyed map in sorted order.
func sortedStringKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}
