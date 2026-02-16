package config

import (
	"fmt"
	"regexp"
	"strconv"

	sigsyaml "sigs.k8s.io/yaml"
)

// TransformConfig holds declarative transformation overrides loaded
// from the config file (.chart2kro.yaml).
type TransformConfig struct {
	// Transformers are per-Kind transformation overrides.
	Transformers []TransformerOverride `json:"transformers,omitempty"`

	// SchemaOverrides override inferred schema field types.
	SchemaOverrides map[string]SchemaOverride `json:"schemaOverrides,omitempty"`

	// ResourceIDOverrides override assigned resource IDs.
	ResourceIDOverrides map[string]string `json:"resourceIdOverrides,omitempty"`
}

// TransformerOverride defines a config-driven transformer match + overrides.
type TransformerOverride struct {
	// Match selects which resources this override applies to.
	Match TransformerMatch `json:"match"`

	// ReadyWhen overrides readiness conditions.
	ReadyWhen []string `json:"readyWhen,omitempty"`

	// StatusFields overrides status projections.
	StatusFields []StatusFieldOverride `json:"statusFields,omitempty"`
}

// TransformerMatch identifies resources by Kind and optionally APIVersion.
type TransformerMatch struct {
	// Kind is the Kubernetes resource kind (e.g., "Deployment").
	Kind string `json:"kind"`

	// APIVersion is an optional API version filter (e.g., "apps/v1").
	APIVersion string `json:"apiVersion,omitempty"`
}

// StatusFieldOverride defines a status field projection.
type StatusFieldOverride struct {
	// Name is the status field name.
	Name string `json:"name"`

	// CELExpression is the CEL expression to extract the value.
	CELExpression string `json:"celExpression"`
}

// SchemaOverride allows overriding the inferred type of a schema field.
type SchemaOverride struct {
	// Type overrides the inferred type (string, integer, number, boolean).
	Type string `json:"type"`

	// Default overrides the default value.
	Default string `json:"default,omitempty"`
}

// ParseTransformConfig parses the transformers, schemaOverrides, and
// resourceIdOverrides sections from raw config file bytes.
func ParseTransformConfig(data []byte) (*TransformConfig, error) {
	// Parse the raw YAML to extract transform-related sections.
	var raw struct {
		Transformers        []TransformerOverride     `json:"transformers,omitempty"`
		SchemaOverrides     map[string]SchemaOverride `json:"schemaOverrides,omitempty"`
		ResourceIDOverrides map[string]string         `json:"resourceIdOverrides,omitempty"`
	}

	if err := sigsyaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing transform config: %w", err)
	}

	cfg := &TransformConfig{
		Transformers:        raw.Transformers,
		SchemaOverrides:     raw.SchemaOverrides,
		ResourceIDOverrides: raw.ResourceIDOverrides,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// resourceIDPattern validates resource ID override values.
// Must start with a letter and contain only letters, digits, and hyphens.
var resourceIDPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]*$`)

// Validate checks the transform config for correctness.
func (c *TransformConfig) Validate() error {
	for i, t := range c.Transformers {
		if t.Match.Kind == "" {
			return fmt.Errorf("transformer[%d]: match.kind is required", i)
		}
	}

	validTypes := map[string]bool{
		"string": true, "integer": true, "number": true, "boolean": true,
	}

	for field, override := range c.SchemaOverrides {
		if override.Type != "" && !validTypes[override.Type] {
			return fmt.Errorf("schemaOverrides[%s]: invalid type %q (must be string, integer, number, or boolean)", field, override.Type)
		}

		if override.Default != "" && override.Type != "" {
			if err := validateDefaultForType(override.Type, override.Default); err != nil {
				return fmt.Errorf("schemaOverrides[%s]: %w", field, err)
			}
		}
	}

	for key, val := range c.ResourceIDOverrides {
		if val == "" {
			return fmt.Errorf("resourceIdOverrides[%s]: value must not be empty", key)
		}

		if !resourceIDPattern.MatchString(val) {
			return fmt.Errorf("resourceIdOverrides[%s]: value %q is invalid (must match %s)", key, val, resourceIDPattern.String())
		}
	}

	return nil
}

// validateDefaultForType checks that a default value string is compatible
// with the declared schema type.
func validateDefaultForType(typ, defaultVal string) error {
	switch typ {
	case "integer":
		if _, err := strconv.ParseInt(defaultVal, 10, 64); err != nil {
			return fmt.Errorf("default %q is not a valid integer", defaultVal)
		}
	case "number":
		if _, err := strconv.ParseFloat(defaultVal, 64); err != nil {
			return fmt.Errorf("default %q is not a valid number", defaultVal)
		}
	case "boolean":
		if defaultVal != "true" && defaultVal != "false" {
			return fmt.Errorf("default %q is not a valid boolean (must be \"true\" or \"false\")", defaultVal)
		}
	case "string":
		// Any string is valid.
	}

	return nil
}

// IsEmpty returns true if the config has no overrides.
func (c *TransformConfig) IsEmpty() bool {
	return len(c.Transformers) == 0 &&
		len(c.SchemaOverrides) == 0 &&
		len(c.ResourceIDOverrides) == 0
}
