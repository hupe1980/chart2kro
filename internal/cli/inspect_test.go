package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInspect_SimpleChart_Table(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("inspect", chartDir)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Chart: simple")
	assert.Contains(t, stdout, "Resources")
	assert.Contains(t, stdout, "Deployment")
	assert.Contains(t, stdout, "Service")
}

func TestInspect_SimpleChart_JSON(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("inspect", chartDir, "--format", "json")
	require.NoError(t, err)

	assert.Contains(t, stdout, `"name": "simple"`)
	assert.Contains(t, stdout, `"kind": "Deployment"`)
}

func TestInspect_SimpleChart_YAML(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("inspect", chartDir, "--format", "yaml")
	require.NoError(t, err)

	assert.Contains(t, stdout, "name: simple")
}

func TestInspect_WithSubchart(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-subchart")
	stdout, _, err := executeCommand("inspect", chartDir)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Chart: with-subchart")
	assert.Contains(t, stdout, "Resources")
}

func TestInspect_ShowResources(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("inspect", chartDir, "--show-resources")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Resources")
	assert.NotContains(t, stdout, "Schema Fields")
}

func TestInspect_ShowSchema(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("inspect", chartDir, "--show-schema")
	require.NoError(t, err)

	// Should show schema but not resource table
	assert.NotContains(t, stdout, "Chart:")
}

func TestInspect_InvalidFormat(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	_, _, err := executeCommand("inspect", chartDir, "--format", "invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown format")
}

func TestInspect_LibraryChart(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "library")
	_, _, err := executeCommand("inspect", chartDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "library chart")
}

func TestInspect_NoArgs(t *testing.T) {
	_, _, err := executeCommand("inspect")
	require.Error(t, err)
}

func TestInspect_WithDatabase(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-database")
	stdout, _, err := executeCommand("inspect", chartDir)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Chart: with-database")
	assert.Contains(t, stdout, "StatefulSet")
	assert.Contains(t, stdout, "Deployment")
	assert.Contains(t, stdout, "Secret")
	// Should show excludable infra & detected external patterns.
	assert.Contains(t, stdout, "postgresql")
}

func TestInspect_WithDatabase_JSON(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-database")
	stdout, _, err := executeCommand("inspect", chartDir, "--format", "json")
	require.NoError(t, err)

	assert.Contains(t, stdout, `"name": "with-database"`)
	assert.Contains(t, stdout, `"kind": "StatefulSet"`)
	assert.Contains(t, stdout, `"subchartName": "postgresql"`)
	assert.Contains(t, stdout, `"externalValuesKey": "externalDatabase"`)
}

func TestInspect_WithDatabase_ShowValues(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-database")
	stdout, _, err := executeCommand("inspect", chartDir, "--show-values")
	require.NoError(t, err)

	// show-values shows subchart info and external patterns.
	assert.Contains(t, stdout, "postgresql")
	assert.Contains(t, stdout, "Excludable Infrastructure")
}
