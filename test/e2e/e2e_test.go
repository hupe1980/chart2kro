//go:build e2e

// Package e2e applies converted charts to a real kro and checks instances. It expects
// KUBECONFIG to point at a cluster with kro installed (see `just e2e`). Test code may
// call kubectl; chart2kro itself never starts processes.
package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/pkg/chart2kro"
)

func kubectl(t *testing.T, stdin string, args ...string) string {
	t.Helper()

	cmd := exec.Command("kubectl", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "kubectl %v: %s", args, out)

	return strings.TrimSpace(string(out))
}

func eventually(t *testing.T, what string, timeout time.Duration, fn func() (bool, string)) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	var last string

	for time.Now().Before(deadline) {
		ok, s := fn()
		if ok {
			return
		}

		last = s

		time.Sleep(3 * time.Second)
	}

	t.Fatalf("timed out waiting for %s (last: %s)", what, last)
}

func apply(t *testing.T, name string) *chart2kro.Result {
	t.Helper()

	res, err := chart2kro.Convert(context.Background(), chart2kro.Options{Chart: filepath.Join("..", "..", "testdata", "charts", name)})
	require.NoError(t, err)

	if len(res.Prerequisites) > 0 {
		kubectl(t, string(res.Prerequisites), "apply", "-f", "-")
	}

	kubectl(t, string(res.RGD), "apply", "-f", "-")

	rgdName := strings.ReplaceAll(name, "_", "-")

	// A reused local cluster may hold a CRD from an older fixture; the fixtures are
	// not a published API, so let kro replace it.
	kubectl(t, "", "annotate", "rgd", rgdName, "kro.run/allow-breaking-changes=true", "--overwrite")
	eventually(t, "RGD "+rgdName+" Active", 2*time.Minute, func() (bool, string) {
		out, err := exec.Command("kubectl", "get", "rgd", rgdName, "-o", "jsonpath={.status.state}").CombinedOutput()
		return err == nil && string(out) == "Active", string(out)
	})

	return res
}

func TestFixturesBecomeActive(t *testing.T) {
	for _, name := range []string{"simple", "features", "with-database", "with-hooks", "with-subchart", "loops"} {
		t.Run(name, func(t *testing.T) { apply(t, name) })
	}
}

func TestInstancesAreIsolated(t *testing.T) {
	apply(t, "simple")

	ns := fmt.Sprintf("e2e-%d", time.Now().Unix())
	kubectl(t, "", "create", "namespace", ns)
	t.Cleanup(func() { _ = exec.Command("kubectl", "delete", "namespace", ns, "--wait=false").Run() })

	inst := func(name, namespace string) string {
		return fmt.Sprintf("apiVersion: kro.run/v1alpha1\nkind: Simple\nmetadata:\n  name: %s\n  namespace: %s\nspec: {}\n", name, namespace)
	}

	for _, x := range [][2]string{{"a", ns}, {"b", ns}, {"c", "default"}} {
		kubectl(t, inst(x[0], x[1]), "apply", "-f", "-")
	}

	t.Cleanup(func() { _ = exec.Command("kubectl", "delete", "simple", "c", "-n", "default", "--wait=false").Run() })

	for _, x := range [][2]string{{"a", ns}, {"b", ns}, {"c", "default"}} {
		eventually(t, "instance "+x[0]+" ACTIVE", 3*time.Minute, func() (bool, string) {
			out, err := exec.Command("kubectl", "get", "simple", x[0], "-n", x[1], "-o", "jsonpath={.status.state}").CombinedOutput()
			return err == nil && string(out) == "ACTIVE", string(out)
		})
	}

	// Every Service selects exactly its own instance's pods, across namespaces too.
	for _, x := range [][2]string{{"a", ns}, {"b", ns}, {"c", "default"}} {
		x, n := x[0], x[1]
		eps := kubectl(t, "", "get", "endpointslices", "-n", n, "-l", "kubernetes.io/service-name="+x+"-simple",
			"-o", "jsonpath={.items[*].endpoints[*].targetRef.name}")
		for _, pod := range strings.Fields(eps) {
			require.True(t, strings.HasPrefix(pod, x+"-simple-"), "service %s-simple selects %s", x, pod)
		}
	}

	// Updating an exposed value changes only that instance.
	kubectl(t, "", "patch", "simple", "a", "-n", ns, "--type", "merge", "-p", `{"spec":{"replicaCount":2}}`)
	eventually(t, "a scaled to 2", 2*time.Minute, func() (bool, string) {
		a, _ := exec.Command("kubectl", "get", "deploy", "a-simple", "-n", ns, "-o", "jsonpath={.spec.replicas}").CombinedOutput()
		return string(a) == "2", string(a)
	})
	require.Equal(t, "1", kubectl(t, "", "get", "deploy", "b-simple", "-n", ns, "-o", "jsonpath={.spec.replicas}"))

	// Deleting an instance removes its objects.
	kubectl(t, "", "delete", "simple", "a", "-n", ns, "--wait=true", "--timeout=2m")
	eventually(t, "a's deployment deleted", 2*time.Minute, func() (bool, string) {
		err := exec.Command("kubectl", "get", "deploy", "a-simple", "-n", ns).Run()
		return err != nil, "still present"
	})
}

// TestMain needs a cluster with kro (kubectl's current context). Without one the
// suite is skipped locally, but fails in CI, where skipping would pass silently.
func TestMain(m *testing.M) {
	if out, err := exec.Command("kubectl", "get", "crd", "resourcegraphdefinitions.kro.run").CombinedOutput(); err != nil {
		if os.Getenv("CI") != "" {
			fmt.Printf("e2e: no cluster with kro reachable: %v\n%s", err, out)
			os.Exit(1)
		}

		fmt.Println("e2e: no cluster with kro reachable; run `just e2e`")
		os.Exit(0)
	}

	os.Exit(m.Run())
}

// Collections: one ConfigMap per tenant, created and removed by kro.
func TestCollectionInstance(t *testing.T) {
	apply(t, "loops")

	ns := fmt.Sprintf("e2e-loops-%d", time.Now().Unix())
	kubectl(t, "", "create", "namespace", ns)
	t.Cleanup(func() { _ = exec.Command("kubectl", "delete", "namespace", ns, "--wait=false").Run() })

	kubectl(t, `apiVersion: kro.run/v1alpha1
kind: Loops
metadata:
  name: three
  namespace: `+ns+`
spec:
  tenants:
    - {name: a, quota: "1", tier: gold}
    - {name: b, quota: "2", tier: free}
    - {name: c, quota: "3", tier: free}
`, "apply", "-f", "-")

	eventually(t, "three tenant ConfigMaps", 3*time.Minute, func() (bool, string) {
		out, _ := exec.Command("kubectl", "get", "configmap", "-n", ns, "-l", "tier", "-o", "jsonpath={.items[*].metadata.name}").CombinedOutput()
		return strings.Fields(string(out)) != nil && len(strings.Fields(string(out))) == 3, string(out)
	})
}
