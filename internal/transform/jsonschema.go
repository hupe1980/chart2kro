// Package transform - jsonschema.go provides utilities to extract type
// information from a Helm chart's values.schema.json, enriching the schema
// beyond what Go runtime types can infer from values.yaml alone.
package transform

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JSONSchemaResolver resolves type information from a parsed values.schema.json.
// The resolver walks the JSON Schema tree to find type, format, enum, and
// description metadata for a given dot-separated Helm values path.
type JSONSchemaResolver struct {
	root map[string]interface{}
}

// NewJSONSchemaResolver parses raw values.schema.json bytes and returns a
// resolver. Returns nil (no error) when schemaBytes is empty, allowing
// callers to skip JSON Schema enrichment without branching.
func NewJSONSchemaResolver(schemaBytes []byte) (*JSONSchemaResolver, error) {
	if len(schemaBytes) == 0 {
		return nil, nil //nolint:nilnil // nil resolver means "no JSON Schema"
	}

	var root map[string]interface{}
	if err := json.Unmarshal(schemaBytes, &root); err != nil {
		return nil, fmt.Errorf("parsing values.schema.json: %w", err)
	}

	return &JSONSchemaResolver{root: root}, nil
}

// JSONSchemaInfo holds type metadata extracted from a JSON Schema property.
type JSONSchemaInfo struct {
	// Type is the JSON Schema type (string, integer, number, boolean, array, object).
	Type string
	// Format is an optional format hint (e.g., "int64", "email", "uri").
	Format string
	// Description is the human-readable description from the schema.
	Description string
	// Enum lists allowed values, if specified.
	Enum []interface{}
	// Minimum is the minimum numeric value, if specified.
	Minimum *float64
	// Maximum is the maximum numeric value, if specified.
	Maximum *float64
}

// Resolve looks up a dot-separated Helm values path in the JSON Schema and
// returns the type info. Returns nil when the path is not found.
func (r *JSONSchemaResolver) Resolve(path string) *JSONSchemaInfo {
	if r == nil || r.root == nil {
		return nil
	}

	segments := strings.Split(path, ".")
	node := r.root

	for _, seg := range segments {
		props, ok := getMap(node, "properties")
		if !ok {
			return nil
		}

		propNode, ok := getMap(props, seg)
		if !ok {
			return nil
		}

		node = propNode
	}

	return extractSchemaInfo(node)
}

// extractSchemaInfo reads JSON Schema type metadata from a property node.
func extractSchemaInfo(node map[string]interface{}) *JSONSchemaInfo {
	info := &JSONSchemaInfo{}

	if typ, ok := node["type"].(string); ok {
		info.Type = typ
	}

	if format, ok := node["format"].(string); ok {
		info.Format = format
	}

	if desc, ok := node["description"].(string); ok {
		info.Description = desc
	}

	if enumVal, ok := node["enum"].([]interface{}); ok {
		info.Enum = enumVal
	}

	if minVal, ok := node["minimum"].(float64); ok {
		info.Minimum = &minVal
	}

	if maxVal, ok := node["maximum"].(float64); ok {
		info.Maximum = &maxVal
	}

	// Return nil if no useful info was extracted.
	if info.Type == "" && info.Format == "" && info.Description == "" && info.Enum == nil {
		return nil
	}

	return info
}

// MapToSimpleSchemaType converts a JSON Schema type string to a KRO
// SimpleSchema type string. JSON Schema "integer" maps to "integer",
// "number" to "number", "boolean" to "boolean", and everything else
// (including "string") to "string".
func MapToSimpleSchemaType(jsonSchemaType string) string {
	switch jsonSchemaType {
	case "integer":
		return "integer"
	case "number":
		return "number"
	case "boolean":
		return "boolean"
	case "array":
		return "array"
	case "object":
		return "object"
	default:
		return "string"
	}
}

// getMap retrieves a nested map from a parent map.
func getMap(parent map[string]interface{}, key string) (map[string]interface{}, bool) {
	val, ok := parent[key]
	if !ok {
		return nil, false
	}

	m, ok := val.(map[string]interface{})

	return m, ok
}
