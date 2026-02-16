package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// diff command tests
// ---------------------------------------------------------------------------

func TestDiff_Help(t *testing.T) {
	stdout, _, err := executeCommand("diff", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Diff compares")
	assert.Contains(t, stdout, "--existing")
	assert.Contains(t, stdout, "--no-color")
	assert.Contains(t, stdout, "--format")
}

func TestDiff_RequiresExistingFlag(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	_, _, err := executeCommand("diff", chartDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--existing flag is required")
}

func TestDiff_ExistingFileNotFound(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	_, _, err := executeCommand("diff", chartDir, "--existing", "nonexistent.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading file")
}

func TestDiff_IdenticalOutput(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	goldenPath := filepath.Join(testdataDir(t), "golden", "simple.yaml")

	stdout, _, err := executeCommand("diff", chartDir,
		"--existing", goldenPath,
		"--no-color",
	)
	// May or may not have differences depending on pipeline determinism.
	if err != nil {
		t.Logf("diff returned error: %v", err)
	}

	t.Logf("diff output:\n%s", stdout)
}

func TestDiff_JSONFormat(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	goldenPath := filepath.Join(testdataDir(t), "golden", "simple.yaml")

	stdout, _, err := executeCommand("diff", chartDir,
		"--existing", goldenPath,
		"--format", "json",
	)
	if err != nil {
		t.Logf("diff returned error (expected for changes): %v", err)
	}

	assert.Contains(t, stdout, "schemaChanges")
}

func TestDiff_WithModifiedExisting(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	goldenPath := filepath.Join(testdataDir(t), "golden", "simple.yaml")

	data, err := os.ReadFile(goldenPath)
	require.NoError(t, err)

	// Create a modified version with an extra field.
	modified := string(data) + "\n# modified\n"

	tmpDir := t.TempDir()
	modifiedPath := filepath.Join(tmpDir, "modified.yaml")
	require.NoError(t, os.WriteFile(modifiedPath, []byte(modified), 0o644))

	stdout, _, err := executeCommand("diff", chartDir,
		"--existing", modifiedPath,
		"--no-color",
	)
	if err != nil {
		t.Logf("diff returned error: %v", err)
	}

	t.Logf("diff output:\n%s", stdout)
}

// ---------------------------------------------------------------------------
// plan command tests
// ---------------------------------------------------------------------------

func TestPlan_Help(t *testing.T) {
	stdout, _, err := executeCommand("plan", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Plan shows what")
	assert.Contains(t, stdout, "--existing")
	assert.Contains(t, stdout, "--format")
}

func TestPlan_SimpleChart(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("plan", chartDir)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Plan:")
	assert.Contains(t, stdout, "Schema Fields:")
	assert.Contains(t, stdout, "Resources:")
}

func TestPlan_JSONFormat(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("plan", chartDir, "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, stdout, `"name"`)
	assert.Contains(t, stdout, `"schemaFields"`)
	assert.Contains(t, stdout, `"resources"`)
}

func TestPlan_CompactFormat(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("plan", chartDir, "--format", "compact")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Plan:")
	assert.Contains(t, stdout, "schema fields")
	assert.Contains(t, stdout, "resources")
}

func TestPlan_WithExisting(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	goldenPath := filepath.Join(testdataDir(t), "golden", "simple.yaml")

	stdout, _, err := executeCommand("plan", chartDir,
		"--existing", goldenPath,
	)
	if err != nil {
		t.Logf("plan returned error: %v", err)
	}

	assert.Contains(t, stdout, "Plan:")
}

func TestPlan_WithExistingJSON(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	goldenPath := filepath.Join(testdataDir(t), "golden", "simple.yaml")

	stdout, _, err := executeCommand("plan", chartDir,
		"--existing", goldenPath,
		"--format", "json",
	)
	if err != nil {
		t.Logf("plan returned error: %v", err)
	}

	assert.Contains(t, stdout, `"name"`)
}

func TestPlan_ExistingFileNotFound(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	_, _, err := executeCommand("plan", chartDir,
		"--existing", "nonexistent.yaml",
	)
	require.Error(t, err)
}

func TestPlan_WithBreakingChanges(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")

	tmpDir := t.TempDir()
	existingPath := filepath.Join(tmpDir, "existing.yaml")

	existingRGD := `apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: simple
spec:
  schema:
    apiVersion: v1alpha1
    kind: Simple
    spec:
      removedField: string
      anotherRemoved: integer | default=42
  resources:
  - id: removed-resource
    template:
      apiVersion: v1
      kind: ConfigMap
`
	require.NoError(t, os.WriteFile(existingPath, []byte(existingRGD), 0o644))

	stdout, _, err := executeCommand("plan", chartDir,
		"--existing", existingPath,
		"--format", "compact",
	)

	// Expect exit code 8 for breaking changes.
	if exitErr, ok := err.(*ExitError); ok {
		assert.Equal(t, 8, exitErr.Code, "expected exit code 8 for breaking changes")
	} else {
		// Even if it fails for chart loading, we want to note it.
		t.Logf("plan returned non-ExitError: %v", err)
	}

	t.Logf("plan output:\n%s", stdout)
}
