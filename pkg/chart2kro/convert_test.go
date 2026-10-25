package chart2kro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite golden files")

var fixtures = []string{"simple", "features", "with-database", "with-hooks", "with-subchart"}

func convertFixture(t *testing.T, name string, o Options) *Result {
	t.Helper()

	o.Chart = filepath.Join("..", "..", "testdata", "charts", name)

	res, err := Convert(context.Background(), o)
	require.NoError(t, err)

	return res
}

func TestGolden(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			res := convertFixture(t, name, Options{})
			rep, err := json.MarshalIndent(res.Report, "", "  ")
			require.NoError(t, err)

			files := map[string][]byte{"rgd.yaml": res.RGD, "instance.yaml": res.Instance, "report.json": append(rep, '\n')}
			if len(res.Prerequisites) > 0 {
				files["prerequisites.yaml"] = res.Prerequisites
			}

			dir := filepath.Join("..", "..", "testdata", "golden", name)

			for f, got := range files {
				path := filepath.Join(dir, f)

				if *update {
					require.NoError(t, os.MkdirAll(dir, 0o750))
					require.NoError(t, os.WriteFile(path, got, 0o600))

					continue
				}

				want, err := os.ReadFile(path)
				require.NoError(t, err, "missing golden file; run go test ./pkg/chart2kro -update")
				require.Equal(t, string(want), string(got), path)
			}
		})
	}
}

func TestDeterministic(t *testing.T) {
	for _, name := range fixtures {
		a := convertFixture(t, name, Options{})
		b := convertFixture(t, name, Options{})
		require.Equal(t, string(a.RGD), string(b.RGD), name)
		require.Equal(t, string(a.Prerequisites), string(b.Prerequisites), name)
		// No provenance or timestamps in the output: only the chart's name and version.
		require.NotRegexp(t, `\d{4}-\d{2}-\d{2}T\d{2}:|chart2kro\.io/(version|commit|built|provenance)`, string(a.RGD), name)
	}
}

// A secret the operator supplies must never become a default or appear in any output.
func TestUserSecretNeverBaked(t *testing.T) {
	const secret = "s3cr3t-Planted-Value"

	res := convertFixture(t, "with-database", Options{Set: []string{"postgresql.auth.password=" + secret}})

	rep, err := json.Marshal(res.Report)
	require.NoError(t, err)

	for _, b := range [][]byte{res.RGD, res.Prerequisites, res.Instance, rep} {
		require.NotContains(t, string(b), secret)
		require.NotContains(t, string(b), "czNjcjN0LVBsYW50ZWQtVmFsdWU") // base64 of the secret
	}
}

// The same holds for Secrets that exist only behind a flag (regression: corpus, wordpress).
func TestUserSecretInConditionalSecret(t *testing.T) {
	const secret = "Planted-Conditional-Secret"

	res := convertFixture(t, "features", Options{SetString: []string{"credentials.username=" + secret}})

	rep, err := json.Marshal(res.Report)
	require.NoError(t, err)

	for _, b := range [][]byte{res.RGD, res.Prerequisites, res.Instance, rep} {
		require.NotContains(t, string(b), secret)
	}

	require.Contains(t, string(res.RGD), "username: string | required=true")
}

func TestExitCodes(t *testing.T) {
	ctx := context.Background()

	_, err := Convert(ctx, Options{Chart: "does-not-exist"})
	require.Equal(t, 3, ExitCode(err))

	_, err = Convert(ctx, Options{Chart: "../../testdata/charts/library"})
	require.Equal(t, 3, ExitCode(err))

	_, err = Convert(ctx, Options{Chart: "../../testdata/charts/simple", Kind: "not-a-kind"})
	require.Equal(t, 2, ExitCode(err))

	_, err = Convert(ctx, Options{Chart: "../../testdata/charts/simple", Expose: []string{"nope.*"}})
	require.Equal(t, 5, ExitCode(err))

	require.Equal(t, 1, ExitCode(errors.New("x")))
	require.Equal(t, 0, ExitCode(nil))
}

func TestValidateRejectsAndAccepts(t *testing.T) {
	res := convertFixture(t, "simple", Options{})
	require.NoError(t, Validate(context.Background(), res.RGD, ValidateOptions{}))

	bad := bytes.Replace(res.RGD, []byte("id: deployment"), []byte("id: my-deployment"), 1)
	err := Validate(context.Background(), bad, ValidateOptions{})
	require.Equal(t, 7, ExitCode(err))
	require.True(t, strings.Contains(err.Error(), "lower camelCase"))
}

func TestDiffAgainstItself(t *testing.T) {
	res := convertFixture(t, "simple", Options{})

	d, err := Diff(res.RGD, res.RGD)
	require.NoError(t, err)
	require.False(t, d.Breaking())

	other := convertFixture(t, "simple", Options{Kind: "Other"})
	d, err = Diff(res.RGD, other.RGD)
	require.NoError(t, err)
	require.True(t, d.Breaking())
}

func TestInstanceFromValues(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "release.yaml")
	require.NoError(t, os.WriteFile(f, []byte("replicaCount: 3\nimage:\n  tag: \"1.27\"\n"), 0o600))

	b, err := Instance(context.Background(), Options{Chart: "../../testdata/charts/simple"},
		InstanceOptions{Name: "web", Namespace: "prod", ValueFiles: []string{f}})
	require.NoError(t, err)
	require.Contains(t, string(b), "replicaCount: 3")
	require.Contains(t, string(b), `tag: "1.27"`)
	require.NotContains(t, string(b), "repository") // unchanged values are not repeated

	require.NoError(t, os.WriteFile(f, []byte("unknownValue: 1\n"), 0o600))
	_, err = Instance(context.Background(), Options{Chart: "../../testdata/charts/simple"},
		InstanceOptions{Name: "web", ValueFiles: []string{f}})
	require.Equal(t, 5, ExitCode(err))
}

func TestDiffNeedsExactlyOneRGD(t *testing.T) {
	one := []byte("apiVersion: kro.run/v1alpha1\nkind: ResourceGraphDefinition\nmetadata:\n  name: a\nspec: {}\n")
	none := []byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: x\n")
	two := append(append(append([]byte{}, one...), "---\n"...), one...)

	_, err := Diff(none, one)
	require.Equal(t, 2, ExitCode(err))

	_, err = Diff(two, one)
	require.Equal(t, 2, ExitCode(err))
	require.ErrorContains(t, err, "2 ResourceGraphDefinitions")

	res, err := Diff(one, one)
	require.NoError(t, err)
	require.Equal(t, DiffSchema, res.SchemaVersion)
}
