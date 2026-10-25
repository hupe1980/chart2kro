package chart2kro

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExampleREADMEs keeps the hand-written example READMEs in line with the
// generated reports: every exposed input is listed, with the witness count.
func TestExampleREADMEs(t *testing.T) {
	dirs, err := filepath.Glob("../../examples/*/report.json")
	require.NoError(t, err)
	require.NotEmpty(t, dirs)

	for _, p := range dirs {
		dir := filepath.Dir(p)

		b, err := os.ReadFile(p) //nolint:gosec // test fixtures
		require.NoError(t, err)

		var rep Report
		require.NoError(t, json.Unmarshal(b, &rep))

		readme, err := os.ReadFile(filepath.Join(dir, "README.md")) //nolint:gosec // test fixtures
		require.NoError(t, err)

		doc := string(readme)
		require.Contains(t, doc, fmt.Sprintf("verified at %d witnesses", rep.Witnesses), dir)

		rows := regexp.MustCompile("(?m)^\\| `spec\\.([^`]+)`").FindAllStringSubmatch(doc, -1)

		var listed []string
		for _, r := range rows {
			listed = append(listed, r[1])
		}

		var exposed []string

		for _, v := range rep.Values {
			if v.Outcome == "exposed" {
				exposed = append(exposed, v.Path)
			}
		}

		require.ElementsMatch(t, exposed, listed, "%s/README.md inputs (%s)", dir, strings.Join(listed, ", "))
	}
}
