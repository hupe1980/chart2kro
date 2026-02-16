package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestRGD(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test-rgd.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644)) //nolint:gosec // test helper

	return path
}

const validRGDYAML = `apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: test
spec:
  schema:
    apiVersion: v1alpha1
    kind: TestApp
    spec:
      replicaCount: integer
  resources:
    - id: deployment
      readyWhen:
        - "${self.status.availableReplicas == self.status.replicas}"
      template:
        apiVersion: apps/v1
        kind: Deployment
        spec:
          replicas: "${schema.spec.replicaCount}"
`

func TestValidate_ValidFile(t *testing.T) {
	path := writeTestRGD(t, validRGDYAML)

	stdout, _, err := executeCommand("validate", path)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Validation passed")
}

func TestValidate_InvalidYAML(t *testing.T) {
	path := writeTestRGD(t, "not: valid: yaml: [")

	_, _, err := executeCommand("validate", path)
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.Code)
}

func TestValidate_MissingFields(t *testing.T) {
	path := writeTestRGD(t, `
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata: {}
spec: {}
`)

	_, _, err := executeCommand("validate", path)
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.Code)
}

func TestValidate_StrictMode(t *testing.T) {
	// A valid RGD with warnings (missing readyWhen).
	path := writeTestRGD(t, `
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: test
spec:
  schema:
    apiVersion: v1alpha1
    kind: TestApp
    spec:
      count: integer
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
`)

	// Without --strict, warnings are OK.
	stdout, _, err := executeCommand("validate", path)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Validation passed")

	// With --strict, warnings become failures.
	_, _, err = executeCommand("validate", "--strict", path)
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.Code)
}

func TestValidate_FileNotFound(t *testing.T) {
	_, _, err := executeCommand("validate", "/nonexistent/file.yaml")
	require.Error(t, err)
}

func TestValidate_Help(t *testing.T) {
	stdout, _, err := executeCommand("validate", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Validate a generated ResourceGraphDefinition")
	assert.Contains(t, stdout, "--strict")
}
