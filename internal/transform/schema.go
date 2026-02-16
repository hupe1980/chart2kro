// Package transform - schema.go implements schema extraction from Helm values.
package transform

import (
	"fmt"
	"sort"
	"strings"
)

// SchemaField represents a single field in the KRO SimpleSchema.
type SchemaField struct {
	// Name is the field name (camelCase).
	Name string

	// Path is the dot-separated Helm values path (e.g., "image.repository").
	Path string

	// Type is the KRO SimpleSchema type (string, integer, number, boolean).
	Type string

	// Default is the default value as a string (e.g., "\"nginx\"", "3", "true").
	Default string

	// Children holds nested fields for object types.
	Children []*SchemaField
}

// SchemaOverride allows overriding the inferred type and default of a schema field.
type SchemaOverride struct {
	// Type overrides the inferred type (string, integer, number, boolean).
	Type string

	// Default overrides the default value.
	Default string
}

// SimpleSchemaString returns the KRO SimpleSchema representation.
// e.g., "string | default=\"nginx\"" or "integer | default=3".
func (f *SchemaField) SimpleSchemaString() string {
	if f.Default == "" {
		return f.Type
	}

	return fmt.Sprintf("%s | default=%s", f.Type, f.Default)
}

// IsObject returns true if this field has children (nested object).
func (f *SchemaField) IsObject() bool {
	return len(f.Children) > 0
}

// SchemaExtractor extracts KRO SimpleSchema from Helm values.
type SchemaExtractor struct {
	// IncludeAll includes all values even if not referenced in templates.
	IncludeAll bool

	// FlatMode uses flat camelCase field names instead of nested objects.
	FlatMode bool

	// JSONSchema is an optional resolver for values.schema.json type info.
	// When non-nil, it enriches inferred types with explicit JSON Schema types.
	JSONSchema *JSONSchemaResolver
}

// NewSchemaExtractor creates a SchemaExtractor with the given options.
func NewSchemaExtractor(includeAll, flatMode bool, jsonSchema *JSONSchemaResolver) *SchemaExtractor {
	return &SchemaExtractor{
		IncludeAll: includeAll,
		FlatMode:   flatMode,
		JSONSchema: jsonSchema,
	}
}

// Extract walks the values tree and produces a list of SchemaFields.
// referencedPaths is the set of Helm value paths actually used in templates.
// If nil or IncludeAll is set, all values are included.
func (e *SchemaExtractor) Extract(values map[string]interface{}, referencedPaths map[string]bool) []*SchemaField {
	if e.FlatMode {
		return e.extractFlat(values, "", referencedPaths)
	}

	return e.extractNested(values, "", referencedPaths)
}

// extractNested produces a nested schema tree.
func (e *SchemaExtractor) extractNested(values map[string]interface{}, prefix string, refs map[string]bool) []*SchemaField {
	var fields []*SchemaField

	keys := sortedKeys(values)

	for _, key := range keys {
		val := values[key]
		path := joinFieldPath(prefix, key)

		if !e.shouldInclude(path, refs) && !e.hasReferencedChildren(path, refs) {
			continue
		}

		switch v := val.(type) {
		case map[string]interface{}:
			children := e.extractNested(v, path, refs)
			if len(children) > 0 || e.shouldInclude(path, refs) {
				fields = append(fields, &SchemaField{
					Name:     key,
					Path:     path,
					Type:     "object",
					Children: children,
				})
			}
		default:
			typ, def := e.inferTypeEnriched(path, val)
			fields = append(fields, &SchemaField{
				Name:    key,
				Path:    path,
				Type:    typ,
				Default: def,
			})
		}
	}

	return fields
}

// extractFlat produces flat camelCase field names.
func (e *SchemaExtractor) extractFlat(values map[string]interface{}, prefix string, refs map[string]bool) []*SchemaField {
	var fields []*SchemaField

	keys := sortedKeys(values)

	for _, key := range keys {
		val := values[key]
		path := joinFieldPath(prefix, key)

		switch v := val.(type) {
		case map[string]interface{}:
			fields = append(fields, e.extractFlat(v, path, refs)...)
		default:
			if !e.shouldInclude(path, refs) {
				continue
			}

			typ, def := e.inferTypeEnriched(path, val)
			fields = append(fields, &SchemaField{
				Name:    ToCamelCase(path),
				Path:    path,
				Type:    typ,
				Default: def,
			})
		}
	}

	return fields
}

// shouldInclude returns true when the path should be included in the schema.
func (e *SchemaExtractor) shouldInclude(path string, refs map[string]bool) bool {
	if e.IncludeAll || refs == nil {
		return true
	}

	return refs[path]
}

// hasReferencedChildren checks whether any descendant of path is referenced.
func (e *SchemaExtractor) hasReferencedChildren(path string, refs map[string]bool) bool {
	if e.IncludeAll || refs == nil {
		return true
	}

	prefix := path + "."

	for ref := range refs {
		if strings.HasPrefix(ref, prefix) {
			return true
		}
	}

	return false
}

// inferTypeEnriched resolves the type from JSON Schema first, falling back
// to runtime inference. This allows values.schema.json to override the type
// inferred from values.yaml defaults (e.g., a string "3" that JSON Schema
// declares as integer).
func (e *SchemaExtractor) inferTypeEnriched(path string, val interface{}) (string, string) {
	if e.JSONSchema != nil {
		if info := e.JSONSchema.Resolve(path); info != nil && info.Type != "" {
			schemaType := MapToSimpleSchemaType(info.Type)
			_, def := inferType(val)

			return schemaType, def
		}
	}

	return inferType(val)
}

// inferType infers the KRO SimpleSchema type and default string from a Go value.
func inferType(val interface{}) (typ string, def string) {
	if val == nil {
		return "string", ""
	}

	switch v := val.(type) {
	case bool:
		return "boolean", fmt.Sprintf("%t", v)
	case int:
		return "integer", fmt.Sprintf("%d", v)
	case int64:
		return "integer", fmt.Sprintf("%d", v)
	case float64:
		// Check if it's actually an integer value.
		if v == float64(int64(v)) {
			return "integer", fmt.Sprintf("%d", int64(v))
		}

		return "number", fmt.Sprintf("%g", v)
	case string:
		if v == "" {
			return "string", ""
		}

		return "string", fmt.Sprintf("%q", v)
	case []interface{}:
		return "array", ""
	default:
		return "string", ""
	}
}

// sortedKeys returns the keys of a map in sorted order for deterministic output.
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// BuildSimpleSchema converts a list of SchemaFields into a map[string]interface{}
// suitable for YAML serialization, using KRO SimpleSchema syntax.
func BuildSimpleSchema(fields []*SchemaField) map[string]interface{} {
	result := make(map[string]interface{})

	for _, f := range fields {
		if f.IsObject() {
			result[f.Name] = BuildSimpleSchema(f.Children)
		} else {
			result[f.Name] = f.SimpleSchemaString()
		}
	}

	return result
}

// ApplySchemaOverrides mutates the extracted schema fields in-place,
// replacing inferred types and defaults with user-specified overrides.
func ApplySchemaOverrides(fields []*SchemaField, overrides map[string]SchemaOverride) {
	if len(overrides) == 0 {
		return
	}

	applySchemaOverridesRecursive(fields, overrides)
}

func applySchemaOverridesRecursive(fields []*SchemaField, overrides map[string]SchemaOverride) {
	for _, f := range fields {
		if override, ok := overrides[f.Path]; ok {
			if override.Type != "" {
				f.Type = override.Type
			}

			if override.Default != "" {
				f.Default = override.Default
			}
		}

		if len(f.Children) > 0 {
			applySchemaOverridesRecursive(f.Children, overrides)
		}
	}
}
