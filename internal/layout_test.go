package internal_test

import (
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLayout keeps the package list deliberate: adding a package means updating this
// list (and the design notes) on purpose.
func TestLayout(t *testing.T) {
	want := []string{"chart", "cli", "kroval", "manifest", "oracle", "probe", "rgd", "rgddiff", "version", "yamlx"}

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var got []string

	for _, e := range entries {
		if e.IsDir() {
			got = append(got, e.Name())
		}
	}

	sort.Strings(got)
	require.Equal(t, want, got)
}
