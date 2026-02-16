package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Subcommand error handling
// ---------------------------------------------------------------------------

func TestValidate_NoArgs(t *testing.T) {
	_, _, err := executeCommand("validate")
	require.Error(t, err)
}

func TestExport_NoArgs(t *testing.T) {
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

func TestAudit_InvalidChart(t *testing.T) {
	_, _, err := executeCommand("audit", "my-chart")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "not yet implemented")
}

func TestDocs_InvalidFile(t *testing.T) {
	_, _, err := executeCommand("docs", "my-chart")
	require.Error(t, err)
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
// Help text
// ---------------------------------------------------------------------------

func TestConvert_Help(t *testing.T) {
	stdout, _, err := executeCommand("convert", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Convert a Helm chart")
}

func TestInspect_Help(t *testing.T) {
	stdout, _, err := executeCommand("inspect", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Inspect a Helm chart")
}

// ---------------------------------------------------------------------------
// Argument validation
// ---------------------------------------------------------------------------

func TestConvert_RequiresChartArg(t *testing.T) {
	_, _, err := executeCommand("convert")
	require.Error(t, err)
}

func TestInspect_ExtraArgs(t *testing.T) {
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
