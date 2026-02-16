package output

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sigsyaml "sigs.k8s.io/yaml"
)

func testRGD() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata": map[string]interface{}{
			"name": "test",
		},
		"spec": map[string]interface{}{
			"resources": []interface{}{},
		},
	}
}

func TestSerialize_FieldOrdering(t *testing.T) {
	out, err := Serialize(testRGD(), DefaultSerializeOptions())
	require.NoError(t, err)

	yaml := string(out)
	assert.True(t, strings.Index(yaml, "apiVersion") < strings.Index(yaml, "kind"))
	assert.True(t, strings.Index(yaml, "kind") < strings.Index(yaml, "metadata"))
	assert.True(t, strings.Index(yaml, "metadata") < strings.Index(yaml, "spec"))

	lines := strings.Split(yaml, "\n")
	assert.True(t, strings.HasPrefix(lines[0], "apiVersion:"))
	assert.True(t, strings.HasPrefix(lines[1], "kind:"))
	assert.True(t, strings.HasPrefix(lines[2], "metadata:"))
}

func TestSerialize_SortedMapKeys(t *testing.T) {
	rgd := testRGD()
	rgd["metadata"] = map[string]interface{}{
		"name":   "test",
		"labels": map[string]interface{}{"zebra": "last", "alpha": "first"},
	}

	out, err := Serialize(rgd, DefaultSerializeOptions())
	require.NoError(t, err)
	assert.True(t, strings.Index(string(out), "alpha") < strings.Index(string(out), "zebra"))
}

func TestSerialize_NoNullValues(t *testing.T) {
	rgd := testRGD()
	rgd["metadata"] = map[string]interface{}{"name": "test", "annotations": nil}

	out, err := Serialize(rgd, DefaultSerializeOptions())
	require.NoError(t, err)
	assert.NotContains(t, string(out), "null")
}

func TestSerialize_ConsistentIndentation(t *testing.T) {
	out, err := Serialize(testRGD(), DefaultSerializeOptions())
	require.NoError(t, err)

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, " ") {
			trimmed := strings.TrimLeft(line, " ")
			indent := len(line) - len(trimmed)
			assert.Equal(t, 0, indent%2, "indentation should be multiples of 2: %q", line)
		}
	}
}

func TestSerialize_TrailingNewline(t *testing.T) {
	out, err := Serialize(testRGD(), DefaultSerializeOptions())
	require.NoError(t, err)
	assert.True(t, len(out) > 0 && out[len(out)-1] == '\n')
}

func TestSerialize_EmptyMap(t *testing.T) {
	_, err := Serialize(map[string]interface{}{}, DefaultSerializeOptions())
	require.NoError(t, err)
}

func TestSerialize_WithComments(t *testing.T) {
	rgd := map[string]interface{}{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata":   map[string]interface{}{"name": "test"},
		"spec": map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"id": "deployment",
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"replicas": "${schema.spec.replicaCount}",
						},
					},
				},
			},
		},
	}

	opts := SerializeOptions{Comments: true, Indent: 2}
	out, err := Serialize(rgd, opts)
	require.NoError(t, err)

	yaml := string(out)
	assert.Contains(t, yaml, "# From Helm values: .Values.replicaCount")
	commentIdx := strings.Index(yaml, "# From Helm values:")
	exprIdx := strings.Index(yaml, "${schema.spec.replicaCount}")
	assert.True(t, commentIdx < exprIdx, "comment should appear before expression")
}

func TestSerialize_WithoutComments(t *testing.T) {
	rgd := map[string]interface{}{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata":   map[string]interface{}{"name": "test"},
		"spec": map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"id":       "deployment",
					"template": map[string]interface{}{"spec": map[string]interface{}{"replicas": "${schema.spec.replicaCount}"}},
				},
			},
		},
	}

	out, err := Serialize(rgd, DefaultSerializeOptions())
	require.NoError(t, err)
	assert.NotContains(t, string(out), "# From Helm values:")
}

func TestSerialize_CommentsDoNotBreakYAML(t *testing.T) {
	rgd := map[string]interface{}{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata":   map[string]interface{}{"name": "test"},
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"status": map[string]interface{}{
					"replicas": "${deployment.status.availableReplicas}",
				},
			},
			"resources": []interface{}{
				map[string]interface{}{
					"id":        "deployment",
					"readyWhen": []interface{}{"${self.status.availableReplicas == self.status.replicas}"},
					"template":  map[string]interface{}{"spec": map[string]interface{}{"replicas": "${schema.spec.replicaCount}"}},
				},
			},
		},
	}

	opts := SerializeOptions{Comments: true, Indent: 2}
	out, err := Serialize(rgd, opts)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, sigsyaml.Unmarshal(out, &parsed))
	assert.Equal(t, "kro.run/v1alpha1", parsed["apiVersion"])
}

func TestDescribeCELExpression(t *testing.T) {
	tests := []struct {
		expr, expected string
	}{
		{"${schema.spec.replicaCount}", "From Helm values: .Values.replicaCount"},
		{"${self.status.availableReplicas}", "Readiness/status self-reference"},
		{"${deployment.status.availableReplicas}", "Status from resource: deployment"},
		{"${configmap.metadata.name}", "Reference to resource: configmap"},
		{"plain-value", ""},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			assert.Equal(t, tt.expected, describeCELExpression(tt.expr))
		})
	}
}

func TestDeepCleanMap(t *testing.T) {
	t.Run("removes nil values", func(t *testing.T) {
		result := deepCleanMap(map[string]interface{}{"keep": "val", "drop": nil})
		assert.Equal(t, "val", result["keep"])
		assert.NotContains(t, result, "drop")
	})

	t.Run("removes empty nested maps", func(t *testing.T) {
		result := deepCleanMap(map[string]interface{}{"keep": "val", "empty": map[string]interface{}{}})
		assert.NotContains(t, result, "empty")
	})

	t.Run("preserves non-empty nested maps", func(t *testing.T) {
		result := deepCleanMap(map[string]interface{}{"nested": map[string]interface{}{"key": "val"}})
		nested := result["nested"].(map[string]interface{})
		assert.Equal(t, "val", nested["key"])
	})
}

func TestSerializeJSON(t *testing.T) {
	out, err := SerializeJSON(testRGD(), "  ")
	require.NoError(t, err)

	json := string(out)
	assert.Contains(t, json, `"apiVersion"`)
	assert.Contains(t, json, `"kro.run/v1alpha1"`)
	assert.True(t, json[len(json)-1] == '\n')
}

func TestSerializeJSON_SortedKeys(t *testing.T) {
	rgd := testRGD()
	rgd["metadata"] = map[string]interface{}{
		"name":   "test",
		"labels": map[string]interface{}{"zebra": "z", "alpha": "a"},
	}

	out, err := SerializeJSON(rgd, "  ")
	require.NoError(t, err)
	assert.True(t, strings.Index(string(out), `"alpha"`) < strings.Index(string(out), `"zebra"`))
}

func TestStripNullFields(t *testing.T) {
	input := "key: value\n  nullField: null\n  kept: true\n"
	result := string(stripNullFields([]byte(input)))
	assert.NotContains(t, result, "null")
	assert.Contains(t, result, "key: value")
	assert.Contains(t, result, "kept: true")
}

func TestSerialize_Determinism(t *testing.T) {
	rgd := map[string]interface{}{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata": map[string]interface{}{
			"name":   "test",
			"labels": map[string]interface{}{"chart": "nginx", "version": "1.0.0"},
		},
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"apiVersion": "v1alpha1",
				"kind":       "NginxApp",
				"spec":       map[string]interface{}{"replicaCount": "integer"},
			},
			"resources": []interface{}{
				map[string]interface{}{
					"id":       "deployment",
					"template": map[string]interface{}{"kind": "Deployment", "spec": map[string]interface{}{"replicas": "${schema.spec.replicaCount}"}},
				},
				map[string]interface{}{
					"id":       "service",
					"template": map[string]interface{}{"kind": "Service"},
				},
			},
		},
	}

	first, err := Serialize(rgd, DefaultSerializeOptions())
	require.NoError(t, err)

	for i := range 10 {
		out, err := Serialize(rgd, DefaultSerializeOptions())
		require.NoError(t, err)
		assert.Equal(t, string(first), string(out), "run %d produced different output", i+1)
	}
}

func TestSerialize_RoundTrip(t *testing.T) {
	rgd := map[string]interface{}{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata": map[string]interface{}{
			"name":   "roundtrip-test",
			"labels": map[string]interface{}{"app": "test"},
		},
		"spec": map[string]interface{}{
			"schema": map[string]interface{}{
				"apiVersion": "v1alpha1",
				"kind":       "TestApp",
				"spec":       map[string]interface{}{"replicas": "integer"},
			},
			"resources": []interface{}{
				map[string]interface{}{
					"id": "svc",
					"template": map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Service",
						"metadata":   map[string]interface{}{"name": "my-svc"},
					},
				},
			},
		},
	}

	// Serialize once.
	first, err := Serialize(rgd, DefaultSerializeOptions())
	require.NoError(t, err)

	// Parse back.
	var parsed map[string]interface{}
	require.NoError(t, sigsyaml.Unmarshal(first, &parsed))

	// Serialize again from parsed result.
	second, err := Serialize(parsed, DefaultSerializeOptions())
	require.NoError(t, err)

	// Round-trip produces identical output.
	assert.Equal(t, string(first), string(second), "round-trip serialization should be identical")
}

func TestSerializeJSON_RoundTrip(t *testing.T) {
	rgd := testRGD()
	rgd["metadata"] = map[string]interface{}{
		"name":   "json-roundtrip",
		"labels": map[string]interface{}{"a": "1", "b": "2"},
	}

	first, err := SerializeJSON(rgd, "  ")
	require.NoError(t, err)

	// Parse JSON back to map.
	var parsed map[string]interface{}
	require.NoError(t, sigsyaml.Unmarshal(first, &parsed))

	// Re-serialize.
	second, err := SerializeJSON(parsed, "  ")
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second), "JSON round-trip should be identical")
}

func TestJsonQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "hello", `"hello"`},
		{"with quotes", `say "hi"`, `"say \"hi\""`},
		{"with backslash", `path\to`, `"path\\to"`},
		{"with newline", "line1\nline2", `"line1\nline2"`},
		{"with tab", "a\tb", `"a\tb"`},
		{"with carriage return", "a\rb", `"a\rb"`},
		{"with backspace", "a\bb", `"a\bb"`},
		{"with formfeed", "a\fb", `"a\fb"`},
		{"empty", "", `""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, jsonQuote(tt.input))
		})
	}
}

func TestCanonicalizeRGD_PreservesResourceOrder(t *testing.T) {
	rgd := map[string]interface{}{
		"spec": map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"id": "first"},
				map[string]interface{}{"id": "second"},
				map[string]interface{}{"id": "third"},
			},
		},
	}

	result := canonicalizeRGD(rgd)
	resources := result["spec"].(map[string]interface{})["resources"].([]interface{})
	assert.Equal(t, "first", resources[0].(map[string]interface{})["id"])
	assert.Equal(t, "second", resources[1].(map[string]interface{})["id"])
	assert.Equal(t, "third", resources[2].(map[string]interface{})["id"])
}

func TestDeepCleanValue_PreservesScalarTypes(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{"string", "hello"},
		{"int", 42},
		{"float", 3.14},
		{"bool", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.input, deepCleanValue(tt.input))
		})
	}
}

func TestSerialize_NestedNullRemoval(t *testing.T) {
	rgd := map[string]interface{}{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata": map[string]interface{}{
			"name":        "test",
			"annotations": map[string]interface{}{"keep": "yes", "drop": nil},
		},
		"spec": map[string]interface{}{
			"resources": []interface{}{},
		},
	}

	out, err := Serialize(rgd, DefaultSerializeOptions())
	require.NoError(t, err)
	assert.NotContains(t, string(out), "null")
	assert.Contains(t, string(out), "keep: \"yes\"")
}
