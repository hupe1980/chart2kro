package transform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/transform"
)

func TestInferType_AllTypes(t *testing.T) {
	tests := []struct {
		name    string
		val     interface{}
		wantTyp string
		wantDef string
	}{
		{"nil", nil, "string", ""},
		{"bool true", true, "boolean", "true"},
		{"bool false", false, "boolean", "false"},
		{"int", int(5), "integer", "5"},
		{"int64", int64(42), "integer", "42"},
		{"float64 integer", float64(3), "integer", "3"},
		{"float64 decimal", float64(3.14), "number", "3.14"},
		{"string", "nginx", "string", "\"nginx\""},
		{"empty string", "", "string", ""},
		{"array", []interface{}{1, 2}, "array", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := transform.NewSchemaExtractor(true, false, nil)
			fields := e.Extract(map[string]interface{}{"val": tt.val}, nil)
			require.Len(t, fields, 1)
			assert.Equal(t, tt.wantTyp, fields[0].Type)
			assert.Equal(t, tt.wantDef, fields[0].Default)
		})
	}
}

func TestSchemaField_SimpleSchemaString(t *testing.T) {
	tests := []struct {
		name string
		f    transform.SchemaField
		want string
	}{
		{"with default", transform.SchemaField{Type: "integer", Default: "3"}, "integer | default=3"},
		{"string default", transform.SchemaField{Type: "string", Default: "\"nginx\""}, "string | default=\"nginx\""},
		{"no default", transform.SchemaField{Type: "string"}, "string"},
		{"boolean", transform.SchemaField{Type: "boolean", Default: "true"}, "boolean | default=true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.f.SimpleSchemaString())
		})
	}
}

func TestExtract_NestedMode(t *testing.T) {
	values := map[string]interface{}{
		"replicaCount": float64(3),
		"image": map[string]interface{}{
			"repository": "nginx",
			"tag":        "1.25",
		},
		"service": map[string]interface{}{
			"type": "ClusterIP",
			"port": float64(80),
		},
	}

	e := transform.NewSchemaExtractor(true, false, nil)
	fields := e.Extract(values, nil)
	require.Len(t, fields, 3) // image, replicaCount, service (sorted)

	// image
	assert.Equal(t, "image", fields[0].Name)
	assert.True(t, fields[0].IsObject())
	require.Len(t, fields[0].Children, 2)
	assert.Equal(t, "repository", fields[0].Children[0].Name)
	assert.Equal(t, "tag", fields[0].Children[1].Name)

	// replicaCount
	assert.Equal(t, "replicaCount", fields[1].Name)
	assert.Equal(t, "integer", fields[1].Type)
	assert.Equal(t, "3", fields[1].Default)

	// service
	assert.Equal(t, "service", fields[2].Name)
	assert.True(t, fields[2].IsObject())
}

func TestExtract_FlatMode(t *testing.T) {
	values := map[string]interface{}{
		"image": map[string]interface{}{
			"repository": "nginx",
			"tag":        "1.25",
		},
		"service": map[string]interface{}{
			"port": float64(80),
		},
	}

	e := transform.NewSchemaExtractor(true, true, nil)
	fields := e.Extract(values, nil)
	require.Len(t, fields, 3)

	assert.Equal(t, "imageRepository", fields[0].Name)
	assert.Equal(t, "imageTag", fields[1].Name)
	assert.Equal(t, "servicePort", fields[2].Name)
}

func TestExtract_Pruning(t *testing.T) {
	values := map[string]interface{}{
		"replicaCount": float64(3),
		"unused":       "should be excluded",
		"image": map[string]interface{}{
			"repository": "nginx",
			"tag":        "1.25",
		},
	}

	refs := map[string]bool{
		"replicaCount":     true,
		"image.repository": true,
	}

	e := transform.NewSchemaExtractor(false, false, nil)
	fields := e.Extract(values, refs)

	// Should have image (with only repository) and replicaCount.
	require.Len(t, fields, 2)
	assert.Equal(t, "image", fields[0].Name)
	require.Len(t, fields[0].Children, 1)
	assert.Equal(t, "repository", fields[0].Children[0].Name)
	assert.Equal(t, "replicaCount", fields[1].Name)
}

func TestExtract_IncludeAll(t *testing.T) {
	values := map[string]interface{}{
		"a": "x",
		"b": "y",
	}

	refs := map[string]bool{"a": true}

	e := transform.NewSchemaExtractor(true, false, nil)
	fields := e.Extract(values, refs)
	assert.Len(t, fields, 2) // IncludeAll overrides pruning.
}

func TestExtract_EmptyValues(t *testing.T) {
	e := transform.NewSchemaExtractor(true, false, nil)
	fields := e.Extract(map[string]interface{}{}, nil)
	assert.Empty(t, fields)
}

func TestExtract_DeeplyNested(t *testing.T) {
	values := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": "deep",
			},
		},
	}

	e := transform.NewSchemaExtractor(true, false, nil)
	fields := e.Extract(values, nil)
	require.Len(t, fields, 1)
	require.Len(t, fields[0].Children, 1)
	require.Len(t, fields[0].Children[0].Children, 1)
	assert.Equal(t, "c", fields[0].Children[0].Children[0].Name)
	assert.Equal(t, "a.b.c", fields[0].Children[0].Children[0].Path)
}

func TestBuildSimpleSchema(t *testing.T) {
	fields := []*transform.SchemaField{
		{Name: "replicas", Type: "integer", Default: "3"},
		{Name: "image", Type: "object", Children: []*transform.SchemaField{
			{Name: "repository", Type: "string", Default: "\"nginx\""},
			{Name: "tag", Type: "string", Default: "\"1.25\""},
		}},
	}

	result := transform.BuildSimpleSchema(fields)
	assert.Equal(t, "integer | default=3", result["replicas"])

	imgMap, ok := result["image"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "string | default=\"nginx\"", imgMap["repository"])
	assert.Equal(t, "string | default=\"1.25\"", imgMap["tag"])
}

func TestExtract_NilValue(t *testing.T) {
	values := map[string]interface{}{
		"optional": nil,
	}

	e := transform.NewSchemaExtractor(true, false, nil)
	fields := e.Extract(values, nil)
	require.Len(t, fields, 1)
	assert.Equal(t, "string", fields[0].Type)
	assert.Equal(t, "", fields[0].Default)
}

// ---------------------------------------------------------------------------
// ApplySchemaOverrides
// ---------------------------------------------------------------------------

func TestApplySchemaOverrides_TypeAndDefault(t *testing.T) {
	fields := []*transform.SchemaField{
		{Name: "replicaCount", Path: "replicaCount", Type: "integer", Default: "1"},
		{Name: "image", Path: "image", Type: "object", Children: []*transform.SchemaField{
			{Name: "tag", Path: "image.tag", Type: "string", Default: "\"latest\""},
		}},
	}

	overrides := map[string]transform.SchemaOverride{
		"replicaCount": {Type: "integer", Default: "3"},
		"image.tag":    {Default: "\"v2.0\""},
	}

	transform.ApplySchemaOverrides(fields, overrides)

	assert.Equal(t, "3", fields[0].Default, "replicaCount default should be overridden")
	assert.Equal(t, "integer", fields[0].Type, "type should remain integer")
	assert.Equal(t, "\"v2.0\"", fields[1].Children[0].Default, "nested default should be overridden")
}

func TestApplySchemaOverrides_TypeOnly(t *testing.T) {
	fields := []*transform.SchemaField{
		{Name: "port", Path: "port", Type: "string", Default: "\"8080\""},
	}

	overrides := map[string]transform.SchemaOverride{
		"port": {Type: "integer"},
	}

	transform.ApplySchemaOverrides(fields, overrides)

	assert.Equal(t, "integer", fields[0].Type, "type should be overridden")
	assert.Equal(t, "\"8080\"", fields[0].Default, "default should remain unchanged")
}

func TestApplySchemaOverrides_EmptyOverrides(t *testing.T) {
	fields := []*transform.SchemaField{
		{Name: "x", Path: "x", Type: "string", Default: "\"a\""},
	}

	transform.ApplySchemaOverrides(fields, nil)
	assert.Equal(t, "\"a\"", fields[0].Default, "nothing should change with nil overrides")

	transform.ApplySchemaOverrides(fields, map[string]transform.SchemaOverride{})
	assert.Equal(t, "\"a\"", fields[0].Default, "nothing should change with empty overrides")
}
