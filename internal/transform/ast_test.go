package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// ---------------------------------------------------------------------------
// AnalyzeTemplates
// ---------------------------------------------------------------------------

func TestAnalyzeTemplates(t *testing.T) {
	t.Run("simple Values references", func(t *testing.T) {
		templates := map[string]string{
			"deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-{{ .Chart.Name }}
spec:
  replicas: {{ .Values.replicaCount }}
  template:
    spec:
      containers:
        - image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          ports:
            - containerPort: {{ .Values.service.port }}`,
		}

		refs, err := AnalyzeTemplates(templates)
		require.NoError(t, err)
		assert.True(t, refs["replicaCount"])
		assert.True(t, refs["image.repository"])
		assert.True(t, refs["image.tag"])
		assert.True(t, refs["service.port"])
		assert.False(t, refs["Release.Name"]) // Not a .Values ref.
	})

	t.Run("conditional references", func(t *testing.T) {
		templates := map[string]string{
			"svc.yaml": `{{ if .Values.service.enabled }}
apiVersion: v1
kind: Service
spec:
  type: {{ .Values.service.type }}
{{ end }}`,
		}

		refs, err := AnalyzeTemplates(templates)
		require.NoError(t, err)
		assert.True(t, refs["service.enabled"])
		assert.True(t, refs["service.type"])
	})

	t.Run("range references", func(t *testing.T) {
		templates := map[string]string{
			"env.yaml": `{{ range .Values.env }}
- name: {{ .name }}
{{ end }}`,
		}

		refs, err := AnalyzeTemplates(templates)
		require.NoError(t, err)
		assert.True(t, refs["env"])
	})

	t.Run("with block references", func(t *testing.T) {
		templates := map[string]string{
			"cfg.yaml": `{{ with .Values.config }}
data:
  key: {{ .key }}
{{ end }}`,
		}

		refs, err := AnalyzeTemplates(templates)
		require.NoError(t, err)
		assert.True(t, refs["config"])
	})

	t.Run("skips non-template files", func(t *testing.T) {
		templates := map[string]string{
			"NOTES.txt":     `{{ .Values.shouldBeIgnored }}`,
			"template.yaml": `{{ .Values.included }}`,
		}

		refs, err := AnalyzeTemplates(templates)
		require.NoError(t, err)
		assert.False(t, refs["shouldBeIgnored"])
		assert.True(t, refs["included"])
	})

	t.Run("empty input", func(t *testing.T) {
		refs, err := AnalyzeTemplates(nil)
		require.NoError(t, err)
		assert.Empty(t, refs)
	})

	t.Run("invalid template is skipped", func(t *testing.T) {
		templates := map[string]string{
			"bad.yaml":  `{{ invalid syntax`,
			"good.yaml": `{{ .Values.valid }}`,
		}

		refs, err := AnalyzeTemplates(templates)
		require.NoError(t, err)
		assert.True(t, refs["valid"])
	})

	t.Run("tpl extension is parsed", func(t *testing.T) {
		templates := map[string]string{
			"_helpers.tpl": `{{- define "name" -}}{{ .Values.nameOverride }}{{- end -}}`,
		}

		refs, err := AnalyzeTemplates(templates)
		require.NoError(t, err)
		assert.True(t, refs["nameOverride"])
	})

	t.Run("template include references", func(t *testing.T) {
		templates := map[string]string{
			"deploy.yaml": `{{ template "myapp.labels" .Values.labels }}`,
		}

		refs, err := AnalyzeTemplates(templates)
		require.NoError(t, err)
		assert.True(t, refs["labels"])
	})
}

// ---------------------------------------------------------------------------
// extractValuesRefs
// ---------------------------------------------------------------------------

func TestExtractValuesRefs(t *testing.T) {
	t.Run("nested Values path", func(t *testing.T) {
		content := `{{ .Values.deeply.nested.value }}`
		paths, err := extractValuesRefs("test.yaml", content)
		require.NoError(t, err)
		assert.Contains(t, paths, "deeply.nested.value")
	})

	t.Run("no Values references", func(t *testing.T) {
		content := `{{ .Release.Name }}{{ .Chart.Name }}`
		paths, err := extractValuesRefs("test.yaml", content)
		require.NoError(t, err)
		assert.Empty(t, paths)
	})

	t.Run("multiple references same path", func(t *testing.T) {
		content := `{{ .Values.name }}{{ .Values.name }}`
		paths, err := extractValuesRefs("test.yaml", content)
		require.NoError(t, err)
		assert.Len(t, paths, 2) // Both occurrences found.
	})

	t.Run("parse error", func(t *testing.T) {
		_, err := extractValuesRefs("bad.yaml", `{{ broken`)
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// isTemplateFile
// ---------------------------------------------------------------------------

func TestIsTemplateFile(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"deployment.yaml", true},
		{"service.yml", true},
		{"_helpers.tpl", true},
		{"NOTES.txt", false},
		{"README.md", false},
		{"chart.json", false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, isTemplateFile(tt.name), tt.name)
	}
}

// ---------------------------------------------------------------------------
// MatchFieldsByValue
// ---------------------------------------------------------------------------

func TestMatchFieldsByValue(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		resource := &k8s.Resource{
			Object: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"spec": map[string]interface{}{
					"type": "ClusterIP",
				},
			}},
			GVK:  schema.GroupVersionKind{Version: "v1", Kind: "Service"},
			Name: "test-svc",
		}

		ids := map[*k8s.Resource]string{resource: "service"}

		values := map[string]interface{}{
			"service": map[string]interface{}{
				"type": "ClusterIP",
			},
		}

		refs := map[string]bool{"service.type": true}

		mappings := MatchFieldsByValue([]*k8s.Resource{resource}, ids, values, refs)
		require.NotEmpty(t, mappings)

		found := false
		for _, m := range mappings {
			if m.ValuesPath == "service.type" && m.FieldPath == "spec.type" {
				found = true
				assert.Equal(t, MatchExact, m.MatchType)
			}
		}
		assert.True(t, found, "expected field mapping for service.type")
	})

	t.Run("substring match", func(t *testing.T) {
		resource := &k8s.Resource{
			Object: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"image": "nginx:1.21",
								},
							},
						},
					},
				},
			}},
			GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Name: "test-deploy",
		}

		ids := map[*k8s.Resource]string{resource: "deployment"}

		values := map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "nginx",
				"tag":        "1.21",
			},
		}

		refs := map[string]bool{"image.repository": true, "image.tag": true}

		mappings := MatchFieldsByValue([]*k8s.Resource{resource}, ids, values, refs)

		// Should find substring matches for the image field.
		var imageMatchCount int
		for _, m := range mappings {
			if m.FieldPath == "spec.template.spec.containers[0].image" {
				imageMatchCount++
				assert.Equal(t, MatchSubstring, m.MatchType)
			}
		}
		assert.Equal(t, 2, imageMatchCount, "expected 2 substring matches for image field (repository + tag)")
	})

	t.Run("no match for unreferenced paths", func(t *testing.T) {
		resource := &k8s.Resource{
			Object: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"spec": map[string]interface{}{
					"type": "ClusterIP",
				},
			}},
			GVK:  schema.GroupVersionKind{Version: "v1", Kind: "Service"},
			Name: "test-svc",
		}

		ids := map[*k8s.Resource]string{resource: "service"}

		values := map[string]interface{}{
			"service": map[string]interface{}{
				"type": "ClusterIP",
			},
		}

		// Empty referenced paths — nothing should match.
		refs := map[string]bool{}

		mappings := MatchFieldsByValue([]*k8s.Resource{resource}, ids, values, refs)
		assert.Empty(t, mappings)
	})

	t.Run("nil resources", func(t *testing.T) {
		mappings := MatchFieldsByValue(nil, nil, nil, nil)
		assert.Empty(t, mappings)
	})

	t.Run("integer value match", func(t *testing.T) {
		resource := &k8s.Resource{
			Object: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"spec": map[string]interface{}{
					"replicas": int64(3),
				},
			}},
			GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Name: "test-deploy",
		}

		ids := map[*k8s.Resource]string{resource: "deployment"}

		values := map[string]interface{}{
			"replicaCount": int64(3),
		}

		refs := map[string]bool{"replicaCount": true}

		mappings := MatchFieldsByValue([]*k8s.Resource{resource}, ids, values, refs)

		found := false
		for _, m := range mappings {
			if m.ValuesPath == "replicaCount" && m.FieldPath == "spec.replicas" {
				found = true
				assert.Equal(t, MatchExact, m.MatchType)
			}
		}
		assert.True(t, found, "expected field mapping for replicaCount")
	})
}

// ---------------------------------------------------------------------------
// flattenValues
// ---------------------------------------------------------------------------

func TestFlattenValues(t *testing.T) {
	t.Run("nested values", func(t *testing.T) {
		values := map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "nginx",
				"tag":        "1.21",
			},
			"replicaCount": 3,
		}

		refs := map[string]bool{
			"image.repository": true,
			"image.tag":        true,
			"replicaCount":     true,
		}

		flat := flattenValues(values, "", refs)
		assert.Equal(t, "nginx", flat["image.repository"])
		assert.Equal(t, "1.21", flat["image.tag"])
		assert.Equal(t, 3, flat["replicaCount"])
	})

	t.Run("filters by referenced paths", func(t *testing.T) {
		values := map[string]interface{}{
			"included": "yes",
			"excluded": "no",
		}

		refs := map[string]bool{"included": true}

		flat := flattenValues(values, "", refs)
		assert.Contains(t, flat, "included")
		assert.NotContains(t, flat, "excluded")
	})
}

// ---------------------------------------------------------------------------
// buildFastModeSentinel
// ---------------------------------------------------------------------------

func TestBuildFastModeSentinel(t *testing.T) {
	t.Run("single substring replacement", func(t *testing.T) {
		stringIndex := map[string][]string{
			"nginx": {"image.repository"},
		}

		result := buildFastModeSentinel("nginx:latest", stringIndex)
		assert.Contains(t, result, SentinelPrefix+"image.repository"+SentinelSuffix)
		assert.Contains(t, result, ":latest")
	})

	t.Run("multiple substring replacements", func(t *testing.T) {
		stringIndex := map[string][]string{
			"nginx": {"image.repository"},
			"1.21":  {"image.tag"},
		}

		result := buildFastModeSentinel("nginx:1.21", stringIndex)
		assert.Contains(t, result, SentinelPrefix+"image.repository"+SentinelSuffix)
		assert.Contains(t, result, SentinelPrefix+"image.tag"+SentinelSuffix)
	})

	t.Run("skips short values", func(t *testing.T) {
		stringIndex := map[string][]string{
			"a": {"short.val"},
		}

		result := buildFastModeSentinel("abc", stringIndex)
		assert.Equal(t, "abc", result) // "a" is too short, not replaced.
	})
}
