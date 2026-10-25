package chart2kro

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

// TestReportsMatchPublishedSchema validates every golden and example report against
// site/static/schemas/report-v1.json, the public contract.
func TestReportsMatchPublishedSchema(t *testing.T) {
	f, err := os.Open("../../site/static/schemas/report-v1.json")
	require.NoError(t, err)

	doc, err := jsonschema.UnmarshalJSON(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	c := jsonschema.NewCompiler()
	require.NoError(t, c.AddResource("report-v1.json", doc))

	sch, err := c.Compile("report-v1.json")
	require.NoError(t, err)

	reports, err := filepath.Glob("../../testdata/golden/*/report.json")
	require.NoError(t, err)

	examples, err := filepath.Glob("../../examples/*/report.json")
	require.NoError(t, err)

	reports = append(reports, examples...)
	require.NotEmpty(t, reports)

	for _, p := range reports {
		b, err := os.ReadFile(p) //nolint:gosec // test fixtures
		require.NoError(t, err)

		v, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		require.NoError(t, err)
		require.NoError(t, sch.Validate(v), p)
	}
}

// TestDiffMatchesPublishedSchema validates diff JSON against site/static/schemas/diff-v1.json.
func TestDiffMatchesPublishedSchema(t *testing.T) {
	f, err := os.Open("../../site/static/schemas/diff-v1.json")
	require.NoError(t, err)

	doc, err := jsonschema.UnmarshalJSON(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	c := jsonschema.NewCompiler()
	require.NoError(t, c.AddResource("diff-v1.json", doc))

	sch, err := c.Compile("diff-v1.json")
	require.NoError(t, err)

	old, err := os.ReadFile("../../testdata/golden/simple/rgd.yaml")
	require.NoError(t, err)

	updated, err := os.ReadFile("../../testdata/golden/features/rgd.yaml")
	require.NoError(t, err)

	res, err := Diff(old, updated)
	require.NoError(t, err)
	require.NotEmpty(t, res.Changes)

	b, err := json.Marshal(res)
	require.NoError(t, err)

	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	require.NoError(t, err)
	require.NoError(t, sch.Validate(v))
}
