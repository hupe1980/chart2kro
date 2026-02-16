package version

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetInfo(t *testing.T) {
	info := GetInfo()

	assert.Equal(t, "dev", info.Version)
	assert.Equal(t, "none", info.GitCommit)
	assert.Equal(t, "unknown", info.BuildDate)
	assert.Equal(t, runtime.Version(), info.GoVersion)
	assert.Equal(t, runtime.GOOS+"/"+runtime.GOARCH, info.Platform)
}

func TestInfoString(t *testing.T) {
	info := GetInfo()
	s := info.String()

	assert.Contains(t, s, "chart2kro")
	assert.Contains(t, s, info.Version)
	assert.Contains(t, s, info.GoVersion)
	assert.Contains(t, s, info.Platform)
}

func TestInfoJSON(t *testing.T) {
	info := GetInfo()

	jsonStr, err := info.JSON()
	require.NoError(t, err)

	var parsed Info
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &parsed))

	assert.Equal(t, info.Version, parsed.Version)
	assert.Equal(t, info.GitCommit, parsed.GitCommit)
	assert.Equal(t, info.BuildDate, parsed.BuildDate)
	assert.Equal(t, info.GoVersion, parsed.GoVersion)
	assert.Equal(t, info.Platform, parsed.Platform)
}

func TestShortCommit(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"long SHA truncated", "abc1234def5678", "abc1234"},
		{"exact 7 unchanged", "abc1234", "abc1234"},
		{"short unchanged", "abc", "abc"},
		{"empty unchanged", "", ""},
		{"none unchanged", "none", "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shortCommit(tt.input))
		})
	}
}
