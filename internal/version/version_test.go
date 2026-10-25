package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	require.NotEmpty(t, Version())
	require.NotEmpty(t, Commit())
	require.Equal(t, "unknown", BuildDate())

	buildDate, gitCommit, version = "2026-10-25", "abc1234", "v0.1.0"
	require.Equal(t, "2026-10-25", BuildDate())
	require.Equal(t, "abc1234", Commit())
	require.Equal(t, "v0.1.0", Version())
}
