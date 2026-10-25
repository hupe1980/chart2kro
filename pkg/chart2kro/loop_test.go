package chart2kro

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const loops = "../../testdata/charts/loops"

func outcomes(r *Report) map[string]ValueReport {
	m := map[string]ValueReport{}
	for _, v := range r.Values {
		m[v.Path] = v
	}

	return m
}

// Supported loops become collections; unsupported shapes keep a reason.
func TestLoopsBecomeCollections(t *testing.T) {
	rep, err := Inspect(context.Background(), Options{Chart: loops})
	require.NoError(t, err)

	got := outcomes(rep)

	for _, p := range []string{"tenants", "credentials", "queues", "workers", "extraQueues", "workloads.apps"} {
		require.Equal(t, "exposed", got[p].Outcome, p)
		require.Equal(t, "collection", got[p].Kind, p)
	}

	for p, reason := range map[string]string{
		"regions": "range-over-map", "teams": "conditional-loop-body", "zones": "loop-shares-template",
		"groups": "nested-loop", "profiles": "loop-shape-unknown",
		"extraArgs": "changes-structure", // an empty list feeding fields is not a loop
	} {
		require.Equal(t, reason, got[p].Reason, p)
	}

	// A value used only inside an empty-default loop is probed with an example entry.
	require.Equal(t, "exposed", got["loopImage"].Outcome)

	forEach := map[string]string{}
	for _, r := range rep.Resources {
		forEach[r.ID] = r.ForEach
	}

	require.Equal(t, "spec.tenants", forEach["tenantConfigMap"])
	require.Equal(t, "spec.queues", forEach["queueSecret"])
}

// A user-supplied list whose entries land in Secret data is never baked in.
func TestLoopUserSecretNeverBaked(t *testing.T) {
	const secret = "Planted-Loop-Secret"

	f := filepath.Join(t.TempDir(), "creds.yaml")
	require.NoError(t, os.WriteFile(f, []byte("credentials:\n  - name: db\n    password: "+secret+"\n"), 0o600))

	res, err := Convert(context.Background(), Options{Chart: loops, ValueFiles: []string{f}})
	require.NoError(t, err)

	rep, err := json.Marshal(res.Report)
	require.NoError(t, err)

	for _, b := range [][]byte{res.RGD, res.Instance, rep} {
		require.NotContains(t, string(b), secret)
	}

	require.Contains(t, string(res.RGD), "credentials: '[]CredentialsEntry | required=true")
}

// Isolation, gates, readiness and --exclude apply to collections.
func TestCollectionsKeepSingleObjectRules(t *testing.T) {
	res, err := Convert(context.Background(), Options{Chart: loops})
	require.NoError(t, err)

	rgd := string(res.RGD)
	require.Contains(t, rgd, "readyWhen:\n        - ${each.status.?availableReplicas")
	require.Contains(t, rgd, "includeWhen:\n        - ${schema.spec.workloads.enabled}")

	isolated := false

	for _, r := range res.Report.Resources {
		if r.ID == "appDeployment" {
			isolated = r.Isolated
		}
	}

	require.True(t, isolated, "the Deployment collection's selector is isolated per instance")

	ex, err := Convert(context.Background(), Options{Chart: loops, Exclude: []string{"tenantConfigMap"}})
	require.NoError(t, err)
	require.NotContains(t, string(ex.RGD), "tenantConfigMap")
	require.NotContains(t, string(ex.RGD), "${schema.metadata.name}-alpha", "the default tenant does not come back as a single resource")
	require.NotContains(t, ex.Report.Warnings, "--exclude tenantConfigMap matches no resource (see 'chart2kro inspect' for ids)")
}

// A release's list goes into the instance and is checked against the element type.
func TestInstanceWithCollection(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "good.yaml")
	require.NoError(t, os.WriteFile(good, []byte("tenants:\n  - {name: a, quota: \"1\", tier: gold}\n  - {name: b, quota: \"2\", tier: free}\n"), 0o600))

	b, err := Instance(context.Background(), Options{Chart: loops}, InstanceOptions{Name: "x", ValueFiles: []string{good}})
	require.NoError(t, err)
	require.Contains(t, string(b), "name: b")

	bad := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("tenants:\n  - {name: a, quota: \"1\"}\n"), 0o600))

	_, err = Instance(context.Background(), Options{Chart: loops}, InstanceOptions{Name: "x", ValueFiles: []string{bad}})
	require.Equal(t, 5, ExitCode(err))
	require.ErrorContains(t, err, "tenants: want []TenantsEntry")
}
