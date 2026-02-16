package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Stub subcommands – all should return "not yet implemented"
// ---------------------------------------------------------------------------

func TestStub_Validate_NoArgs(t *testing.T) {
	_, _, err := executeCommand("validate")
	require.Error(t, err)
}

func TestStub_Export_NoArgs(t *testing.T) {
	_, _, err := executeCommand("export")
	require.Error(t, err)
}

func TestDiff_NoArgs(t *testing.T) {
	_, _, err := executeCommand("diff")
	require.Error(t, err)
}

func TestDiff_MissingExisting(t *testing.T) {
	// diff requires --existing flag
	_, _, err := executeCommand("diff", "my-chart")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--existing flag is required")
}

func TestStub_Audit(t *testing.T) {
	_, _, err := executeCommand("audit", "my-chart")
	require.Error(t, err)
	// Audit is now implemented; error is about loading chart, not about stubs.
	assert.NotContains(t, err.Error(), "not yet implemented")
}

func TestStub_Docs(t *testing.T) {
	_, _, err := executeCommand("docs", "my-chart")
	require.Error(t, err)
	// Docs is now implemented; error is about reading the file, not about stubs.
	assert.NotContains(t, err.Error(), "not yet implemented")
}

func TestPlan_NoArgs(t *testing.T) {
	_, _, err := executeCommand("plan")
	require.Error(t, err)
}

func TestWatch_RequiresOutput(t *testing.T) {
	_, _, err := executeCommand("watch", "my-chart")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--output (-o) is required")
}

// ---------------------------------------------------------------------------
// Stub help text – each stub should have a description
// ---------------------------------------------------------------------------

func TestStub_ConvertHelp(t *testing.T) {
	stdout, _, err := executeCommand("convert", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Convert a Helm chart")
}

func TestStub_InspectHelp(t *testing.T) {
	stdout, _, err := executeCommand("inspect", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Inspect a Helm chart")
}

// ---------------------------------------------------------------------------
// Stubs require the correct number of arguments
// ---------------------------------------------------------------------------

func TestStub_ConvertNoArgs(t *testing.T) {
	_, _, err := executeCommand("convert")
	require.Error(t, err)
}

func TestStub_InspectExtraArgs(t *testing.T) {
	_, _, err := executeCommand("inspect", "a", "b")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Completion command
// ---------------------------------------------------------------------------

func TestCompletion_Bash(t *testing.T) {
	stdout, _, err := executeCommand("completion", "bash")
	require.NoError(t, err)
	assert.Contains(t, stdout, "bash completion")
}

func TestCompletion_Zsh(t *testing.T) {
	stdout, _, err := executeCommand("completion", "zsh")
	require.NoError(t, err)
	assert.NotEmpty(t, stdout)
}

func TestCompletion_Fish(t *testing.T) {
	stdout, _, err := executeCommand("completion", "fish")
	require.NoError(t, err)
	assert.Contains(t, stdout, "fish")
}

func TestCompletion_PowerShell(t *testing.T) {
	stdout, _, err := executeCommand("completion", "powershell")
	require.NoError(t, err)
	assert.NotEmpty(t, stdout)
}

func TestCompletion_InvalidShell(t *testing.T) {
	_, _, err := executeCommand("completion", "invalid")
	require.Error(t, err)
}

func TestCompletion_NoArgs(t *testing.T) {
	_, _, err := executeCommand("completion")
	require.Error(t, err)
}
