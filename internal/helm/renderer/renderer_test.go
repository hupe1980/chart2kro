package renderer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
)

func newTestChart(name, version string) *chart.Chart {
	return &chart.Chart{
		Metadata: &chart.Metadata{
			Name:       name,
			Version:    version,
			APIVersion: "v2",
			Type:       "application",
		},
		Values: map[string]interface{}{
			"replicaCount": 1,
			"image": map[string]interface{}{
				"repository": "nginx",
				"tag":        "latest",
			},
		},
		Templates: []*chart.File{
			{
				Name: "templates/deployment.yaml",
				Data: []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: {{ .Release.Name }}-{{ .Chart.Name }}\n  namespace: {{ .Release.Namespace }}\nspec:\n  replicas: {{ .Values.replicaCount }}\n  template:\n    spec:\n      containers:\n      - name: {{ .Chart.Name }}\n        image: \"{{ .Values.image.repository }}:{{ .Values.image.tag }}\"\n"),
			},
			{
				Name: "templates/service.yaml",
				Data: []byte("apiVersion: v1\nkind: Service\nmetadata:\n  name: {{ .Release.Name }}-{{ .Chart.Name }}\n  namespace: {{ .Release.Namespace }}\nspec:\n  type: ClusterIP\n  ports:\n  - port: 80\n"),
			},
		},
	}
}

func TestHelmRenderer_Render(t *testing.T) {
	ch := newTestChart("test-app", "1.0.0")
	r := New(DefaultRenderOptions())

	out, err := r.Render(context.Background(), ch, ch.Values)
	require.NoError(t, err)

	yaml := string(out)
	assert.Contains(t, yaml, "kind: Deployment")
	assert.Contains(t, yaml, "kind: Service")
	assert.Contains(t, yaml, "name: release-test-app")
	assert.Contains(t, yaml, "namespace: default")
	assert.Contains(t, yaml, "replicas: 1")
	assert.Contains(t, yaml, "image: \"nginx:latest\"")
}

func TestHelmRenderer_Render_CustomReleaseName(t *testing.T) {
	ch := newTestChart("myapp", "0.1.0")
	r := New(RenderOptions{
		ReleaseName: "my-release",
		Namespace:   "production",
	})

	out, err := r.Render(context.Background(), ch, ch.Values)
	require.NoError(t, err)

	yaml := string(out)
	assert.Contains(t, yaml, "name: my-release-myapp")
	assert.Contains(t, yaml, "namespace: production")
}

func TestHelmRenderer_Render_WithValuesOverride(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	r := New(DefaultRenderOptions())

	vals := map[string]interface{}{
		"replicaCount": 3,
		"image": map[string]interface{}{
			"repository": "custom-image",
			"tag":        "v2.0",
		},
	}

	out, err := r.Render(context.Background(), ch, vals)
	require.NoError(t, err)

	yaml := string(out)
	assert.Contains(t, yaml, "replicas: 3")
	assert.Contains(t, yaml, "image: \"custom-image:v2.0\"")
}

func TestHelmRenderer_Render_CancelledContext(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	r := New(DefaultRenderOptions())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Render(ctx, ch, ch.Values)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
}

func TestHelmRenderer_Render_DeadlineContext(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	r := New(DefaultRenderOptions())

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	_, err := r.Render(ctx, ch, ch.Values)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
}

func TestHelmRenderer_Render_StrictMode(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name: "strict-chart", Version: "1.0.0",
			APIVersion: "v2", Type: "application",
		},
		Values:    map[string]interface{}{},
		Templates: []*chart.File{{Name: "templates/test.yaml", Data: []byte("value: {{ .Values.required }}")}},
	}

	r := New(RenderOptions{ReleaseName: "release", Namespace: "default", Strict: true})
	_, err := r.Render(context.Background(), ch, ch.Values)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rendering templates")
}

func TestHelmRenderer_Render_SkipsNotes(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name: "notes-chart", Version: "1.0.0",
			APIVersion: "v2", Type: "application",
		},
		Values: map[string]interface{}{},
		Templates: []*chart.File{
			{Name: "templates/deployment.yaml", Data: []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: test\n")},
			{Name: "templates/NOTES.txt", Data: []byte("Thank you for installing the chart!")},
		},
	}

	r := New(DefaultRenderOptions())
	out, err := r.Render(context.Background(), ch, ch.Values)
	require.NoError(t, err)
	assert.Contains(t, string(out), "kind: Deployment")
	assert.NotContains(t, string(out), "Thank you")
}

func TestHelmRenderer_Render_SkipsEmptyTemplates(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name: "empty-chart", Version: "1.0.0",
			APIVersion: "v2", Type: "application",
		},
		Values: map[string]interface{}{"enabled": false},
		Templates: []*chart.File{
			{Name: "templates/deployment.yaml", Data: []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: test\n")},
			{Name: "templates/optional.yaml", Data: []byte("{{- if .Values.enabled }}\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: optional\n{{- end }}")},
		},
	}

	r := New(DefaultRenderOptions())
	out, err := r.Render(context.Background(), ch, ch.Values)
	require.NoError(t, err)
	assert.Contains(t, string(out), "kind: Deployment")
	assert.NotContains(t, string(out), "ConfigMap")
}

func TestDefaultRenderOptions(t *testing.T) {
	opts := DefaultRenderOptions()
	assert.Equal(t, "release", opts.ReleaseName)
	assert.Equal(t, "default", opts.Namespace)
	assert.False(t, opts.Strict)
}

func TestNew_FillsDefaults(t *testing.T) {
	r := New(RenderOptions{})
	assert.Equal(t, "release", r.opts.ReleaseName)
	assert.Equal(t, "default", r.opts.Namespace)
}

func TestCombineManifests_DeterministicOrder(t *testing.T) {
	rendered := map[string]string{
		"chart/templates/b.yaml": "kind: B\nmetadata:\n  name: b",
		"chart/templates/a.yaml": "kind: A\nmetadata:\n  name: a",
	}

	out := combineManifests(rendered)
	yaml := string(out)
	aPos := strings.Index(yaml, "kind: A")
	bPos := strings.Index(yaml, "kind: B")
	assert.True(t, aPos < bPos, "a should come before b in output")
	assert.Contains(t, yaml, "---\n")
}

func TestMergeValues_Defaults(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	vals, err := MergeValues(ch, ValuesOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, vals["replicaCount"])
}

func TestMergeValues_SingleValuesFile(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	dir := t.TempDir()
	vf := filepath.Join(dir, "custom.yaml")
	require.NoError(t, os.WriteFile(vf, []byte("replicaCount: 5\n"), 0o600))

	vals, err := MergeValues(ch, ValuesOptions{ValueFiles: []string{vf}})
	require.NoError(t, err)
	assert.EqualValues(t, 5, vals["replicaCount"])
}

func TestMergeValues_MultipleValuesFiles_LastWins(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	dir := t.TempDir()
	vf1 := filepath.Join(dir, "v1.yaml")
	vf2 := filepath.Join(dir, "v2.yaml")
	require.NoError(t, os.WriteFile(vf1, []byte("replicaCount: 3\n"), 0o600))
	require.NoError(t, os.WriteFile(vf2, []byte("replicaCount: 7\n"), 0o600))

	vals, err := MergeValues(ch, ValuesOptions{ValueFiles: []string{vf1, vf2}})
	require.NoError(t, err)
	assert.EqualValues(t, 7, vals["replicaCount"])
}

func TestMergeValues_SetOverride(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	vals, err := MergeValues(ch, ValuesOptions{Values: []string{"replicaCount=10"}})
	require.NoError(t, err)
	assert.Equal(t, int64(10), vals["replicaCount"])
}

func TestMergeValues_SetStringOverride(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	vals, err := MergeValues(ch, ValuesOptions{StringValues: []string{"image.tag=v3.0"}})
	require.NoError(t, err)
	img := vals["image"].(map[string]interface{})
	assert.Equal(t, "v3.0", img["tag"])
}

func TestMergeValues_SetFileOverride(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	dir := t.TempDir()
	dataFile := filepath.Join(dir, "config.txt")
	require.NoError(t, os.WriteFile(dataFile, []byte("some-config-data"), 0o600))

	vals, err := MergeValues(ch, ValuesOptions{FileValues: []string{"configData=" + dataFile}})
	require.NoError(t, err)
	assert.Equal(t, "some-config-data", vals["configData"])
}

func TestMergeValues_ValuesFileNotFound(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	_, err := MergeValues(ch, ValuesOptions{ValueFiles: []string{"/nonexistent/values.yaml"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading values file")
}

func TestMergeValues_InvalidSetSyntax(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	_, err := MergeValues(ch, ValuesOptions{Values: []string{"invalid[bracket"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing --set")
}

func TestMergeValues_SetFileInvalidFormat(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	_, err := MergeValues(ch, ValuesOptions{FileValues: []string{"no-equals-sign"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --set-file format")
}

func TestMergeValues_SetFileNotFound(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	_, err := MergeValues(ch, ValuesOptions{FileValues: []string{"key=/nonexistent/file"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading --set-file")
}

func TestMergeValues_Combined(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")
	dir := t.TempDir()
	vf := filepath.Join(dir, "values.yaml")
	require.NoError(t, os.WriteFile(vf, []byte("replicaCount: 3\n"), 0o600))

	vals, err := MergeValues(ch, ValuesOptions{
		ValueFiles:   []string{vf},
		Values:       []string{"image.tag=v2.0"},
		StringValues: []string{"image.repository=custom"},
	})
	require.NoError(t, err)
	assert.EqualValues(t, 3, vals["replicaCount"])
	img := vals["image"].(map[string]interface{})
	assert.Equal(t, "v2.0", img["tag"])
	assert.Equal(t, "custom", img["repository"])
}

func TestMergeValues_EmptyChartValues(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{Name: "empty", Version: "1.0.0", APIVersion: "v2"},
		Values:   nil,
	}
	vals, err := MergeValues(ch, ValuesOptions{Values: []string{"key=value"}})
	require.NoError(t, err)
	assert.Equal(t, "value", vals["key"])
}

func TestMergeValues_DoesNotMutateChartValues(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")

	// Record original state.
	originalReplicas := ch.Values["replicaCount"]

	// Merge with --set overrides.
	_, err := MergeValues(ch, ValuesOptions{Values: []string{"replicaCount=99"}})
	require.NoError(t, err)

	// Original chart values must remain unchanged.
	assert.Equal(t, originalReplicas, ch.Values["replicaCount"],
		"MergeValues must not mutate the original chart.Values")
}

func TestMergeValues_RepeatedCallsAreIdempotent(t *testing.T) {
	ch := newTestChart("myapp", "1.0.0")

	vals1, err := MergeValues(ch, ValuesOptions{Values: []string{"replicaCount=5"}})
	require.NoError(t, err)

	vals2, err := MergeValues(ch, ValuesOptions{Values: []string{"replicaCount=10"}})
	require.NoError(t, err)

	// Both calls should start from the same chart defaults, not from
	// the mutated map left by the previous call.
	assert.Equal(t, int64(5), vals1["replicaCount"])
	assert.Equal(t, int64(10), vals2["replicaCount"])
	assert.Equal(t, 1, ch.Values["replicaCount"], "chart.Values must be untouched")
}
