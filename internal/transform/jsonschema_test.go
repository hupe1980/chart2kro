package transform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/transform"
)

func TestNewJSONSchemaResolver_Empty(t *testing.T) {
	r, err := transform.NewJSONSchemaResolver(nil)
	assert.NoError(t, err)
	assert.Nil(t, r)

	r2, err := transform.NewJSONSchemaResolver([]byte{})
	assert.NoError(t, err)
	assert.Nil(t, r2)
}

func TestNewJSONSchemaResolver_Invalid(t *testing.T) {
	_, err := transform.NewJSONSchemaResolver([]byte("{invalid"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing values.schema.json")
}

func TestJSONSchemaResolver_Resolve(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"replicaCount": {
				"type": "integer",
				"description": "Number of replicas",
				"minimum": 1,
				"maximum": 100
			},
			"image": {
				"type": "object",
				"properties": {
					"repository": {
						"type": "string",
						"description": "Container image repository"
					},
					"tag": {
						"type": "string",
						"format": "semver",
						"description": "Container image tag"
					},
					"pullPolicy": {
						"type": "string",
						"enum": ["Always", "IfNotPresent", "Never"]
					}
				}
			},
			"service": {
				"type": "object",
				"properties": {
					"port": {
						"type": "number",
						"description": "Service port"
					},
					"enabled": {
						"type": "boolean"
					}
				}
			}
		}
	}`)

	r, err := transform.NewJSONSchemaResolver(schema)
	require.NoError(t, err)
	require.NotNil(t, r)

	t.Run("top-level integer", func(t *testing.T) {
		info := r.Resolve("replicaCount")
		require.NotNil(t, info)
		assert.Equal(t, "integer", info.Type)
		assert.Equal(t, "Number of replicas", info.Description)
		require.NotNil(t, info.Minimum)
		assert.Equal(t, float64(1), *info.Minimum)
		require.NotNil(t, info.Maximum)
		assert.Equal(t, float64(100), *info.Maximum)
	})

	t.Run("nested string with format", func(t *testing.T) {
		info := r.Resolve("image.tag")
		require.NotNil(t, info)
		assert.Equal(t, "string", info.Type)
		assert.Equal(t, "semver", info.Format)
	})

	t.Run("nested string with enum", func(t *testing.T) {
		info := r.Resolve("image.pullPolicy")
		require.NotNil(t, info)
		assert.Equal(t, "string", info.Type)
		assert.Len(t, info.Enum, 3)
	})

	t.Run("nested number", func(t *testing.T) {
		info := r.Resolve("service.port")
		require.NotNil(t, info)
		assert.Equal(t, "number", info.Type)
	})

	t.Run("nested boolean", func(t *testing.T) {
		info := r.Resolve("service.enabled")
		require.NotNil(t, info)
		assert.Equal(t, "boolean", info.Type)
	})

	t.Run("not found", func(t *testing.T) {
		info := r.Resolve("missing.path")
		assert.Nil(t, info)
	})

	t.Run("nil resolver", func(t *testing.T) {
		var nilResolver *transform.JSONSchemaResolver
		info := nilResolver.Resolve("replicaCount")
		assert.Nil(t, info)
	})
}

func TestMapToSimpleSchemaType(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"integer", "integer"},
		{"number", "number"},
		{"boolean", "boolean"},
		{"array", "array"},
		{"object", "object"},
		{"string", "string"},
		{"unknown", "string"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, transform.MapToSimpleSchemaType(tc.input))
		})
	}
}

func TestSchemaExtractor_WithJSONSchema(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"replicaCount": {
				"type": "integer",
				"description": "Number of replicas"
			},
			"service": {
				"type": "object",
				"properties": {
					"port": {
						"type": "number"
					}
				}
			}
		}
	}`)

	resolver, err := transform.NewJSONSchemaResolver(schema)
	require.NoError(t, err)

	values := map[string]interface{}{
		"replicaCount": float64(3), // Go infers "integer" from float64, JSON Schema confirms
		"service": map[string]interface{}{
			"port": float64(80), // Go infers "integer" (80.0), JSON Schema says "number"
		},
	}

	extractor := transform.NewSchemaExtractor(true, false, resolver)
	fields := extractor.Extract(values, nil)

	require.Len(t, fields, 2)

	// replicaCount: JSON Schema says integer, Go agrees.
	assert.Equal(t, "replicaCount", fields[0].Name)
	assert.Equal(t, "integer", fields[0].Type)

	// service.port: JSON Schema says number, should override Go's integer inference.
	require.True(t, fields[1].IsObject())
	require.Len(t, fields[1].Children, 1)
	assert.Equal(t, "port", fields[1].Children[0].Name)
	assert.Equal(t, "number", fields[1].Children[0].Type)
}

func TestSchemaExtractor_JSONSchemaOverridesNilType(t *testing.T) {
	schema := []byte(`{
		"type": "object",
		"properties": {
			"config": {
				"type": "string",
				"description": "Config value that defaults to null in values.yaml"
			}
		}
	}`)

	resolver, err := transform.NewJSONSchemaResolver(schema)
	require.NoError(t, err)

	values := map[string]interface{}{
		"config": nil, // nil in values.yaml, but JSON Schema says string
	}

	extractor := transform.NewSchemaExtractor(true, false, resolver)
	fields := extractor.Extract(values, nil)

	require.Len(t, fields, 1)
	assert.Equal(t, "string", fields[0].Type) // JSON Schema wins
}
