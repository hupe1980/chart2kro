package plan

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyze_NoChanges(t *testing.T) {
	rgd := map[string]interface{}{
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"spec": map[string]interface{}{
					"name": `string | default="app"`,
				},
			},
			"resources": []interface{}{
				map[string]interface{}{
					"id":       "deployment",
					"template": map[string]interface{}{"kind": "Deployment"},
				},
			},
		},
	}

	result := Analyze(rgd, rgd)
	assert.False(t, result.HasChanges())
	assert.False(t, result.HasBreakingChanges())
	assert.Equal(t, 0, result.BreakingCount())
	assert.Equal(t, 0, result.NonBreakingCount())
}

func TestAnalyze_BreakingSchemaChange(t *testing.T) {
	old := map[string]interface{}{
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"spec": map[string]interface{}{
					"name": `string | default="app"`,
					"port": `integer | default=80`,
				},
			},
			"resources": []interface{}{},
		},
	}
	new := map[string]interface{}{
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"spec": map[string]interface{}{
					"name": `string | default="app"`,
				},
			},
			"resources": []interface{}{},
		},
	}

	result := Analyze(old, new)
	assert.True(t, result.HasChanges())
	assert.True(t, result.HasBreakingChanges())
	assert.Equal(t, 1, result.BreakingCount())
}

func TestAnalyze_NonBreakingChanges(t *testing.T) {
	old := map[string]interface{}{
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"spec": map[string]interface{}{
					"name": `string | default="app"`,
				},
			},
			"resources": []interface{}{
				map[string]interface{}{
					"id":       "deployment",
					"template": map[string]interface{}{"kind": "Deployment"},
				},
			},
		},
	}
	new := map[string]interface{}{
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"spec": map[string]interface{}{
					"name":    `string | default="app"`,
					"version": `string | default="1.0"`,
				},
			},
			"resources": []interface{}{
				map[string]interface{}{
					"id":       "deployment",
					"template": map[string]interface{}{"kind": "Deployment"},
				},
				map[string]interface{}{
					"id":       "service",
					"template": map[string]interface{}{"kind": "Service"},
				},
			},
		},
	}

	result := Analyze(old, new)
	assert.True(t, result.HasChanges())
	assert.False(t, result.HasBreakingChanges())
	assert.Equal(t, 2, result.NonBreakingCount())
}

func TestFormatTable_NoChanges(t *testing.T) {
	result := &EvolutionResult{}
	var buf bytes.Buffer
	FormatTable(&buf, result)
	assert.Contains(t, buf.String(), "No changes detected.")
}

func TestFormatTable_WithBreaking(t *testing.T) {
	result := &EvolutionResult{
		SchemaChanges: []SchemaChange{
			{Type: ChangeRemoved, Field: "port", Details: "removed", Impact: "fail", Breaking: true},
		},
	}

	var buf bytes.Buffer
	FormatTable(&buf, result)
	out := buf.String()
	assert.Contains(t, out, "Schema Changes:")
	assert.Contains(t, out, "Breaking changes: 1")
	assert.Contains(t, out, "WARNING")
}

func TestFormatJSON(t *testing.T) {
	result := &EvolutionResult{
		SchemaChanges: []SchemaChange{
			{Type: ChangeRemoved, Field: "port", Breaking: true},
		},
	}

	var buf bytes.Buffer
	err := FormatJSON(&buf, result)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)
	assert.Contains(t, parsed, "schemaChanges")
}

func TestFormatCompactSummary(t *testing.T) {
	result := &EvolutionResult{}
	assert.Equal(t, "No changes detected.", FormatCompactSummary(result))

	result = &EvolutionResult{
		SchemaChanges: []SchemaChange{
			{Type: ChangeAdded},
			{Type: ChangeRemoved},
		},
		ResourceChanges: []ResourceChange{
			{Type: ChangeModified},
		},
	}
	summary := FormatCompactSummary(result)
	assert.Contains(t, summary, "1 schema fields added")
	assert.Contains(t, summary, "1 schema fields removed")
	assert.Contains(t, summary, "1 resources modified")
}

func TestExtractSchemaSpec(t *testing.T) {
	rgd := map[string]interface{}{
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"spec": map[string]interface{}{
					"name": `string | default="app"`,
				},
			},
		},
	}

	spec := extractSchemaSpec(rgd)
	require.NotNil(t, spec)
	assert.Equal(t, `string | default="app"`, spec["name"])
	assert.Nil(t, extractSchemaSpec(map[string]interface{}{}))
}

func TestExtractResources(t *testing.T) {
	rgd := map[string]interface{}{
		"spec": map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"id": "svc"},
			},
		},
	}
	res := extractResources(rgd)
	require.Len(t, res, 1)

	assert.Nil(t, extractResources(map[string]interface{}{}))
}

func TestAnalyze_MixedChanges(t *testing.T) {
	old := map[string]interface{}{
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"spec": map[string]interface{}{
					"name": `string | default="app"`,
					"port": `integer | default=80`,
				},
			},
			"resources": []interface{}{
				map[string]interface{}{
					"id":       "deployment",
					"template": map[string]interface{}{"kind": "Deployment"},
				},
				map[string]interface{}{
					"id":       "oldService",
					"template": map[string]interface{}{"kind": "Service"},
				},
			},
		},
	}
	new := map[string]interface{}{
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"spec": map[string]interface{}{
					"name":    `string | default="app"`,
					"version": `string | default="1.0"`,
				},
			},
			"resources": []interface{}{
				map[string]interface{}{
					"id":       "deployment",
					"template": map[string]interface{}{"kind": "Deployment"},
				},
				map[string]interface{}{
					"id":       "configMap",
					"template": map[string]interface{}{"kind": "ConfigMap"},
				},
			},
		},
	}

	result := Analyze(old, new)
	assert.True(t, result.HasChanges())
	assert.True(t, result.HasBreakingChanges())

	// Schema: port removed (breaking), version added (non-breaking)
	// Resources: oldService removed (breaking), configMap added (non-breaking)
	assert.Equal(t, 2, result.BreakingCount())
	assert.Equal(t, 2, result.NonBreakingCount())
}

func TestFormatJSON_Structure(t *testing.T) {
	result := &EvolutionResult{
		SchemaChanges: []SchemaChange{
			{Type: ChangeRemoved, Field: "port", Details: "removed", Breaking: true},
			{Type: ChangeAdded, Field: "version", Details: "added", Breaking: false},
		},
		ResourceChanges: []ResourceChange{
			{Type: ChangeAdded, ID: "configMap", Kind: "ConfigMap", Breaking: false},
		},
	}

	var buf bytes.Buffer
	err := FormatJSON(&buf, result)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	summary := parsed["summary"].(map[string]interface{})
	assert.Equal(t, float64(1), summary["breaking"])
	assert.Equal(t, float64(2), summary["nonBreaking"])
	assert.True(t, summary["hasBreaking"].(bool))

	schemaChanges := parsed["schemaChanges"].([]interface{})
	assert.Len(t, schemaChanges, 2)

	resourceChanges := parsed["resourceChanges"].([]interface{})
	assert.Len(t, resourceChanges, 1)
}

func TestFormatTable_ResourceChanges(t *testing.T) {
	result := &EvolutionResult{
		ResourceChanges: []ResourceChange{
			{Type: ChangeRemoved, ID: "deployment", Kind: "Deployment", Details: "removed", Breaking: true},
			{Type: ChangeAdded, ID: "service", Kind: "Service", Details: "added", Breaking: false},
		},
	}

	var buf bytes.Buffer
	FormatTable(&buf, result)
	out := buf.String()
	assert.Contains(t, out, "Resource Changes:")
	assert.Contains(t, out, "deployment")
	assert.Contains(t, out, "service")
	assert.Contains(t, out, "Breaking changes: 1")
}

func TestFormatJSON_EmptyChanges(t *testing.T) {
	result := &EvolutionResult{}

	var buf bytes.Buffer
	err := FormatJSON(&buf, result)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	// Empty arrays, not null.
	schemaChanges := parsed["schemaChanges"].([]interface{})
	assert.Len(t, schemaChanges, 0)

	resourceChanges := parsed["resourceChanges"].([]interface{})
	assert.Len(t, resourceChanges, 0)
}
