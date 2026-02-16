package chart2kro_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/pkg/chart2kro"
)

func TestConvert_EmptyRef(t *testing.T) {
	_, err := chart2kro.Convert(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chart reference must not be empty")
}

func TestConvert_NoOptions(t *testing.T) {
	_, err := chart2kro.Convert(context.Background(), "nonexistent-chart")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading chart")
}

func TestConvert_InvalidChartRef(t *testing.T) {
	_, err := chart2kro.Convert(context.Background(), "/nonexistent/path/to/chart")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading chart")
}

func TestConvert_LocalChart(t *testing.T) {
	result, err := chart2kro.Convert(context.Background(), "../../testdata/charts/simple")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotEmpty(t, result.YAML)
	assert.NotNil(t, result.RGDMap)
	assert.NotEmpty(t, result.ChartName)
	assert.Greater(t, result.ResourceCount, 0)

	yamlStr := string(result.YAML)
	assert.Contains(t, yamlStr, "apiVersion: kro.run/v1alpha1")
	assert.Contains(t, yamlStr, "kind: ResourceGraphDefinition")
}

func TestConvert_WithOptions(t *testing.T) {
	result, err := chart2kro.Convert(context.Background(), "../../testdata/charts/simple",
		chart2kro.WithReleaseName("test-release"),
		chart2kro.WithNamespace("test-ns"),
		chart2kro.WithTimeout(60*time.Second),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.YAML)
}

func TestConvert_IncludeAllValues(t *testing.T) {
	resultDefault, err := chart2kro.Convert(context.Background(), "../../testdata/charts/simple")
	require.NoError(t, err)

	resultAll, err := chart2kro.Convert(context.Background(), "../../testdata/charts/simple",
		chart2kro.WithIncludeAllValues(),
	)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, resultAll.SchemaFieldCount, resultDefault.SchemaFieldCount)
}

func TestConvert_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := chart2kro.Convert(ctx, "../../testdata/charts/simple")
	require.Error(t, err)
}

func TestConvert_SchemaOverrides(t *testing.T) {
	result, err := chart2kro.Convert(context.Background(), "../../testdata/charts/simple",
		chart2kro.WithSchemaOverrides(map[string]chart2kro.SchemaOverride{
			"replicaCount": {Type: "integer", Default: 5},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.YAML)
}

func TestConvert_MultipleOptions(t *testing.T) {
	result, err := chart2kro.Convert(context.Background(), "../../testdata/charts/simple",
		chart2kro.WithReleaseName("multi"),
		chart2kro.WithNamespace("staging"),
		chart2kro.WithIncludeAllValues(),
		chart2kro.WithTimeout(45*time.Second),
	)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.YAML)
}

func TestConvert_Defaults(t *testing.T) {
	result, err := chart2kro.Convert(context.Background(), "../../testdata/charts/simple")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestResult_HasExpectedStructure(t *testing.T) {
	result, err := chart2kro.Convert(context.Background(), "../../testdata/charts/simple")
	require.NoError(t, err)

	apiVersion, ok := result.RGDMap["apiVersion"]
	assert.True(t, ok, "RGDMap should have apiVersion")
	assert.Equal(t, "kro.run/v1alpha1", apiVersion)

	kind, ok := result.RGDMap["kind"]
	assert.True(t, ok, "RGDMap should have kind")
	assert.Equal(t, "ResourceGraphDefinition", kind)

	_, ok = result.RGDMap["metadata"]
	assert.True(t, ok, "RGDMap should have metadata")

	_, ok = result.RGDMap["spec"]
	assert.True(t, ok, "RGDMap should have spec")
}
