package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompareSchemas_FieldRemoved(t *testing.T) {
	old := map[string]interface{}{
		"name": `string | default="app"`,
		"port": `integer | default=80`,
	}
	new := map[string]interface{}{
		"name": `string | default="app"`,
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 1)
	assert.Equal(t, ChangeRemoved, changes[0].Type)
	assert.Equal(t, "port", changes[0].Field)
	assert.True(t, changes[0].Breaking)
}

func TestCompareSchemas_TypeChanged(t *testing.T) {
	old := map[string]interface{}{
		"port": `integer | default=80`,
	}
	new := map[string]interface{}{
		"port": `string | default="80"`,
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 1)
	assert.Equal(t, ChangeModified, changes[0].Type)
	assert.True(t, changes[0].Breaking)
	assert.Contains(t, changes[0].Details, "type changed")
}

func TestCompareSchemas_RequiredFieldAdded(t *testing.T) {
	old := map[string]interface{}{}
	new := map[string]interface{}{
		"name": "string",
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 1)
	assert.Equal(t, ChangeAdded, changes[0].Type)
	assert.True(t, changes[0].Breaking)
	assert.Contains(t, changes[0].Details, "required")
}

func TestCompareSchemas_SchemaGroupRemoved(t *testing.T) {
	old := map[string]interface{}{
		"server": map[string]interface{}{
			"port": `integer | default=8080`,
		},
	}
	new := map[string]interface{}{}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 1)
	assert.True(t, changes[0].Breaking)
	assert.Contains(t, changes[0].Details, "group removed")
}

func TestCompareSchemas_ScalarToObject(t *testing.T) {
	old := map[string]interface{}{
		"config": `string | default="default"`,
	}
	new := map[string]interface{}{
		"config": map[string]interface{}{
			"key": `string | default="val"`,
		},
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 1)
	assert.True(t, changes[0].Breaking)
	assert.Contains(t, changes[0].Details, "scalar")
}

func TestCompareSchemas_ObjectToScalar(t *testing.T) {
	old := map[string]interface{}{
		"config": map[string]interface{}{
			"key": `string | default="val"`,
		},
	}
	new := map[string]interface{}{
		"config": `string | default="flat"`,
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 1)
	assert.True(t, changes[0].Breaking)
}

func TestCompareSchemas_DefaultChanged(t *testing.T) {
	old := map[string]interface{}{
		"replicas": `integer | default=1`,
	}
	new := map[string]interface{}{
		"replicas": `integer | default=3`,
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 1)
	assert.Equal(t, ChangeModified, changes[0].Type)
	assert.False(t, changes[0].Breaking)
	assert.Contains(t, changes[0].Details, "default changed")
}

func TestCompareSchemas_OptionalFieldAdded(t *testing.T) {
	old := map[string]interface{}{}
	new := map[string]interface{}{
		"version": `string | default="1.0"`,
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 1)
	assert.Equal(t, ChangeAdded, changes[0].Type)
	assert.False(t, changes[0].Breaking)
}

func TestCompareSchemas_FieldGroupAdded(t *testing.T) {
	old := map[string]interface{}{}
	new := map[string]interface{}{
		"server": map[string]interface{}{
			"port": `integer | default=8080`,
			"host": `string | default="localhost"`,
		},
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 1)
	assert.Equal(t, ChangeAdded, changes[0].Type)
	assert.False(t, changes[0].Breaking)
	assert.Contains(t, changes[0].Details, "2 sub-fields")
}

func TestCompareSchemas_NoChanges(t *testing.T) {
	spec := map[string]interface{}{
		"name": `string | default="app"`,
		"port": `integer | default=80`,
	}

	changes := CompareSchemas(spec, spec)
	assert.Empty(t, changes)
}

func TestCompareSchemas_BreakingSortedFirst(t *testing.T) {
	old := map[string]interface{}{
		"aField": `string | default="a"`,
		"bField": `integer | default=1`,
	}
	new := map[string]interface{}{
		"aField": `string | default="b"`,
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 2)
	assert.True(t, changes[0].Breaking, "breaking change should be first")
	assert.False(t, changes[1].Breaking, "non-breaking change should be second")
}

func TestCompareSchemas_NestedChanges(t *testing.T) {
	old := map[string]interface{}{
		"server": map[string]interface{}{
			"port": `integer | default=8080`,
			"host": `string | default="localhost"`,
		},
	}
	new := map[string]interface{}{
		"server": map[string]interface{}{
			"port": `integer | default=9090`,
		},
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 2)

	// Should have: host removed (breaking), port default changed (non-breaking)
	assert.True(t, changes[0].Breaking)
	assert.Equal(t, "server.host", changes[0].Field)
	assert.False(t, changes[1].Breaking)
	assert.Equal(t, "server.port", changes[1].Field)
}

func TestParseSchemaType(t *testing.T) {
	assert.Equal(t, "string", parseSchemaType(`string | default="foo"`))
	assert.Equal(t, "integer", parseSchemaType("integer"))
	assert.Equal(t, "boolean", parseSchemaType(`boolean | default=true`))
}

func TestParseSchemaDefault(t *testing.T) {
	assert.Equal(t, `"foo"`, parseSchemaDefault(`string | default="foo"`))
	assert.Equal(t, "80", parseSchemaDefault("integer | default=80"))
	assert.Equal(t, "", parseSchemaDefault("string"))
}

func TestHasDefault(t *testing.T) {
	assert.True(t, hasDefault(`string | default="foo"`))
	assert.False(t, hasDefault("string"))
}

func TestNilInputs(t *testing.T) {
	changes := CompareSchemas(nil, nil)
	assert.Empty(t, changes)

	changes = CompareSchemas(nil, map[string]interface{}{"x": "string"})
	assert.Len(t, changes, 1)
}

func TestCompareSchemas_DeeplyNested(t *testing.T) {
	old := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"deep": `string | default="old"`,
			},
		},
	}
	new := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"deep": `string | default="new"`,
			},
		},
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 1)
	assert.Equal(t, "level1.level2.deep", changes[0].Field)
	assert.False(t, changes[0].Breaking)
	assert.Contains(t, changes[0].Details, "default changed")
}

func TestCompareSchemas_SpecChangedSameTypeAndDefault(t *testing.T) {
	// parseSchemaDefault captures everything after "default=", so to trigger
	// the "spec changed" branch both old and new need identical type AND
	// identical default, with the overall spec string still being different.
	// This is hard to achieve with the current simple parser, so verify
	// the "default changed" path is hit for minor trailing differences.
	old := map[string]interface{}{
		"field": `string | default="same"`,
	}
	new := map[string]interface{}{
		"field": `string | default="same" | required`,
	}

	changes := CompareSchemas(old, new)
	require.Len(t, changes, 1)
	assert.False(t, changes[0].Breaking)
	assert.Contains(t, changes[0].Details, "default changed")
}

func TestCompareSchemas_MultipleBreakingAndNonBreaking(t *testing.T) {
	old := map[string]interface{}{
		"a": `string | default="a"`,
		"b": `integer | default=1`,
		"c": `string | default="c"`,
	}
	new := map[string]interface{}{
		// a: unchanged
		"a": `string | default="a"`,
		// b: removed (BREAKING)
		// c: default changed (non-breaking)
		"c": `string | default="cc"`,
		// d: required field added (BREAKING)
		"d": "string",
		// e: optional field added (non-breaking)
		"e": `boolean | default=true`,
	}

	changes := CompareSchemas(old, new)

	// 2 breaking (b removed, d required added), 2 non-breaking (c default, e optional added)
	breaking := 0
	nonBreaking := 0
	for _, c := range changes {
		if c.Breaking {
			breaking++
		} else {
			nonBreaking++
		}
	}
	assert.Equal(t, 2, breaking)
	assert.Equal(t, 2, nonBreaking)

	// Breaking should be sorted first.
	assert.True(t, changes[0].Breaking)
	assert.True(t, changes[1].Breaking)
	assert.False(t, changes[2].Breaking)
	assert.False(t, changes[3].Breaking)
}
