package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const features = "../../testdata/charts/features"

func TestInspectTextAndExplain(t *testing.T) {
	code, out, _ := run(t, "inspect", features)
	require.Equal(t, 0, code)
	require.Contains(t, out, "VALUE")
	require.Contains(t, out, "RESOURCE")
	require.Contains(t, out, "${schema.spec.ingress.enabled}")

	code, out, _ = run(t, "inspect", features, "--value", "shout", "-q")
	require.Equal(t, 0, code)
	require.Contains(t, out, "transformed")

	code, _, _ = run(t, "inspect", features, "--value", "nope", "-q")
	require.Equal(t, 2, code)

	code, _, _ = run(t, "inspect", features, "--format", "yaml", "-q")
	require.Equal(t, 2, code)
}

func TestConvertSummaryAndReport(t *testing.T) {
	report := filepath.Join(t.TempDir(), "r.json")
	code, out, errOut := run(t, "convert", features, "--report", report)
	require.Equal(t, 0, code)
	require.Contains(t, out, "kind: CustomResourceDefinition") // prerequisites first on stdout
	require.Contains(t, errOut, "prerequisite (install once)")
	// A password the chart generates must be supplied per instance, never derived.
	require.Contains(t, out, "password: ${schema.spec.chart2kroGenerated.secret.password}")
	require.Contains(t, errOut, "required input: spec.chart2kroGenerated.secret.password (generated)")

	b, err := os.ReadFile(report)
	require.NoError(t, err)
	require.Contains(t, string(b), `"prerequisites"`)

	code, _, _ = run(t, "convert", features, "--check")
	require.Equal(t, 2, code)
}

func TestDiffJSONAndInstance(t *testing.T) {
	dir := t.TempDir()
	code, _, _ := run(t, "convert", simple, "-o", dir, "-q")
	require.Equal(t, 0, code)

	code, out, _ := run(t, "diff", simple, "--kind", "Other", "--against", filepath.Join(dir, "rgd.yaml"), "--format", "json", "-q")
	require.Equal(t, 8, code)
	require.Contains(t, out, `"severity": "breaking"`)

	code, _, _ = run(t, "diff", simple, "-q")
	require.Equal(t, 2, code)

	vals := filepath.Join(dir, "release.yaml")
	require.NoError(t, os.WriteFile(vals, []byte("replicaCount: 4\n"), 0o600))

	code, out, _ = run(t, "instance", simple, "--from", vals, "--name", "web", "--namespace", "prod", "-q")
	require.Equal(t, 0, code)
	require.Contains(t, out, "replicaCount: 4")

	code, _, _ = run(t, "instance", simple, "--name", "web", "-q")
	require.Equal(t, 2, code)

	code, _, errOut := run(t, "instance", simple, "--from", vals, "--name", "Web_1", "-q")
	require.Equal(t, 2, code)
	require.Contains(t, errOut, "DNS-1123")

	// A saved stdout stream (prerequisites first) works as --against.
	stream := filepath.Join(dir, "stream.yaml")
	rgdYAML, err := os.ReadFile(filepath.Join(dir, "rgd.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(stream, append([]byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: x\n---\n"), rgdYAML...), 0o600))

	code, out, _ = run(t, "diff", simple, "--against", stream, "--format", "json", "-q")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"schemaVersion": "chart2kro.io/diff/v1"`)
}

func TestValidateJSON(t *testing.T) {
	dir := t.TempDir()
	code, _, _ := run(t, "convert", simple, "-o", dir, "-q")
	require.Equal(t, 0, code)

	code, out, _ := run(t, "validate", filepath.Join(dir, "rgd.yaml"), "--format", "json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"valid": true`)

	bad := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("apiVersion: kro.run/v1alpha1\nkind: ResourceGraphDefinition\nmetadata:\n  name: x\nspec:\n  schema:\n    kind: lower\n"), 0o600))

	code, out, _ = run(t, "validate", bad, "--format", "json")
	require.Equal(t, 7, code)
	require.Contains(t, out, `"valid": false`)
	require.NotContains(t, out, `"problems": []`)

	code, _, _ = run(t, "validate", bad, "--format", "yaml")
	require.Equal(t, 2, code)
}

func TestQuietSilencesWarnings(t *testing.T) {
	code, _, errOut := run(t, "convert", simple, "--exclude", "nope", "-q", "--log-level", "warn")
	require.Equal(t, 0, code)
	require.Empty(t, errOut)
}

func TestVersionAndGlobals(t *testing.T) {
	code, out, _ := run(t, "version", "--format", "json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"kro"`)

	code, out, _ = run(t, "version")
	require.Equal(t, 0, code)
	require.True(t, strings.HasPrefix(out, "chart2kro "))

	code, _, _ = run(t, "convert", simple, "--log-level", "loud")
	require.Equal(t, 2, code)

	code, _, _ = run(t, "convert", simple, "--log-format", "xml")
	require.Equal(t, 2, code)

	code, _, _ = run(t, "validate")
	require.Equal(t, 2, code)
}
