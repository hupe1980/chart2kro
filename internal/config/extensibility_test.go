package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ParseTransformConfig
// ---------------------------------------------------------------------------

func TestParseTransformConfig_Transformers(t *testing.T) {
	data := []byte(`
transformers:
  - match:
      kind: Deployment
      apiVersion: apps/v1
    readyWhen:
      - "self.status.availableReplicas == self.status.replicas"
    statusFields:
      - name: deploymentReady
        celExpression: "webDeployment.status.availableReplicas"
`)

	cfg, err := ParseTransformConfig(data)
	require.NoError(t, err)
	require.Len(t, cfg.Transformers, 1)
	assert.Equal(t, "Deployment", cfg.Transformers[0].Match.Kind)
	assert.Equal(t, "apps/v1", cfg.Transformers[0].Match.APIVersion)
	assert.Len(t, cfg.Transformers[0].ReadyWhen, 1)
	assert.Len(t, cfg.Transformers[0].StatusFields, 1)
}

func TestParseTransformConfig_SchemaOverrides(t *testing.T) {
	data := []byte(`
schemaOverrides:
  image.tag:
    type: string
    default: "\"latest\""
  replicas:
    type: integer
`)

	cfg, err := ParseTransformConfig(data)
	require.NoError(t, err)
	require.Len(t, cfg.SchemaOverrides, 2)
	assert.Equal(t, "string", cfg.SchemaOverrides["image.tag"].Type)
	assert.Equal(t, "\"latest\"", cfg.SchemaOverrides["image.tag"].Default)
	assert.Equal(t, "integer", cfg.SchemaOverrides["replicas"].Type)
}

func TestParseTransformConfig_ResourceIDOverrides(t *testing.T) {
	data := []byte(`
resourceIdOverrides:
  "apps/v1/Deployment/web": "mainDeployment"
  "v1/Service/api": "apiService"
`)

	cfg, err := ParseTransformConfig(data)
	require.NoError(t, err)
	require.Len(t, cfg.ResourceIDOverrides, 2)
	assert.Equal(t, "mainDeployment", cfg.ResourceIDOverrides["apps/v1/Deployment/web"])
	assert.Equal(t, "apiService", cfg.ResourceIDOverrides["v1/Service/api"])
}

func TestParseTransformConfig_Empty(t *testing.T) {
	cfg, err := ParseTransformConfig([]byte("log-level: info\n"))
	require.NoError(t, err)
	assert.True(t, cfg.IsEmpty())
}

func TestParseTransformConfig_ValidationError_MissingKind(t *testing.T) {
	data := []byte(`
transformers:
  - match:
      apiVersion: apps/v1
    readyWhen:
      - "always"
`)

	_, err := ParseTransformConfig(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "match.kind is required")
}

func TestParseTransformConfig_ValidationError_InvalidType(t *testing.T) {
	data := []byte(`
schemaOverrides:
  replicas:
    type: float
`)

	_, err := ParseTransformConfig(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
}

func TestParseTransformConfig_MalformedYAML(t *testing.T) {
	_, err := ParseTransformConfig([]byte(": bad yaml :"))
	require.Error(t, err)
}

func TestTransformConfig_IsEmpty(t *testing.T) {
	assert.True(t, (&TransformConfig{}).IsEmpty())
	assert.False(t, (&TransformConfig{
		ResourceIDOverrides: map[string]string{"a": "b"},
	}).IsEmpty())
}

func TestTransformConfig_Validate_Valid(t *testing.T) {
	cfg := &TransformConfig{
		Transformers: []TransformerOverride{
			{Match: TransformerMatch{Kind: "Deployment"}},
		},
		SchemaOverrides: map[string]SchemaOverride{
			"replicas": {Type: "integer"},
		},
	}
	assert.NoError(t, cfg.Validate())
}

func TestValidate_ResourceIDOverrides_Empty(t *testing.T) {
	cfg := &TransformConfig{
		ResourceIDOverrides: map[string]string{"apps/v1/Deployment/web": ""},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestValidate_ResourceIDOverrides_InvalidChars(t *testing.T) {
	tests := []struct {
		name string
		val  string
	}{
		{"spaces", "main deployment"},
		{"dots", "main.deployment"},
		{"starts with digit", "1deploy"},
		{"CEL operator", "a+b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &TransformConfig{
				ResourceIDOverrides: map[string]string{"key": tt.val},
			}
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid")
		})
	}
}

func TestValidate_ResourceIDOverrides_Valid(t *testing.T) {
	cfg := &TransformConfig{
		ResourceIDOverrides: map[string]string{
			"apps/v1/Deployment/web": "mainDeployment",
			"v1/Service/api":         "api-service",
		},
	}
	assert.NoError(t, cfg.Validate())
}

func TestValidate_SchemaDefault_IntegerMismatch(t *testing.T) {
	cfg := &TransformConfig{
		SchemaOverrides: map[string]SchemaOverride{
			"replicas": {Type: "integer", Default: "hello"},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid integer")
}

func TestValidate_SchemaDefault_NumberMismatch(t *testing.T) {
	cfg := &TransformConfig{
		SchemaOverrides: map[string]SchemaOverride{
			"ratio": {Type: "number", Default: "abc"},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid number")
}

func TestValidate_SchemaDefault_BooleanMismatch(t *testing.T) {
	cfg := &TransformConfig{
		SchemaOverrides: map[string]SchemaOverride{
			"enabled": {Type: "boolean", Default: "yes"},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid boolean")
}

func TestValidate_SchemaDefault_ValidValues(t *testing.T) {
	tests := []struct {
		typ    string
		defVal string
	}{
		{"integer", "42"},
		{"number", "3.14"},
		{"boolean", "true"},
		{"boolean", "false"},
		{"string", "anything"},
	}
	for _, tt := range tests {
		t.Run(tt.typ+"_"+tt.defVal, func(t *testing.T) {
			cfg := &TransformConfig{
				SchemaOverrides: map[string]SchemaOverride{
					"field": {Type: tt.typ, Default: tt.defVal},
				},
			}
			assert.NoError(t, cfg.Validate())
		})
	}
}
