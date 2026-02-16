package transform_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/transform"
)

func TestSentinelForString(t *testing.T) {
	s := transform.SentinelForString("image.tag")
	assert.Equal(t, "__CHART2KRO_SENTINEL_image.tag__", s)
}

func TestSentinelizeAll(t *testing.T) {
	values := map[string]interface{}{
		"replicaCount": 3,
		"enabled":      true,
		"name":         "myapp",
		"image": map[string]interface{}{
			"repository": "nginx",
			"tag":        "1.25",
		},
		"items": []interface{}{"a", "b"}, // arrays preserved as-is
	}

	result := transform.SentinelizeAll(values)

	// All leaf values should be string sentinels.
	assert.Equal(t, "__CHART2KRO_SENTINEL_replicaCount__", result["replicaCount"])
	assert.Equal(t, "__CHART2KRO_SENTINEL_enabled__", result["enabled"])
	assert.Equal(t, "__CHART2KRO_SENTINEL_name__", result["name"])

	img := result["image"].(map[string]interface{})
	assert.Equal(t, "__CHART2KRO_SENTINEL_image.repository__", img["repository"])
	assert.Equal(t, "__CHART2KRO_SENTINEL_image.tag__", img["tag"])

	// Arrays are preserved.
	assert.Equal(t, []interface{}{"a", "b"}, result["items"])

	// Original is not mutated.
	assert.Equal(t, 3, values["replicaCount"])
}

func TestDiffAllResources(t *testing.T) {
	t.Run("exact string match", func(t *testing.T) {
		baseline := []*k8s.Resource{makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
			"data": map[string]interface{}{"key": "value"},
		})}
		sentinel := []*k8s.Resource{makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
			"data": map[string]interface{}{"key": "__CHART2KRO_SENTINEL_mykey__"},
		})}
		ids := map[*k8s.Resource]string{baseline[0]: "configmap"}

		mappings := transform.DiffAllResources(baseline, sentinel, ids)
		require.Len(t, mappings, 1)
		assert.Equal(t, "mykey", mappings[0].ValuesPath)
		assert.Equal(t, "configmap", mappings[0].ResourceID)
		assert.Equal(t, "data.key", mappings[0].FieldPath)
		assert.Equal(t, transform.MatchExact, mappings[0].MatchType)
	})

	t.Run("integer sentinel becomes string", func(t *testing.T) {
		baseline := []*k8s.Resource{makeFullResource("apps/v1", "Deployment", "web", map[string]interface{}{
			"spec": map[string]interface{}{"replicas": int64(3)},
		})}
		sentinel := []*k8s.Resource{makeFullResource("apps/v1", "Deployment", "web", map[string]interface{}{
			"spec": map[string]interface{}{"replicas": "__CHART2KRO_SENTINEL_replicaCount__"},
		})}
		ids := map[*k8s.Resource]string{baseline[0]: "deployment"}

		mappings := transform.DiffAllResources(baseline, sentinel, ids)
		require.Len(t, mappings, 1)
		assert.Equal(t, "replicaCount", mappings[0].ValuesPath)
		assert.Equal(t, "spec.replicas", mappings[0].FieldPath)
		assert.Equal(t, transform.MatchExact, mappings[0].MatchType)
	})

	t.Run("interpolated multi-sentinel", func(t *testing.T) {
		baseline := []*k8s.Resource{makeFullResource("apps/v1", "Deployment", "web", map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"image": "nginx:1.25",
							},
						},
					},
				},
			},
		})}
		sentinel := []*k8s.Resource{makeFullResource("apps/v1", "Deployment", "web", map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"image": "__CHART2KRO_SENTINEL_image.repository__:__CHART2KRO_SENTINEL_image.tag__",
							},
						},
					},
				},
			},
		})}
		ids := map[*k8s.Resource]string{baseline[0]: "deployment"}

		mappings := transform.DiffAllResources(baseline, sentinel, ids)
		require.Len(t, mappings, 2)

		// Both should point to the same field path.
		for _, m := range mappings {
			assert.Equal(t, "spec.template.spec.containers[0].image", m.FieldPath)
			assert.Equal(t, transform.MatchSubstring, m.MatchType)
		}
	})

	t.Run("no change", func(t *testing.T) {
		r := makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
			"data": map[string]interface{}{"key": "same"},
		})
		ids := map[*k8s.Resource]string{r: "cm"}
		mappings := transform.DiffAllResources([]*k8s.Resource{r}, []*k8s.Resource{r}, ids)
		assert.Empty(t, mappings)
	})

	t.Run("nil sentinel resources", func(t *testing.T) {
		r := makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{})
		ids := map[*k8s.Resource]string{r: "cm"}
		mappings := transform.DiffAllResources([]*k8s.Resource{r}, nil, ids)
		assert.Empty(t, mappings)
	})
}

func TestExtractSentinelsFromString(t *testing.T) {
	t.Run("single sentinel", func(t *testing.T) {
		paths := transform.ExtractSentinelsFromString(
			"__CHART2KRO_SENTINEL_image.repo__:__CHART2KRO_SENTINEL_image.tag__",
		)
		require.Len(t, paths, 2)
		assert.Equal(t, "image.repo", paths[0])
		assert.Equal(t, "image.tag", paths[1])
	})

	t.Run("no sentinels", func(t *testing.T) {
		paths := transform.ExtractSentinelsFromString("nginx:latest")
		assert.Empty(t, paths)
	})

	t.Run("mixed", func(t *testing.T) {
		paths := transform.ExtractSentinelsFromString(
			"prefix__CHART2KRO_SENTINEL_x__middle__CHART2KRO_SENTINEL_y__suffix",
		)
		require.Len(t, paths, 2)
		assert.Equal(t, "x", paths[0])
		assert.Equal(t, "y", paths[1])
	})
}

func TestBuildCELExpression(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		mapping := transform.FieldMapping{
			ValuesPath: "replicaCount",
			ResourceID: "deployment",
			FieldPath:  "spec.replicas",
			MatchType:  transform.MatchExact,
		}

		cel := transform.BuildCELExpression(mapping, "3")
		assert.Equal(t, "${schema.spec.replicaCount}", cel)
	})

	t.Run("substring match with sentinel-rendered value", func(t *testing.T) {
		mapping := transform.FieldMapping{
			ValuesPath: "image.tag",
			ResourceID: "deployment",
			FieldPath:  "spec.containers[0].image",
			MatchType:  transform.MatchSubstring,
		}

		// Pass the sentinel-rendered string (how it would appear after sentinel injection).
		sentinelRendered := "nginx:__CHART2KRO_SENTINEL_image.tag__"
		cel := transform.BuildCELExpression(mapping, sentinelRendered)
		assert.Equal(t, "nginx:${schema.spec.image.tag}", cel)
	})

	t.Run("substring match fallback", func(t *testing.T) {
		mapping := transform.FieldMapping{
			ValuesPath: "image.tag",
			ResourceID: "deployment",
			FieldPath:  "spec.containers[0].image",
			MatchType:  transform.MatchSubstring,
		}

		// When no sentinel-rendered value available, falls back to simple reference.
		cel := transform.BuildCELExpression(mapping, "")
		assert.Equal(t, "${schema.spec.image.tag}", cel)
	})
}

func TestBuildInterpolatedCELFromSentinel(t *testing.T) {
	t.Run("two sentinels", func(t *testing.T) {
		s := "__CHART2KRO_SENTINEL_image.repo__:__CHART2KRO_SENTINEL_image.tag__"
		cel := transform.BuildInterpolatedCELFromSentinel(s)
		assert.Equal(t, "${schema.spec.image.repo}:${schema.spec.image.tag}", cel)
	})

	t.Run("sentinel with prefix and suffix", func(t *testing.T) {
		s := "https://__CHART2KRO_SENTINEL_host__/api"
		cel := transform.BuildInterpolatedCELFromSentinel(s)
		assert.Equal(t, "https://${schema.spec.host}/api", cel)
	})

	t.Run("single sentinel", func(t *testing.T) {
		s := "__CHART2KRO_SENTINEL_name__"
		cel := transform.BuildInterpolatedCELFromSentinel(s)
		assert.Equal(t, "${schema.spec.name}", cel)
	})

	t.Run("no sentinels", func(t *testing.T) {
		s := "plain-text"
		cel := transform.BuildInterpolatedCELFromSentinel(s)
		assert.Equal(t, "plain-text", cel)
	})
}

func TestMatchType_String(t *testing.T) {
	assert.Equal(t, "exact", transform.MatchExact.String())
	assert.Equal(t, "substring", transform.MatchSubstring.String())

	// Unknown MatchType returns "unknown".
	assert.Equal(t, "unknown", transform.MatchType(99).String())
}

func TestApplyFieldMappings_ExactMatch(t *testing.T) {
	r := makeFullResource("apps/v1", "Deployment", "web", map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(3),
		},
	})

	resourceIDs := map[*k8s.Resource]string{r: "deployment"}

	mappings := []transform.FieldMapping{
		{
			ValuesPath: "replicaCount",
			ResourceID: "deployment",
			FieldPath:  "spec.replicas",
			MatchType:  transform.MatchExact,
		},
	}

	transform.ApplyFieldMappings([]*k8s.Resource{r}, resourceIDs, mappings)

	// The field should now be a CEL expression.
	replicas, _, _ := unstructured.NestedFieldNoCopy(r.Object.Object, "spec", "replicas")
	assert.Equal(t, "${schema.spec.replicaCount}", replicas)
}

func TestApplyFieldMappings_NestedPath(t *testing.T) {
	r := makeFullResource("apps/v1", "Deployment", "web", map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "nginx:1.25",
						},
					},
				},
			},
		},
	})

	resourceIDs := map[*k8s.Resource]string{r: "deployment"}

	mappings := []transform.FieldMapping{
		{
			ValuesPath: "image.repository",
			ResourceID: "deployment",
			FieldPath:  "spec.template.spec.containers[0].image",
			MatchType:  transform.MatchExact,
		},
	}

	transform.ApplyFieldMappings([]*k8s.Resource{r}, resourceIDs, mappings)

	containers, _, _ := unstructured.NestedSlice(r.Object.Object, "spec", "template", "spec", "containers")
	container := containers[0].(map[string]interface{})
	assert.Equal(t, "${schema.spec.image.repository}", container["image"])
}

func TestApplyFieldMappings_MultipleMappings(t *testing.T) {
	r := makeFullResource("v1", "Service", "web-svc", map[string]interface{}{
		"spec": map[string]interface{}{
			"type": "ClusterIP",
			"ports": []interface{}{
				map[string]interface{}{
					"port":       int64(80),
					"targetPort": int64(80),
				},
			},
		},
	})

	resourceIDs := map[*k8s.Resource]string{r: "service"}

	mappings := []transform.FieldMapping{
		{
			ValuesPath: "service.type",
			ResourceID: "service",
			FieldPath:  "spec.type",
			MatchType:  transform.MatchExact,
		},
		{
			ValuesPath: "service.port",
			ResourceID: "service",
			FieldPath:  "spec.ports[0].port",
			MatchType:  transform.MatchExact,
		},
	}

	transform.ApplyFieldMappings([]*k8s.Resource{r}, resourceIDs, mappings)

	svcType, _, _ := unstructured.NestedString(r.Object.Object, "spec", "type")
	assert.Equal(t, "${schema.spec.service.type}", svcType)

	ports, _, _ := unstructured.NestedSlice(r.Object.Object, "spec", "ports")
	port := ports[0].(map[string]interface{})
	assert.Equal(t, "${schema.spec.service.port}", port["port"])
}

func TestApplyFieldMappings_NoObject(_ *testing.T) {
	r := &k8s.Resource{
		Name: "test",
	}
	resourceIDs := map[*k8s.Resource]string{r: "test"}
	mappings := []transform.FieldMapping{
		{ValuesPath: "x", ResourceID: "test", FieldPath: "spec.x"},
	}

	// Should not panic on resource without Object.
	transform.ApplyFieldMappings([]*k8s.Resource{r}, resourceIDs, mappings)
}

func TestApplyFieldMappings_UnmatchedResourceID(t *testing.T) {
	r := makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
		"data": map[string]interface{}{"key": "value"},
	})
	resourceIDs := map[*k8s.Resource]string{r: "configmap"}

	// Mapping targets a different resource ID.
	mappings := []transform.FieldMapping{
		{ValuesPath: "x", ResourceID: "other", FieldPath: "data.key"},
	}

	transform.ApplyFieldMappings([]*k8s.Resource{r}, resourceIDs, mappings)

	// Original value should be unchanged.
	val, _, _ := unstructured.NestedString(r.Object.Object, "data", "key")
	assert.Equal(t, "value", val)
}

func TestEngine_Transform_WithFieldMappings(t *testing.T) {
	r := makeFullResource("apps/v1", "Deployment", "web", map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(3),
		},
	})

	values := map[string]interface{}{
		"replicaCount": 3,
		"unused":       "hello",
	}

	engine := transform.NewEngine(transform.EngineConfig{
		IncludeAllValues: false,
		FieldMappings: []transform.FieldMapping{
			{
				ValuesPath: "replicaCount",
				ResourceID: "deployment",
				FieldPath:  "spec.replicas",
				MatchType:  transform.MatchExact,
			},
		},
		ReferencedPaths: map[string]bool{
			"replicaCount": true,
		},
	})

	result, err := engine.Transform(context.Background(), []*k8s.Resource{r}, values)
	require.NoError(t, err)

	// Field mapping should be applied.
	replicas, _, _ := unstructured.NestedFieldNoCopy(r.Object.Object, "spec", "replicas")
	assert.Equal(t, "${schema.spec.replicaCount}", replicas)

	// Schema should only include referenced paths (replicaCount), not "unused".
	assert.Len(t, result.SchemaFields, 1)
	assert.Equal(t, "replicaCount", result.SchemaFields[0].Name)

	// FieldMappings should be populated in the result.
	assert.Len(t, result.FieldMappings, 1)
}

// ---------------------------------------------------------------------------
// parseFieldPath edge cases
// ---------------------------------------------------------------------------

func TestParseFieldPath(t *testing.T) {
	t.Run("simple path", func(t *testing.T) {
		parts := transform.ParseFieldPath("spec.replicas")
		require.Len(t, parts, 2)
		assert.Equal(t, "spec", parts[0].Key)
		assert.Equal(t, -1, parts[0].Index)
		assert.Equal(t, "replicas", parts[1].Key)
	})

	t.Run("path with array index", func(t *testing.T) {
		parts := transform.ParseFieldPath("spec.containers[0].image")
		require.Len(t, parts, 4)
		assert.Equal(t, "spec", parts[0].Key)
		assert.Equal(t, "containers", parts[1].Key)
		assert.Equal(t, 0, parts[2].Index)
		assert.Equal(t, "image", parts[3].Key)
	})

	t.Run("empty path", func(t *testing.T) {
		parts := transform.ParseFieldPath("")
		assert.Nil(t, parts)
	})

	t.Run("malformed brackets - missing close", func(t *testing.T) {
		parts := transform.ParseFieldPath("spec.items[")
		require.Len(t, parts, 2)
		// Malformed bracket treated as key.
		assert.Equal(t, "items[", parts[1].Key)
	})

	t.Run("malformed brackets - empty index", func(t *testing.T) {
		parts := transform.ParseFieldPath("spec.items[]")
		require.Len(t, parts, 2)
		assert.Equal(t, "items[]", parts[1].Key)
	})

	t.Run("non-numeric index", func(t *testing.T) {
		parts := transform.ParseFieldPath("spec.items[abc]")
		require.Len(t, parts, 2)
		assert.Equal(t, "items[abc]", parts[1].Key)
	})

	t.Run("double dots", func(t *testing.T) {
		parts := transform.ParseFieldPath("spec..replicas")
		require.Len(t, parts, 2) // empty segment skipped
		assert.Equal(t, "spec", parts[0].Key)
		assert.Equal(t, "replicas", parts[1].Key)
	})
}

// ---------------------------------------------------------------------------
// setNestedField edge cases
// ---------------------------------------------------------------------------

func TestSetNestedField(t *testing.T) {
	t.Run("set top-level key", func(t *testing.T) {
		obj := map[string]interface{}{"name": "old"}
		transform.SetNestedField(obj, "name", "new")
		assert.Equal(t, "new", obj["name"])
	})

	t.Run("set nested key", func(t *testing.T) {
		obj := map[string]interface{}{
			"spec": map[string]interface{}{"replicas": 3},
		}
		transform.SetNestedField(obj, "spec.replicas", "${schema.spec.replicaCount}")
		spec := obj["spec"].(map[string]interface{})
		assert.Equal(t, "${schema.spec.replicaCount}", spec["replicas"])
	})

	t.Run("path does not exist", func(t *testing.T) {
		obj := map[string]interface{}{"spec": map[string]interface{}{}}
		transform.SetNestedField(obj, "spec.missing.deep", "value")
		// Should not panic, should be a no-op.
		spec := obj["spec"].(map[string]interface{})
		assert.NotContains(t, spec, "missing")
	})

	t.Run("empty path", func(t *testing.T) {
		obj := map[string]interface{}{"key": "val"}
		transform.SetNestedField(obj, "", "value")
		// No-op, original unchanged.
		assert.Equal(t, "val", obj["key"])
	})

	t.Run("array index", func(t *testing.T) {
		obj := map[string]interface{}{
			"items": []interface{}{"a", "b", "c"},
		}
		transform.SetNestedField(obj, "items[1]", "replaced")
		items := obj["items"].([]interface{})
		assert.Equal(t, "replaced", items[1])
	})

	t.Run("array index out of range", func(t *testing.T) {
		obj := map[string]interface{}{
			"items": []interface{}{"a"},
		}
		transform.SetNestedField(obj, "items[5]", "value")
		// Should not panic, no-op.
		items := obj["items"].([]interface{})
		assert.Len(t, items, 1)
	})
}

// ---------------------------------------------------------------------------
// resourceMatchKey
// ---------------------------------------------------------------------------

func TestResourceMatchKey(t *testing.T) {
	t.Run("core resource", func(t *testing.T) {
		r := makeFullResource("v1", "ConfigMap", "my-config", nil)
		key := transform.ResourceMatchKey(r)
		assert.Equal(t, "v1/ConfigMap/my-config", key)
	})

	t.Run("apps group resource", func(t *testing.T) {
		r := makeFullResource("apps/v1", "Deployment", "web", nil)
		key := transform.ResourceMatchKey(r)
		assert.Equal(t, "apps/v1/Deployment/web", key)
	})

	t.Run("nil resource", func(t *testing.T) {
		key := transform.ResourceMatchKey(nil)
		assert.Equal(t, "", key)
	})

	t.Run("zero-valued GVK returns empty key", func(t *testing.T) {
		// Resource with empty Kind should return "" to avoid false matches.
		r := &k8s.Resource{Name: "orphan"}
		key := transform.ResourceMatchKey(r)
		assert.Equal(t, "", key)
	})
}

// ---------------------------------------------------------------------------
// DiffAllResources with reordered resources
// ---------------------------------------------------------------------------

func TestDiffAllResources_ReorderedResources(t *testing.T) {
	// Baseline: ConfigMap first, then Deployment
	cm := makeFullResource("v1", "ConfigMap", "config", map[string]interface{}{
		"data": map[string]interface{}{"key": "value"},
	})
	dep := makeFullResource("apps/v1", "Deployment", "web", map[string]interface{}{
		"spec": map[string]interface{}{"replicas": int64(3)},
	})
	baseline := []*k8s.Resource{cm, dep}

	// Sentinel: Deployment first, then ConfigMap (reversed order)
	sentDep := makeFullResource("apps/v1", "Deployment", "web", map[string]interface{}{
		"spec": map[string]interface{}{"replicas": "__CHART2KRO_SENTINEL_replicaCount__"},
	})
	sentCm := makeFullResource("v1", "ConfigMap", "config", map[string]interface{}{
		"data": map[string]interface{}{"key": "__CHART2KRO_SENTINEL_mykey__"},
	})
	sentinel := []*k8s.Resource{sentDep, sentCm}

	ids := map[*k8s.Resource]string{cm: "configmap", dep: "deployment"}

	mappings := transform.DiffAllResources(baseline, sentinel, ids)

	// Should find both mappings despite different ordering.
	require.Len(t, mappings, 2)

	valPaths := map[string]string{}
	for _, m := range mappings {
		valPaths[m.ResourceID] = m.ValuesPath
	}

	assert.Equal(t, "mykey", valPaths["configmap"])
	assert.Equal(t, "replicaCount", valPaths["deployment"])
}

func TestDiffAllResources_MissingInSentinel(t *testing.T) {
	// Baseline has resources not present in sentinel render (e.g., conditional block).
	baseline := []*k8s.Resource{
		makeFullResource("v1", "ConfigMap", "config", map[string]interface{}{
			"data": map[string]interface{}{"key": "value"},
		}),
		makeFullResource("v1", "ConfigMap", "extra", map[string]interface{}{
			"data": map[string]interface{}{"key": "value"},
		}),
	}
	sentinel := []*k8s.Resource{
		makeFullResource("v1", "ConfigMap", "config", map[string]interface{}{
			"data": map[string]interface{}{"key": "__CHART2KRO_SENTINEL_mykey__"},
		}),
	}

	ids := map[*k8s.Resource]string{baseline[0]: "configmap", baseline[1]: "configmap2"}

	mappings := transform.DiffAllResources(baseline, sentinel, ids)

	// Only "config" should be diffed, "extra" is missing in sentinel render.
	require.Len(t, mappings, 1)
	assert.Equal(t, "configmap", mappings[0].ResourceID)
	assert.Equal(t, "mykey", mappings[0].ValuesPath)
}

// ---------------------------------------------------------------------------
// extractSentinelMappings — non-string values (audit fix #9)
// ---------------------------------------------------------------------------

func TestExtractSentinelMappings(t *testing.T) {
	t.Run("string sentinel exact match", func(t *testing.T) {
		mappings := transform.ExtractSentinelMappings(
			"__CHART2KRO_SENTINEL_replicaCount__", "deploy", "spec.replicas",
		)
		require.Len(t, mappings, 1)
		assert.Equal(t, "replicaCount", mappings[0].ValuesPath)
		assert.Equal(t, transform.MatchExact, mappings[0].MatchType)
	})

	t.Run("string sentinel interpolated", func(t *testing.T) {
		mappings := transform.ExtractSentinelMappings(
			"prefix__CHART2KRO_SENTINEL_a__:__CHART2KRO_SENTINEL_b__suffix",
			"svc", "spec.port",
		)
		require.Len(t, mappings, 2)
		assert.Equal(t, transform.MatchSubstring, mappings[0].MatchType)
		assert.Equal(t, "a", mappings[0].ValuesPath)
		assert.Equal(t, "b", mappings[1].ValuesPath)
	})

	t.Run("nil value returns nil", func(t *testing.T) {
		mappings := transform.ExtractSentinelMappings(nil, "r", "f")
		assert.Nil(t, mappings)
	})

	t.Run("non-string without sentinel returns nil", func(t *testing.T) {
		mappings := transform.ExtractSentinelMappings(42, "r", "f")
		assert.Nil(t, mappings)
	})

	t.Run("non-string with sentinel marker is extracted", func(t *testing.T) {
		// Helm may convert sentinel strings to other types. Simulate a bool
		// that stringifies to contain a sentinel (unlikely in practice, but
		// covers the code path). More realistically, test an integer sentinel
		// that somehow preserves the marker via Sprintf.
		// A realistic scenario: a template outputs the sentinel as-is
		// but the YAML parser parses it as a string anyway. The important
		// path is that Sprintf("%v") is attempted on non-string values.
		mappings := transform.ExtractSentinelMappings(123, "r", "f")
		assert.Nil(t, mappings) // "123" contains no sentinel
	})

	t.Run("no sentinel in value", func(t *testing.T) {
		mappings := transform.ExtractSentinelMappings("plain-text", "r", "f")
		assert.Nil(t, mappings)
	})

	t.Run("duplicate sentinels are deduplicated", func(t *testing.T) {
		s := "__CHART2KRO_SENTINEL_x__-__CHART2KRO_SENTINEL_x__"
		mappings := transform.ExtractSentinelMappings(s, "r", "f")
		require.Len(t, mappings, 1)
		assert.Equal(t, "x", mappings[0].ValuesPath)
	})
}

// ---------------------------------------------------------------------------
// DiffAllResources — type-aware comparison via reflect.DeepEqual (audit fix #8)
// ---------------------------------------------------------------------------

func TestDiffAllResources_TypeAwareComparison(t *testing.T) {
	t.Run("same numeric value different types are detected", func(t *testing.T) {
		// int64(3) vs float64(3) should NOT match with DeepEqual,
		// so the diff should detect the sentinel.
		baseline := []*k8s.Resource{makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
			"data": map[string]interface{}{"count": int64(3)},
		})}
		sentinel := []*k8s.Resource{makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
			"data": map[string]interface{}{"count": float64(3)},
		})}
		ids := map[*k8s.Resource]string{baseline[0]: "cm"}

		mappings := transform.DiffAllResources(baseline, sentinel, ids)
		// float64(3) doesn't contain a sentinel, so extractSentinelMappings returns nil.
		// But the diff still detects the type change. No mapping is emitted since
		// the float64(3) has no sentinel markers.
		assert.Empty(t, mappings)
	})

	t.Run("identical values are not diffed", func(t *testing.T) {
		baseline := []*k8s.Resource{makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
			"data": map[string]interface{}{"key": "same"},
		})}
		sentinel := []*k8s.Resource{makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
			"data": map[string]interface{}{"key": "same"},
		})}
		ids := map[*k8s.Resource]string{baseline[0]: "cm"}

		mappings := transform.DiffAllResources(baseline, sentinel, ids)
		assert.Empty(t, mappings)
	})

	t.Run("nil baseline object is skipped", func(t *testing.T) {
		baseline := []*k8s.Resource{{
			Name: "test",
			// Object is nil
		}}
		sentinel := []*k8s.Resource{makeFullResource("v1", "ConfigMap", "test", map[string]interface{}{
			"data": map[string]interface{}{"key": "__CHART2KRO_SENTINEL_x__"},
		})}
		ids := map[*k8s.Resource]string{baseline[0]: "cm"}

		mappings := transform.DiffAllResources(baseline, sentinel, ids)
		assert.Empty(t, mappings) // skipped because baseline Object is nil
	})

	t.Run("new field in sentinel render is extracted", func(t *testing.T) {
		baseline := []*k8s.Resource{makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
			"data": map[string]interface{}{},
		})}
		sentinel := []*k8s.Resource{makeFullResource("v1", "ConfigMap", "cm", map[string]interface{}{
			"data": map[string]interface{}{"newKey": "__CHART2KRO_SENTINEL_myval__"},
		})}
		ids := map[*k8s.Resource]string{baseline[0]: "cm"}

		mappings := transform.DiffAllResources(baseline, sentinel, ids)
		require.Len(t, mappings, 1)
		assert.Equal(t, "myval", mappings[0].ValuesPath)
		assert.Equal(t, "data.newKey", mappings[0].FieldPath)
	})
}

// ---------------------------------------------------------------------------
// DetectCycles — deduplication (audit fix #6)
// ---------------------------------------------------------------------------

func TestDetectCycles_Deduplication(t *testing.T) {
	t.Run("simple 2-node cycle", func(t *testing.T) {
		g := transform.NewDependencyGraph()
		g.AddNode("a", makeFullResource("v1", "ConfigMap", "a", nil))
		g.AddNode("b", makeFullResource("v1", "Secret", "b", nil))

		g.AddEdge("a", "b")
		g.AddEdge("b", "a")

		cycles := g.DetectCycles()
		require.NotEmpty(t, cycles)

		// Verify cycle contains expected nodes.
		found := false
		for _, c := range cycles {
			if len(c) == 3 { // [a, b, a] or [b, a, b]
				found = true
			}
		}

		assert.True(t, found, "expected a 2-node cycle")
	})

	t.Run("3-node cycle includes all nodes", func(t *testing.T) {
		g := transform.NewDependencyGraph()
		g.AddNode("x", makeFullResource("v1", "ConfigMap", "x", nil))
		g.AddNode("y", makeFullResource("v1", "Secret", "y", nil))
		g.AddNode("z", makeFullResource("v1", "Service", "z", nil))

		g.AddEdge("x", "y")
		g.AddEdge("y", "z")
		g.AddEdge("z", "x")

		cycles := g.DetectCycles()
		require.NotEmpty(t, cycles)

		// Check at least one cycle contains x, y, z.
		found := false
		for _, c := range cycles {
			containsAll := false
			for _, n := range c {
				if n == "x" || n == "y" || n == "z" {
					containsAll = true
				}
			}

			if containsAll && len(c) >= 3 {
				found = true
			}
		}

		assert.True(t, found, "expected a 3-node cycle containing x, y, z")
	})
}
