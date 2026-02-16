package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/version"
)

func TestVersionCommand_Human(t *testing.T) {
	stdout, _, err := executeCommand("version")
	require.NoError(t, err)

	assert.Contains(t, stdout, "chart2kro")
	assert.Contains(t, stdout, "dev")
}

func TestVersionCommand_JSON(t *testing.T) {
	stdout, _, err := executeCommand("version", "--json")
	require.NoError(t, err)

	var info version.Info
	require.NoError(t, json.Unmarshal([]byte(stdout), &info))

	assert.Equal(t, "dev", info.Version)
	assert.NotEmpty(t, info.GoVersion)
	assert.NotEmpty(t, info.Platform)
}

func TestVersionCommand_NoArgs(t *testing.T) {
	_, _, err := executeCommand("version", "extra")
	require.Error(t, err)
}
