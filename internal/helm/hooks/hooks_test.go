package hooks

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

func TestHookType_IsTestHook(t *testing.T) {
	tests := []struct {
		hookType HookType
		expected bool
	}{
		{HookTest, true},
		{HookTestSuccess, true},
		{HookPreInstall, false},
		{HookPostInstall, false},
		{HookPreUpgrade, false},
		{HookPostUpgrade, false},
		{HookPreDelete, false},
		{HookPostDelete, false},
		{HookPreRollback, false},
		{HookPostRollback, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.hookType), func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.hookType.IsTestHook())
		})
	}
}

func TestSplitYAMLDocuments(t *testing.T) {
	input := "doc1\n---\ndoc2\n---\ndoc3"
	docs := splitYAMLDocuments([]byte(input))
	assert.Len(t, docs, 3)
}

func TestSplitYAMLDocuments_SingleDoc(t *testing.T) {
	docs := splitYAMLDocuments([]byte("only-one"))
	assert.Len(t, docs, 1)
}

func TestSplitYAMLDocuments_LeadingSeparator(t *testing.T) {
	docs := splitYAMLDocuments([]byte("---\napiVersion: v1\nkind: ConfigMap"))
	assert.Len(t, docs, 1)
	assert.Contains(t, docs[0], "kind: ConfigMap")
}

func TestSplitYAMLDocuments_TrailingWhitespace(t *testing.T) {
	docs := splitYAMLDocuments([]byte("doc1\n---  \ndoc2"))
	assert.Len(t, docs, 2)
}

func TestParseResource_RegularResource(t *testing.T) {
	doc := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: my-config\ndata:\n  key: value"
	res := parseResource(doc)
	assert.Equal(t, "ConfigMap", res.Kind)
	assert.Equal(t, "v1", res.APIVersion)
	assert.Equal(t, "my-config", res.Name)
	assert.False(t, res.IsHook)
	assert.Empty(t, res.HookTypes)
}

func TestParseResource_HookResource(t *testing.T) {
	doc := "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: db-migrate\n  annotations:\n    helm.sh/hook: pre-install,pre-upgrade"
	res := parseResource(doc)
	assert.Equal(t, "Job", res.Kind)
	assert.Equal(t, "db-migrate", res.Name)
	assert.True(t, res.IsHook)
	assert.Len(t, res.HookTypes, 2)
	assert.Equal(t, HookPreInstall, res.HookTypes[0])
	assert.Equal(t, HookPreUpgrade, res.HookTypes[1])
}

func TestParseResource_TestHook(t *testing.T) {
	doc := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: test-connection\n  annotations:\n    helm.sh/hook: test"
	res := parseResource(doc)
	assert.True(t, res.IsHook)
	assert.Len(t, res.HookTypes, 1)
	assert.Equal(t, HookTest, res.HookTypes[0])
	assert.True(t, res.HookTypes[0].IsTestHook())
}

func TestParseResource_QuotedAnnotation(t *testing.T) {
	doc := "apiVersion: v1\nkind: Job\nmetadata:\n  name: hook-job\n  annotations:\n    helm.sh/hook: \"post-install\""
	res := parseResource(doc)
	assert.True(t, res.IsHook)
	assert.Equal(t, HookPostInstall, res.HookTypes[0])
}

func TestExtractAnnotation_ViaYAMLParse(t *testing.T) {
	doc := "apiVersion: v1\nkind: Job\nmetadata:\n  name: hook-job\n  annotations:\n    helm.sh/hook: pre-install\n    helm.sh/hook-weight: \"-5\""
	res := parseResource(doc)
	assert.True(t, res.IsHook)
	assert.Len(t, res.HookTypes, 1)
	assert.Equal(t, HookPreInstall, res.HookTypes[0])
}

func TestExtractAnnotation_HookWeightDoesNotTriggerHook(t *testing.T) {
	// A resource with only helm.sh/hook-weight but no helm.sh/hook is NOT a hook.
	doc := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: weighted\n  annotations:\n    helm.sh/hook-weight: \"5\""
	res := parseResource(doc)
	assert.False(t, res.IsHook)
	assert.Empty(t, res.HookTypes)
}

func TestParseResource_MalformedYAML(t *testing.T) {
	doc := "not: valid: yaml: {{template}}"
	res := parseResource(doc)
	// Should not panic, returns empty metadata.
	assert.Empty(t, res.Kind)
	assert.False(t, res.IsHook)
}

func TestParseResource_KindInDataBlock(t *testing.T) {
	// Ensures YAML parsing extracts top-level 'kind' not 'kind' in data.
	doc := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cfg\ndata:\n  kind: ShouldNotMatch\n  name: inner"
	res := parseResource(doc)
	assert.Equal(t, "ConfigMap", res.Kind)
	assert.Equal(t, "cfg", res.Name)
}

func TestStripHookAnnotations(t *testing.T) {
	input := "apiVersion: v1\nkind: Job\nmetadata:\n  name: test\n  annotations:\n    helm.sh/hook: pre-install\n    helm.sh/hook-weight: \"0\"\n    other: keep-me"
	result := stripHookAnnotations(input)
	assert.NotContains(t, result, "helm.sh/hook")
	assert.Contains(t, result, "other: keep-me")
	assert.Contains(t, result, "kind: Job")
}

func TestStripHookAnnotations_NoHooks(t *testing.T) {
	input := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test"
	result := stripHookAnnotations(input)
	assert.Equal(t, input, result)
}

func TestStripHookAnnotations_AllAnnotationsAreHooks(t *testing.T) {
	input := "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: migrate\n  annotations:\n    helm.sh/hook: pre-install\n    helm.sh/hook-weight: \"0\""
	result := stripHookAnnotations(input)
	assert.NotContains(t, result, "helm.sh/hook")
	assert.NotContains(t, result, "annotations:")
	assert.NotContains(t, result, "annotations: null")
	assert.Contains(t, result, "name: migrate")
}

func TestStripHookAnnotations_PreservesNonHookAnnotations(t *testing.T) {
	input := "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: test\n  annotations:\n    helm.sh/hook: pre-install\n    app.kubernetes.io/managed-by: helm"
	result := stripHookAnnotations(input)
	assert.NotContains(t, result, "helm.sh/hook")
	assert.Contains(t, result, "annotations:")
	assert.Contains(t, result, "app.kubernetes.io/managed-by: helm")
}

func TestFilter_NoHooks(t *testing.T) {
	docs := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config-a\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config-b")
	result, err := Filter(docs, false, discardLogger())
	require.NoError(t, err)
	assert.Len(t, result.Resources, 2)
	assert.Equal(t, 0, result.HookCount)
	assert.Empty(t, result.DroppedHooks)
	assert.Empty(t, result.IncludedHooks)
}

func TestFilter_DropsLifecycleHooks(t *testing.T) {
	docs := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config\n---\napiVersion: batch/v1\nkind: Job\nmetadata:\n  name: migrate\n  annotations:\n    helm.sh/hook: pre-install")
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	result, err := Filter(docs, false, logger)
	require.NoError(t, err)
	assert.Len(t, result.Resources, 1)
	assert.Equal(t, "config", result.Resources[0].Name)
	assert.Len(t, result.DroppedHooks, 1)
	assert.Equal(t, "migrate", result.DroppedHooks[0].Name)
	assert.Equal(t, 1, result.HookCount)
	assert.Contains(t, logBuf.String(), "dropping Helm hook resource")
}

func TestFilter_DropsTestHooksSilently(t *testing.T) {
	docs := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config\n---\napiVersion: v1\nkind: Pod\nmetadata:\n  name: test-connection\n  annotations:\n    helm.sh/hook: test")
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	result, err := Filter(docs, false, logger)
	require.NoError(t, err)
	assert.Len(t, result.Resources, 1)
	assert.Len(t, result.DroppedHooks, 1)
	assert.Equal(t, 1, result.HookCount)
	assert.NotContains(t, logBuf.String(), "dropping Helm hook resource")
}

func TestFilter_IncludeHooks(t *testing.T) {
	docs := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config\n---\napiVersion: batch/v1\nkind: Job\nmetadata:\n  name: migrate\n  annotations:\n    helm.sh/hook: pre-install\n    helm.sh/hook-weight: \"5\"")
	result, err := Filter(docs, true, discardLogger())
	require.NoError(t, err)
	assert.Len(t, result.Resources, 2)
	assert.Len(t, result.IncludedHooks, 1)
	assert.Empty(t, result.DroppedHooks)
	assert.Equal(t, 1, result.HookCount)
	assert.NotContains(t, result.IncludedHooks[0].RawYAML, "helm.sh/hook")
}

func TestFilter_MultipleHookTypes(t *testing.T) {
	docs := []byte("apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: rotate\n  annotations:\n    helm.sh/hook: pre-install,post-upgrade")
	result, err := Filter(docs, false, discardLogger())
	require.NoError(t, err)
	assert.Len(t, result.DroppedHooks, 1)
	assert.Len(t, result.DroppedHooks[0].HookTypes, 2)
	assert.Equal(t, HookPreInstall, result.DroppedHooks[0].HookTypes[0])
	assert.Equal(t, HookPostUpgrade, result.DroppedHooks[0].HookTypes[1])
}

func TestFilter_EmptyDocument(t *testing.T) {
	docs := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config\n---\n\n---\napiVersion: v1\nkind: Service\nmetadata:\n  name: svc")
	result, err := Filter(docs, false, discardLogger())
	require.NoError(t, err)
	assert.Len(t, result.Resources, 2)
}

func TestFilter_AllHooks(t *testing.T) {
	docs := []byte("apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: pre-job\n  annotations:\n    helm.sh/hook: pre-install\n---\napiVersion: v1\nkind: Pod\nmetadata:\n  name: test-pod\n  annotations:\n    helm.sh/hook: test")
	result, err := Filter(docs, false, discardLogger())
	require.NoError(t, err)
	assert.Empty(t, result.Resources)
	assert.Len(t, result.DroppedHooks, 2)
	assert.Equal(t, 2, result.HookCount)
}

func TestCombineResources(t *testing.T) {
	result := &FilterResult{
		Resources: []Resource{
			{RawYAML: "kind: ConfigMap\nmetadata:\n  name: a"},
			{RawYAML: "kind: Service\nmetadata:\n  name: b"},
		},
	}
	combined := CombineResources(result)
	output := string(combined)
	assert.Contains(t, output, "kind: ConfigMap")
	assert.Contains(t, output, "---\n")
	assert.Contains(t, output, "kind: Service")
}

func TestCombineResources_Empty(t *testing.T) {
	result := &FilterResult{}
	combined := CombineResources(result)
	assert.Empty(t, combined)
}

func TestCombineResources_SingleResource(t *testing.T) {
	result := &FilterResult{
		Resources: []Resource{{RawYAML: "kind: ConfigMap"}},
	}
	combined := CombineResources(result)
	assert.NotContains(t, string(combined), "---")
}

func TestPrintHookSummary_NoHooks(t *testing.T) {
	var buf bytes.Buffer
	PrintHookSummary(&buf, &FilterResult{})
	assert.Empty(t, buf.String())
}

func TestPrintHookSummary_DroppedHooks(t *testing.T) {
	var buf bytes.Buffer
	result := &FilterResult{
		HookCount: 2,
		DroppedHooks: []Resource{
			{Kind: "Job", Name: "migrate", HookTypes: []HookType{HookPreInstall}},
			{Kind: "Pod", Name: "test-conn", HookTypes: []HookType{HookTest}},
		},
	}
	PrintHookSummary(&buf, result)
	output := buf.String()
	assert.Contains(t, output, "Hooks detected: 2")
	assert.Contains(t, output, "Dropped: 2")
	assert.Contains(t, output, "Job/migrate (pre-install)")
	assert.Contains(t, output, "Pod/test-conn (test)")
}

func TestPrintHookSummary_IncludedHooks(t *testing.T) {
	var buf bytes.Buffer
	result := &FilterResult{
		HookCount:     1,
		IncludedHooks: []Resource{{Kind: "Job", Name: "x"}},
	}
	PrintHookSummary(&buf, result)
	assert.Contains(t, buf.String(), "Included as regular resources: 1")
}

func TestFilter_FullPipeline(t *testing.T) {
	yamlDocs := []byte(strings.Join([]string{
		"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: my-app\nspec:\n  replicas: 1",
		"apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: db-migrate\n  annotations:\n    helm.sh/hook: pre-install\n    helm.sh/hook-weight: \"-5\"",
		"apiVersion: v1\nkind: Pod\nmetadata:\n  name: test-connection\n  annotations:\n    helm.sh/hook: test-success",
		"apiVersion: v1\nkind: Service\nmetadata:\n  name: my-svc\nspec:\n  type: ClusterIP",
	}, "\n---\n"))

	result, err := Filter(yamlDocs, false, discardLogger())
	require.NoError(t, err)
	assert.Len(t, result.Resources, 2)
	assert.Equal(t, "my-app", result.Resources[0].Name)
	assert.Equal(t, "my-svc", result.Resources[1].Name)
	assert.Len(t, result.DroppedHooks, 2)
	assert.Equal(t, 2, result.HookCount)

	combined := CombineResources(result)
	assert.Contains(t, string(combined), "kind: Deployment")
	assert.Contains(t, string(combined), "kind: Service")
	assert.NotContains(t, string(combined), "helm.sh/hook")

	result2, err := Filter(yamlDocs, true, discardLogger())
	require.NoError(t, err)
	assert.Len(t, result2.Resources, 4)
	assert.Len(t, result2.IncludedHooks, 2)
	assert.Empty(t, result2.DroppedHooks)

	for _, r := range result2.IncludedHooks {
		assert.NotContains(t, r.RawYAML, "helm.sh/hook")
	}
}
