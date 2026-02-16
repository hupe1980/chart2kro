package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvert_SimpleChart(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, stderr, err := executeCommand("convert", chartDir)
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "kind: Deployment")
	assert.Contains(t, stdout, "kind: Service")
	assert.Contains(t, stdout, "nginx")
	assert.Contains(t, stdout, "release-simple")
}

func TestConvert_SimpleChart_CustomReleaseName(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("convert", chartDir, "--release-name", "myrel")
	require.NoError(t, err)
	assert.Contains(t, stdout, "myrel-simple")
}

func TestConvert_SimpleChart_CustomNamespace(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("convert", chartDir, "--namespace", "prod")
	require.NoError(t, err)
	assert.Contains(t, stdout, "namespace: prod")
}

func TestConvert_WithHooks_Default(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-hooks")
	stdout, stderr, err := executeCommand("convert", chartDir)
	require.NoError(t, err)
	// Regular resources should be present.
	assert.Contains(t, stdout, "kind: ConfigMap")
	// Hooks should be dropped.
	assert.NotContains(t, stdout, "helm.sh/hook")
	assert.NotContains(t, stdout, "kind: Job")
	// Summary should report dropped hooks.
	assert.Contains(t, stderr, "Hooks detected: 2")
}

func TestConvert_WithHooks_Include(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-hooks")
	stdout, _, err := executeCommand("convert", chartDir, "--include-hooks")
	require.NoError(t, err)
	// All resources should be present.
	assert.Contains(t, stdout, "kind: ConfigMap")
	assert.Contains(t, stdout, "kind: Job")
	assert.Contains(t, stdout, "kind: Pod")
	// Hook annotations should be stripped.
	assert.NotContains(t, stdout, "helm.sh/hook")
}

func TestConvert_LibraryChart(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "library")
	_, _, err := executeCommand("convert", chartDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "library chart")
}

func TestConvert_WithSubchart(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-subchart")
	stdout, _, err := executeCommand("convert", chartDir)
	require.NoError(t, err)
	assert.Contains(t, stdout, "release-frontend")
	assert.Contains(t, stdout, "release-backend")
}

func TestConvert_WithValuesFile(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	valuesFile := filepath.Join(t.TempDir(), "custom-values.yaml")
	err := os.WriteFile(valuesFile, []byte("replicaCount: 5\n"), 0o600)
	require.NoError(t, err)

	stdout, _, err := executeCommand("convert", chartDir, "-f", valuesFile)
	require.NoError(t, err)
	// With sentinel detection, replicas is now a CEL expression.
	assert.Contains(t, stdout, "${schema.spec.replicaCount}")
	// The schema default should reflect the overridden value.
	assert.Contains(t, stdout, "integer | default=5")
}

func TestConvert_WithSet(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("convert", chartDir, "--set", "replicaCount=3")
	require.NoError(t, err)
	// With sentinel detection, replicas is a CEL expression, not a hardcoded value.
	assert.Contains(t, stdout, "${schema.spec.replicaCount}")
}

func TestConvert_InvalidChartRef(t *testing.T) {
	_, _, err := executeCommand("convert", "/nonexistent/path/chart")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading chart")
}

func TestConvert_NoArgs(t *testing.T) {
	_, _, err := executeCommand("convert")
	require.Error(t, err)
}

func TestConvert_OutputToFile(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	outFile := filepath.Join(t.TempDir(), "output.yaml")

	_, _, err := executeCommand("convert", chartDir, "-o", outFile)
	require.NoError(t, err)

	data, err := os.ReadFile(outFile) //nolint:gosec // test file
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "kind: ResourceGraphDefinition")
	assert.Contains(t, content, "kind: Deployment")
}

func TestConvert_OutputToNestedDir(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	outFile := filepath.Join(t.TempDir(), "nested", "dir", "output.yaml")

	_, _, err := executeCommand("convert", chartDir, "-o", outFile)
	require.NoError(t, err)

	_, err = os.Stat(outFile)
	require.NoError(t, err, "output file should be created with parent dirs")
}

func TestConvert_DryRun(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, stderr, err := executeCommand("convert", chartDir, "--dry-run")
	require.NoError(t, err)

	// Dry-run should still print output to stdout.
	assert.Contains(t, stdout, "kind: ResourceGraphDefinition")
	// Dry-run message on stderr.
	assert.Contains(t, stderr, "Dry-run")
}

func TestConvert_DryRunDoesNotWriteFile(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	outFile := filepath.Join(t.TempDir(), "should-not-exist.yaml")

	_, _, err := executeCommand("convert", chartDir, "-o", outFile, "--dry-run")
	require.NoError(t, err)

	_, err = os.Stat(outFile)
	assert.True(t, os.IsNotExist(err), "file should not be created in dry-run mode")
}

func TestConvert_HelpShowsFlags(t *testing.T) {
	stdout, _, err := executeCommand("convert", "--help")
	require.NoError(t, err)
	for _, flag := range []string{
		"--release-name", "--namespace", "--values", "--set",
		"--set-string", "--set-file", "--include-hooks", "--strict",
		"--timeout", "--repo-url", "--version", "--username", "--password",
	} {
		assert.Contains(t, stdout, flag, "help should mention %s flag", flag)
	}
}

// testdataDir returns the path to the testdata directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	// Walk up from the test file to find testdata/.
	dir, err := os.Getwd()
	require.NoError(t, err)
	// The test runs from internal/cli/, testdata is at project root.
	return filepath.Join(dir, "..", "..", "testdata")
}

// goldenFile reads a golden file from testdata/golden/.
func goldenFile(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(testdataDir(t), "golden", name)

	data, err := os.ReadFile(path) //nolint:gosec // test helper reading golden files from testdata
	require.NoError(t, err, "reading golden file %q", path)

	return string(data)
}

// ---------------------------------------------------------------------------
// Golden file tests
// ---------------------------------------------------------------------------

func TestConvert_Golden_Simple(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, stderr, err := executeCommand("convert", chartDir)
	require.NoError(t, err, "stderr: %s", stderr)

	expected := goldenFile(t, "simple.yaml")
	assert.Equal(t, expected, stdout)
}

func TestConvert_Golden_WithHooks(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-hooks")
	stdout, _, err := executeCommand("convert", chartDir)
	require.NoError(t, err)

	expected := goldenFile(t, "with-hooks.yaml")
	assert.Equal(t, expected, stdout)
}

func TestConvert_Golden_WithHooksIncluded(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-hooks")
	stdout, _, err := executeCommand("convert", chartDir, "--include-hooks")
	require.NoError(t, err)

	expected := goldenFile(t, "with-hooks-included.yaml")
	assert.Equal(t, expected, stdout)
}

func TestConvert_Golden_WithSubchart(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-subchart")
	stdout, _, err := executeCommand("convert", chartDir)
	require.NoError(t, err)

	expected := goldenFile(t, "with-subchart.yaml")
	assert.Equal(t, expected, stdout)
}

func TestConvert_Golden_WithDatabase(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-database")
	stdout, _, err := executeCommand("convert", chartDir)
	require.NoError(t, err)

	expected := goldenFile(t, "with-database.yaml")
	assert.Equal(t, expected, stdout)
}

func TestConvert_Golden_WithDatabaseEnterprise(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-database")
	stdout, stderr, err := executeCommand("convert", chartDir, "--profile", "enterprise")
	require.NoError(t, err, "stderr: %s", stderr)

	expected := goldenFile(t, "with-database-enterprise.yaml")
	assert.Equal(t, expected, stdout)

	// Verify filter summary reports the exclusion.
	assert.Contains(t, stderr, "Excluded: 3")
	assert.Contains(t, stderr, "subchart: postgresql")
}

// ---------------------------------------------------------------------------
// Fast mode tests
// ---------------------------------------------------------------------------

func TestConvert_FastMode_Simple(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, _, err := executeCommand("convert", chartDir, "--fast")
	require.NoError(t, err)

	// Fast mode should produce the same output as sentinel mode for simple charts.
	expected := goldenFile(t, "simple.yaml")
	assert.Equal(t, expected, stdout)
}

func TestConvert_FastMode_VsSentinel(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")

	sentinelOut, _, err := executeCommand("convert", chartDir)
	require.NoError(t, err)

	fastOut, _, err := executeCommand("convert", chartDir, "--fast")
	require.NoError(t, err)

	assert.Equal(t, sentinelOut, fastOut, "fast mode output should match sentinel mode for simple chart")
}

func TestConvert_FastMode_Flag(t *testing.T) {
	// Verify the --fast flag is recognized.
	stdout, _, err := executeCommand("convert", "--help")
	require.NoError(t, err)

	assert.Contains(t, stdout, "--fast")
	assert.Contains(t, stdout, "template AST analysis")
}

// ---------------------------------------------------------------------------
// Filter flag tests
// ---------------------------------------------------------------------------

func TestConvert_ExcludeKinds(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	stdout, stderr, err := executeCommand("convert", chartDir, "--exclude-kinds", "Service")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stdout, "Deployment")
	assert.NotContains(t, stdout, "kind: Service")
	assert.Contains(t, stderr, "Excluded")
}

func TestConvert_ExcludeSubcharts(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-subchart")
	stdout, stderr, err := executeCommand("convert", chartDir, "--exclude-subcharts", "backend")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stdout, "release-frontend")
	assert.NotContains(t, stdout, "release-backend")
	assert.Contains(t, stderr, "Excluded")
}

func TestConvert_ProfileEnterprise(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-subchart")
	// The "enterprise" profile won't match "backend" subchart but should not error.
	stdout, _, err := executeCommand("convert", chartDir, "--profile", "enterprise")
	require.NoError(t, err)
	assert.Contains(t, stdout, "kind: ResourceGraphDefinition")
}

func TestConvert_ProfileMinimal(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-subchart")
	stdout, stderr, err := executeCommand("convert", chartDir, "--profile", "minimal")
	require.NoError(t, err, "stderr: %s", stderr)

	// Minimal profile excludes all subchart resources.
	assert.Contains(t, stdout, "release-frontend")
	assert.Contains(t, stderr, "Excluded")
}

func TestConvert_WithDatabase_ExcludeKinds(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-database")
	stdout, stderr, err := executeCommand("convert", chartDir, "--exclude-kinds", "StatefulSet,Secret")
	require.NoError(t, err, "stderr: %s", stderr)

	// StatefulSet and Secret should be excluded.
	assert.NotContains(t, stdout, "kind: StatefulSet")
	assert.NotContains(t, stdout, "kind: Secret")
	// App resources and postgresql Service should remain.
	assert.Contains(t, stdout, "kind: Deployment")
	assert.Contains(t, stdout, "kind: Service")
	assert.Contains(t, stderr, "excluded by kind")
}

func TestConvert_WithDatabase_ProfileMinimal(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-database")
	stdout, stderr, err := executeCommand("convert", chartDir, "--profile", "minimal")
	require.NoError(t, err, "stderr: %s", stderr)

	// Minimal profile should exclude ALL postgresql subchart resources.
	assert.NotContains(t, stdout, "kind: StatefulSet")
	assert.NotContains(t, stdout, "kind: Secret")
	// App resources should remain.
	assert.Contains(t, stdout, "release-webapp")
	assert.Contains(t, stderr, "minimal profile")
}

func TestConvert_WithDatabase_ExcludeLabels(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-database")
	stdout, stderr, err := executeCommand("convert", chartDir,
		"--exclude-labels", "app.kubernetes.io/component=database")
	require.NoError(t, err, "stderr: %s", stderr)

	// Resources with the database label should be excluded.
	assert.Contains(t, stderr, "excluded by label")
	// App resources should remain.
	assert.Contains(t, stdout, "release-webapp")
}

func TestConvert_WithDatabase_ProfileComposable(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "with-database")
	// Combine enterprise profile with additional kind exclusion.
	stdout, stderr, err := executeCommand("convert", chartDir,
		"--profile", "enterprise", "--exclude-kinds", "Service")
	require.NoError(t, err, "stderr: %s", stderr)

	// Enterprise excludes postgresql subchart, plus exclude-kinds removes all Services.
	assert.NotContains(t, stdout, "kind: Service")
	assert.NotContains(t, stdout, "kind: StatefulSet")
	assert.Contains(t, stdout, "kind: Deployment")
	assert.Contains(t, stderr, "subchart: postgresql")
	assert.Contains(t, stderr, "excluded by kind")
}

func TestConvert_ProfileUnknown(t *testing.T) {
	chartDir := filepath.Join(testdataDir(t), "charts", "simple")
	_, _, err := executeCommand("convert", chartDir, "--profile", "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown profile")
}

func TestConvert_FilterHelpFlags(t *testing.T) {
	stdout, _, err := executeCommand("convert", "--help")
	require.NoError(t, err)

	assert.Contains(t, stdout, "--exclude-kinds")
	assert.Contains(t, stdout, "--exclude-resources")
	assert.Contains(t, stdout, "--exclude-subcharts")
	assert.Contains(t, stdout, "--exclude-labels")
	assert.Contains(t, stdout, "--externalize-secret")
	assert.Contains(t, stdout, "--externalize-service")
	assert.Contains(t, stdout, "--use-external-pattern")
	assert.Contains(t, stdout, "--profile")
}
