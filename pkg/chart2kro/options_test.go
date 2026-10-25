package chart2kro

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const features = "../../testdata/charts/features"

func TestStrictExposeExplainsWhy(t *testing.T) {
	_, err := Convert(context.Background(), Options{Chart: features, Expose: []string{"shout", "replicaCount"}})
	require.Equal(t, 5, ExitCode(err))
	require.Contains(t, err.Error(), "shout (transformed")
	require.NotContains(t, err.Error(), "replicaCount")
}

func TestExposeLimitsTheAPI(t *testing.T) {
	res, err := Convert(context.Background(), Options{Chart: features, Expose: []string{"replicaCount", "ingress.*"}})
	require.NoError(t, err)
	require.Equal(t, 3, res.Report.Summary.Exposed)
	require.NotContains(t, string(res.RGD), "schema.spec.image")
}

func TestExcludeAndHooks(t *testing.T) {
	res, err := Convert(context.Background(), Options{Chart: features, Exclude: []string{"widget"}})
	require.NoError(t, err)
	require.NotContains(t, string(res.RGD), "kind: Widget")

	res, err = Convert(context.Background(), Options{Chart: "../../testdata/charts/with-database", ExcludeSubcharts: []string{"postgresql"}})
	require.NoError(t, err)
	require.NotContains(t, string(res.RGD), "kind: StatefulSet")

	hooks, err := Convert(context.Background(), Options{Chart: "../../testdata/charts/with-hooks", IncludeHooks: true})
	require.NoError(t, err)

	without, err := Convert(context.Background(), Options{Chart: "../../testdata/charts/with-hooks"})
	require.NoError(t, err)
	require.Greater(t, len(hooks.Report.Resources), len(without.Report.Resources))
	require.True(t, anyContains(hooks.Report.Warnings, "kept as an ordinary resource"), hooks.Report.Warnings)

	// A value that only affects dropped hooks has no effect on instances: unused, not frozen.
	for _, v := range without.Report.Values {
		if v.Path == "image" {
			require.Equal(t, "unused", v.Outcome)
			require.Equal(t, "affects-only-dropped-objects", v.Reason)
		}
	}

	res, err = Convert(context.Background(), Options{Chart: "../../testdata/charts/simple", Exclude: []string{"nope"}})
	require.NoError(t, err)
	require.True(t, anyContains(res.Report.Warnings, "--exclude nope matches no resource"), res.Report.Warnings)
}

func anyContains(list []string, s string) bool {
	for _, x := range list {
		if strings.Contains(x, s) {
			return true
		}
	}

	return false
}

func TestOptionValidation(t *testing.T) {
	for _, o := range []Options{
		{},
		{Chart: features, Group: "Bad Group"},
		{Chart: features, APIVersion: "version1"},
		{Chart: features, Expose: []string{"image.[tag"}},
	} {
		_, err := Convert(context.Background(), o)
		require.Equal(t, 2, ExitCode(err), "%+v", o)
	}

	_, err := Convert(context.Background(), Options{Chart: features, KubeVersion: "1.2"})
	require.Error(t, err)

	_, err = Convert(context.Background(), Options{Chart: features, ValueFiles: []string{"/missing.yaml"}})
	require.Equal(t, 2, ExitCode(err))

	_, err = Convert(context.Background(), Options{Chart: features, CRDFiles: []string{"/missing.yaml"}})
	require.Equal(t, 2, ExitCode(err))
}

func TestValidateWithCRDs(t *testing.T) {
	res, err := Convert(context.Background(), Options{Chart: features})
	require.NoError(t, err)

	// Without the prerequisites the Widget kind is unknown.
	err = Validate(context.Background(), res.RGD, ValidateOptions{})
	require.Equal(t, 7, ExitCode(err))

	// CRDs in the same stream, or passed as files, resolve it.
	require.NoError(t, Validate(context.Background(), append(append(res.Prerequisites, []byte("\n---\n")...), res.RGD...), ValidateOptions{}))

	crd := filepath.Join(t.TempDir(), "crd.yaml")
	require.NoError(t, os.WriteFile(crd, res.Prerequisites, 0o600))
	require.NoError(t, Validate(context.Background(), res.RGD, ValidateOptions{CRDFiles: []string{crd}}))
}

func TestInspectAndInstanceAllowUnexposed(t *testing.T) {
	rep, err := Inspect(context.Background(), Options{Chart: features})
	require.NoError(t, err)
	require.Equal(t, ReportSchema, rep.SchemaVersion)

	f := filepath.Join(t.TempDir(), "v.yaml")
	require.NoError(t, os.WriteFile(f, []byte("shout: LOUD\nreplicaCount: 5\n"), 0o600))

	var logs bytes.Buffer

	log := slog.New(slog.NewTextHandler(&logs, nil))

	b, err := Instance(context.Background(), Options{Chart: features, Logger: log}, InstanceOptions{Name: "x", ValueFiles: []string{f}, AllowUnexposed: true})
	require.NoError(t, err)
	require.Contains(t, string(b), "replicaCount: 5")
	require.False(t, strings.Contains(string(b), "LOUD"))
	require.Contains(t, logs.String(), "shout (frozen: transformed)")

	g := filepath.Join(t.TempDir(), "g.yaml")
	require.NoError(t, os.WriteFile(g, []byte("shout: LOUD\nnoSuchValue: 1\n"), 0o600))

	_, err = Instance(context.Background(), Options{Chart: features}, InstanceOptions{Name: "x", ValueFiles: []string{g}})
	require.Equal(t, 5, ExitCode(err))
	require.ErrorContains(t, err, "shout (frozen: transformed)")
	require.ErrorContains(t, err, "noSuchValue (not a chart value)")

	_, err = Instance(context.Background(), Options{Chart: features}, InstanceOptions{Name: strings.Repeat("a", 254), ValueFiles: []string{f}})
	require.Equal(t, 2, ExitCode(err))

	_, err = Instance(context.Background(), Options{Chart: features}, InstanceOptions{})
	require.Equal(t, 2, ExitCode(err))
}

func TestCredentialsNeverLogged(t *testing.T) {
	var logs bytes.Buffer

	log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	_, err := Convert(context.Background(), Options{
		Chart: features, Username: "user", Password: "Pl4nted-Passw0rd", Logger: log,
	})
	require.NoError(t, err)
	require.NotEmpty(t, logs.String())
	require.NotContains(t, logs.String(), "Pl4nted-Passw0rd")
}

func TestInstanceChecksTypesAndWarnsOnSecrets(t *testing.T) {
	db := Options{Chart: "../../testdata/charts/with-database"}
	dir := t.TempDir()

	bad := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("replicaCount: \"two\"\n"), 0o600))

	_, err := Instance(context.Background(), db, InstanceOptions{Name: "x", ValueFiles: []string{bad}})
	require.Equal(t, 5, ExitCode(err))
	require.ErrorContains(t, err, "replicaCount: want integer, got string")

	pw := filepath.Join(dir, "pw.yaml")
	require.NoError(t, os.WriteFile(pw, []byte("postgresql:\n  auth:\n    password: s3cret\n"), 0o600))

	var logs bytes.Buffer

	db.Logger = slog.New(slog.NewTextHandler(&logs, nil))

	b, err := Instance(context.Background(), db, InstanceOptions{Name: "x", ValueFiles: []string{pw}})
	require.NoError(t, err)
	require.Contains(t, string(b), "s3cret")
	require.Contains(t, logs.String(), "lands in a Secret")
}
