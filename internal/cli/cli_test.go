package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/pkg/chart2kro"
)

const simple = "../../testdata/charts/simple"

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var out, errb bytes.Buffer
	code := Run(context.Background(), args, &out, &errb)

	return code, out.String(), errb.String()
}

func TestConvertStdoutMatchesLibrary(t *testing.T) {
	code, out, _ := run(t, "convert", simple, "-q")
	require.Equal(t, 0, code)

	res, err := chart2kro.Convert(context.Background(), chart2kro.Options{Chart: simple})
	require.NoError(t, err)
	require.Equal(t, string(res.RGD), out, "the CLI is a client of the library")
}

func TestConvertDirAndCheck(t *testing.T) {
	dir := t.TempDir()

	code, _, _ := run(t, "convert", simple, "-o", dir, "-q")
	require.Equal(t, 0, code)

	for _, f := range []string{"rgd.yaml", "instance.yaml", "report.json"} {
		_, err := os.Stat(filepath.Join(dir, f))
		require.NoError(t, err, f)
	}

	code, _, _ = run(t, "convert", simple, "-o", dir, "--check", "-q")
	require.Equal(t, 0, code)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "rgd.yaml"), []byte("stale"), 0o600))
	code, _, errOut := run(t, "convert", simple, "-o", dir, "--check", "-q")
	require.Equal(t, 9, code)
	require.Contains(t, errOut, "rgd.yaml")
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"convert"}, 2},
		{[]string{"convert", simple, "--no-such-flag"}, 2},
		{[]string{"convert", simple, "--kind", "bad-kind"}, 2},
		{[]string{"convert", "./missing-chart"}, 3},
		{[]string{"convert", "../../testdata/charts/broken-render"}, 4},
		{[]string{"convert", simple, "-o", "cli_test.go"}, 6},
		{[]string{"convert", simple, "--expose", "nothing.*"}, 5},
		{[]string{"validate", "../../internal/kroval/testdata/bad-hyphen-id.yaml"}, 7},
		{[]string{"validate", "../../internal/kroval/testdata/good.yaml"}, 0},
		{[]string{"version"}, 0},
	}

	for _, c := range cases {
		code, _, errOut := run(t, c.args...)
		require.Equal(t, c.code, code, "%v: %s", c.args, errOut)
	}
}

func TestDiffBreakingExit(t *testing.T) {
	dir := t.TempDir()
	code, _, _ := run(t, "convert", simple, "-o", dir, "-q")
	require.Equal(t, 0, code)

	code, out, _ := run(t, "diff", simple, "--against", filepath.Join(dir, "rgd.yaml"), "-q")
	require.Equal(t, 0, code)
	require.Contains(t, out, "no changes")

	code, out, _ = run(t, "diff", simple, "--kind", "Renamed", "--against", filepath.Join(dir, "rgd.yaml"), "-q")
	require.Equal(t, 8, code)
	require.Contains(t, out, "breaking")
}

func TestInspectValue(t *testing.T) {
	code, out, _ := run(t, "inspect", simple, "--value", "service.port", "-q")
	require.Equal(t, 0, code)
	require.Contains(t, out, "exposed")
	require.Contains(t, out, "service.spec.ports[0].port")

	code, out, _ = run(t, "inspect", simple, "--format", "json", "-q")
	require.Equal(t, 0, code)
	require.True(t, strings.Contains(out, `"schemaVersion": "chart2kro.io/report/v1"`))
}

func TestRemovedCommandsAreGone(t *testing.T) {
	for _, c := range []string{"audit", "docs", "export", "watch", "plan"} {
		code, _, _ := run(t, c)
		require.Equal(t, 2, code, c)
	}

	code, _, _ := run(t, "convert", simple, "--harden")
	require.Equal(t, 2, code)
}
