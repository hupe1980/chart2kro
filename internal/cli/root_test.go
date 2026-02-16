package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeCommand is a test helper that runs the CLI with the given args and
// captures both stdout and stderr.
func executeCommand(args ...string) (stdout, stderr string, err error) {
	cmd := NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()

	return outBuf.String(), errBuf.String(), err
}

// ---------------------------------------------------------------------------
// Help output
// ---------------------------------------------------------------------------

func TestRootCommand_Help(t *testing.T) {
	stdout, _, err := executeCommand("--help")
	require.NoError(t, err)

	// Must list every planned subcommand.
	for _, sub := range []string{
		"convert", "inspect", "validate", "export", "diff",
		"audit", "docs", "plan", "watch", "version", "completion",
	} {
		assert.Contains(t, stdout, sub, "help should mention %q subcommand", sub)
	}

	// Must list global flags.
	for _, flag := range []string{"--config", "--log-level", "--log-format", "--no-color", "--quiet"} {
		assert.Contains(t, stdout, flag, "help should mention %q flag", flag)
	}
}

// ---------------------------------------------------------------------------
// Unknown flags → exit code 2
// ---------------------------------------------------------------------------

func TestRootCommand_UnknownFlag(t *testing.T) {
	_, _, err := executeCommand("--nonexistent")
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 2, exitErr.Code)
}

// ---------------------------------------------------------------------------
// SilenceErrors – cobra must not print errors itself
// ---------------------------------------------------------------------------

func TestRootCommand_SilenceErrors(t *testing.T) {
	_, stderr, err := executeCommand("--nonexistent")
	require.Error(t, err)
	assert.Empty(t, stderr, "cobra should not print errors to stderr (SilenceErrors)")
}

// ---------------------------------------------------------------------------
// Invalid --config → exit code 2
// ---------------------------------------------------------------------------

func TestRootCommand_InvalidConfig(t *testing.T) {
	_, _, err := executeCommand("--config", "/nonexistent/path.yaml", "inspect", "my-chart")
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 2, exitErr.Code)
	assert.Contains(t, err.Error(), "reading config file")
}

// ---------------------------------------------------------------------------
// Invalid --log-level → exit code 2 (validation error)
// ---------------------------------------------------------------------------

func TestRootCommand_InvalidLogLevel(t *testing.T) {
	_, _, err := executeCommand("--log-level", "trace", "inspect", "my-chart")
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 2, exitErr.Code)
	assert.Contains(t, err.Error(), "invalid log level")
}

// ---------------------------------------------------------------------------
// Invalid --log-format → exit code 2 (validation error)
// ---------------------------------------------------------------------------

func TestRootCommand_InvalidLogFormat(t *testing.T) {
	_, _, err := executeCommand("--log-format", "xml", "inspect", "my-chart")
	require.Error(t, err)

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 2, exitErr.Code)
	assert.Contains(t, err.Error(), "invalid log format")
}

// ---------------------------------------------------------------------------
// Stub commands return non-zero exit code via Execute()
// ---------------------------------------------------------------------------

func TestExecute_ConvertReturnsExitCode1_BadRef(t *testing.T) {
	// Convert with an invalid chart reference returns exit code 1.
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"convert", "some-chart"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading chart")
}

// ---------------------------------------------------------------------------
// Execute helper
// ---------------------------------------------------------------------------

func TestExecute_Success(t *testing.T) {
	code := Execute()
	// Execute runs with no args, which shows help and returns 0.
	assert.Equal(t, 0, code)
}

func TestExecute_VersionSubcommand(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"version"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	code := 0
	if err := cmd.Execute(); err != nil {
		code = 1
	}

	assert.Equal(t, 0, code)
}

// ---------------------------------------------------------------------------
// ExitError
// ---------------------------------------------------------------------------

func TestExitError_ErrorWithMessage(t *testing.T) {
	err := &ExitError{Code: 1, Err: assert.AnError}
	assert.Contains(t, err.Error(), assert.AnError.Error())
	assert.ErrorIs(t, err, assert.AnError)
}

func TestExitError_ErrorWithoutMessage(t *testing.T) {
	err := &ExitError{Code: 42}
	assert.Equal(t, "exit code 42", err.Error())
	assert.Nil(t, err.Unwrap())
}
