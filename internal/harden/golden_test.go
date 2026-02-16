package harden

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestGoldenFile_FullPipeline runs an unhardened Deployment through the full
// hardening pipeline and verifies the output matches expected hardened state.
func TestGoldenFile_FullPipeline(t *testing.T) {
	// --- Input: unhardened Deployment ---
	input := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "web-app",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "web-app"},
				"spec": map[string]interface{}{
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app": "web-app",
						},
					},
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "app",
									"image": "nginx:1.25",
									"ports": []interface{}{
										map[string]interface{}{
											"containerPort": int64(8080),
											"protocol":      "TCP",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	svc := makeService("web-app-svc",
		map[string]interface{}{"app": "web-app"},
		[]interface{}{
			map[string]interface{}{
				"port":       int64(80),
				"targetPort": int64(8080),
				"protocol":   "TCP",
			},
		},
	)

	resourceIDs := map[*k8s.Resource]string{
		input: "webApp",
		svc:   "webAppSvc",
	}

	mockClient := &mockRegistryClient{
		digests: map[string]string{
			"nginx:1.25": "sha256:abc123def456",
		},
	}

	cfg := Config{
		SecurityLevel:           SecurityLevelRestricted,
		ResourceDefaults:        DefaultResourceDefaults,
		ImagePolicy:             &ImagePolicyConfig{DenyLatestTag: true, RequireDigests: true},
		GenerateNetworkPolicies: true,
		GenerateRBAC:            true,
		ResolveDigests:          true,
		RegistryClient:          mockClient,
		ResourceIDs:             resourceIDs,
	}

	hardener := New(cfg)
	result, err := hardener.Harden(context.Background(), []*k8s.Resource{input, svc})
	require.NoError(t, err)

	// --- Verify: PSS restricted fields ---
	podSpec := getPodSpec(input)
	require.NotNil(t, podSpec, "podSpec should exist")

	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})

	sc, ok := container["securityContext"].(map[string]interface{})
	require.True(t, ok, "securityContext should be set")
	assert.Equal(t, true, sc["runAsNonRoot"])
	assert.Equal(t, true, sc["readOnlyRootFilesystem"])
	assert.Equal(t, false, sc["allowPrivilegeEscalation"])

	caps, ok := sc["capabilities"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, []interface{}{"ALL"}, caps["drop"])

	// --- Verify: Resource defaults injected ---
	resources, ok := container["resources"].(map[string]interface{})
	require.True(t, ok)

	requests := resources["requests"].(map[string]interface{})
	assert.Equal(t, "100m", requests["cpu"])
	assert.Equal(t, "128Mi", requests["memory"])

	limits := resources["limits"].(map[string]interface{})
	assert.Equal(t, "500m", limits["cpu"])
	assert.Equal(t, "512Mi", limits["memory"])

	// --- Verify: Image digest resolved ---
	assert.Equal(t, "nginx@sha256:abc123def456", container["image"])

	// --- Verify: Generated resources ---
	// Should have: original deploy + svc + NetworkPolicy + SA + Role + RoleBinding = 6.
	assert.GreaterOrEqual(t, len(result.Resources), 5)

	// Find generated NetworkPolicy.
	var foundNetpol, foundSA, foundRole, foundRB bool

	for _, res := range result.Resources {
		switch res.Kind() {
		case "NetworkPolicy":
			foundNetpol = true
		case "ServiceAccount":
			foundSA = true
		case "Role":
			foundRole = true
		case "RoleBinding":
			foundRB = true
		}
	}

	assert.True(t, foundNetpol, "expected NetworkPolicy to be generated")
	assert.True(t, foundSA, "expected ServiceAccount to be generated")
	assert.True(t, foundRole, "expected Role to be generated")
	assert.True(t, foundRB, "expected RoleBinding to be generated")

	// --- Verify: Changes and warnings recorded ---
	assert.True(t, len(result.Changes) > 0, "expected hardening changes")
	// Image warnings: requireDigests warns on tag before digest resolver runs.
	assert.True(t, len(result.Warnings) > 0, "expected hardening warnings")
}

func TestNew_WithDigestResolver(t *testing.T) {
	client := &mockRegistryClient{}

	h := New(Config{
		ResolveDigests: true,
		RegistryClient: client,
	})

	assert.Len(t, h.policies, 1)
	assert.Equal(t, "digest-resolver", h.policies[0].Name())
}

func TestNew_WithDigestResolver_DefaultClient(t *testing.T) {
	h := New(Config{
		ResolveDigests: true,
		// RegistryClient nil → default.
	})

	assert.Len(t, h.policies, 1)
	assert.Equal(t, "digest-resolver", h.policies[0].Name())
}

func TestNew_PolicyOrdering_WithDigestResolver(t *testing.T) {
	h := New(Config{
		SecurityLevel:           SecurityLevelRestricted,
		ResourceDefaults:        DefaultResourceDefaults,
		ImagePolicy:             &ImagePolicyConfig{DenyLatestTag: true},
		ResolveDigests:          true,
		RegistryClient:          &mockRegistryClient{},
		GenerateNetworkPolicies: true,
		GenerateRBAC:            true,
		ResourceIDs:             map[*k8s.Resource]string{},
	})

	assert.Len(t, h.policies, 6)
	assert.Equal(t, "pod-security-standards", h.policies[0].Name())
	assert.Equal(t, "resource-requirements", h.policies[1].Name())
	assert.Equal(t, "image-policy", h.policies[2].Name())
	assert.Equal(t, "digest-resolver", h.policies[3].Name())
	assert.Equal(t, "network-policy-generator", h.policies[4].Name())
	assert.Equal(t, "rbac-generator", h.policies[5].Name())
}

func TestHarden_PolicyFailure_ReturnsError(t *testing.T) {
	client := &mockRegistryClient{
		err: fmt.Errorf("registry unreachable"),
	}

	h := New(Config{
		ResolveDigests: true,
		RegistryClient: client,
	})

	deploy := makeDeployment("app", []interface{}{makeContainer("web", "nginx:1.25")})

	_, err := h.Harden(context.Background(), []*k8s.Resource{deploy})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "policy digest-resolver")
	assert.Contains(t, err.Error(), "registry unreachable")
}

func TestParseFileConfig_WithRequireLimits(t *testing.T) {
	data := []byte(`
harden:
  resources:
    cpu-limit: "500m"
    memory-limit: "512Mi"
    require-limits: true
`)
	fc, err := ParseFileConfig(data)
	require.NoError(t, err)
	require.NotNil(t, fc)
	require.NotNil(t, fc.Resources)
	assert.True(t, fc.Resources.RequireLimits)

	rdc := fc.ToResourceDefaultsConfig()
	require.NotNil(t, rdc)
	assert.True(t, rdc.RequireLimits)
	assert.Equal(t, "500m", rdc.CPULimit)
}
