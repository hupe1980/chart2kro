package harden

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

func TestParseSecurityLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected SecurityLevel
		wantErr  bool
	}{
		{"", SecurityLevelNone, false},
		{"none", SecurityLevelNone, false},
		{"baseline", SecurityLevelBaseline, false},
		{"restricted", SecurityLevelRestricted, false},
		{"RESTRICTED", SecurityLevelRestricted, false},
		{"  Baseline ", SecurityLevelBaseline, false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseSecurityLevel(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestNew_EmptyConfig(t *testing.T) {
	h := New(Config{})
	result, err := h.Harden(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, result.Changes)
	assert.Empty(t, result.Warnings)
}

func TestNew_AllPoliciesEnabled(t *testing.T) {
	h := New(Config{
		SecurityLevel:           SecurityLevelRestricted,
		ResourceDefaults:        DefaultResourceDefaults,
		ImagePolicy:             &ImagePolicyConfig{DenyLatestTag: true},
		GenerateNetworkPolicies: true,
		GenerateRBAC:            true,
		ResourceIDs:             map[*k8s.Resource]string{},
	})
	deploy := makeDeployment("app", []interface{}{makeContainer("web", "nginx:1.25")})
	result, err := h.Harden(context.Background(), []*k8s.Resource{deploy})
	require.NoError(t, err)
	assert.True(t, len(result.Changes) > 0, "expected changes from hardening")
}

func TestHarden_NonWorkloadsSkipped(t *testing.T) {
	h := New(Config{SecurityLevel: SecurityLevelRestricted})
	configMap := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
		Name: "my-cm",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "my-cm"},
				"data":       map[string]interface{}{"key": "val"},
			},
		},
	}
	result, err := h.Harden(context.Background(), []*k8s.Resource{configMap})
	require.NoError(t, err)
	assert.Empty(t, result.Changes)
}

func TestHarden_PolicyOrdering(t *testing.T) {
	h := New(Config{
		SecurityLevel:           SecurityLevelRestricted,
		ResourceDefaults:        DefaultResourceDefaults,
		ImagePolicy:             &ImagePolicyConfig{DenyLatestTag: true},
		GenerateNetworkPolicies: true,
		GenerateRBAC:            true,
		ResourceIDs:             map[*k8s.Resource]string{},
	})
	assert.Len(t, h.policies, 5)
	assert.Equal(t, "pod-security-standards", h.policies[0].Name())
	assert.Equal(t, "resource-requirements", h.policies[1].Name())
	assert.Equal(t, "image-policy", h.policies[2].Name())
	assert.Equal(t, "network-policy-generator", h.policies[3].Name())
	assert.Equal(t, "rbac-generator", h.policies[4].Name())
}

func TestResult_AccumulatesChanges(t *testing.T) {
	h := New(Config{
		SecurityLevel:    SecurityLevelRestricted,
		ResourceDefaults: DefaultResourceDefaults,
		ImagePolicy:      &ImagePolicyConfig{DenyLatestTag: true},
	})
	deploy := makeDeployment("app", []interface{}{makeContainer("web", "nginx")})
	result, err := h.Harden(context.Background(), []*k8s.Resource{deploy})
	require.NoError(t, err)
	assert.True(t, len(result.Changes) > 5, "expected multiple changes, got %d", len(result.Changes))
	assert.True(t, len(result.Warnings) > 0, "expected warnings for latest tag")
}

func TestParseFileConfig_Full(t *testing.T) {
	data := []byte(`
harden:
  enabled: true
  security-level: restricted
  generate-network-policies: true
  generate-rbac: true
  images:
    deny-latest-tag: true
    allowed-registries:
      - gcr.io/
      - quay.io/
    require-digests: true
  resources:
    cpu-request: "200m"
    memory-request: "256Mi"
    cpu-limit: "1"
    memory-limit: "1Gi"
`)
	fc, err := ParseFileConfig(data)
	require.NoError(t, err)
	require.NotNil(t, fc)
	assert.True(t, fc.Enabled)
	assert.Equal(t, "restricted", fc.SecurityLevel)
	assert.True(t, fc.GenerateNetworkPolicies)
	assert.True(t, fc.GenerateRBAC)
	require.NotNil(t, fc.Images)
	assert.True(t, fc.Images.DenyLatestTag)
	assert.Equal(t, []string{"gcr.io/", "quay.io/"}, fc.Images.AllowedRegistries)
	assert.True(t, fc.Images.RequireDigests)
	require.NotNil(t, fc.Resources)
	assert.Equal(t, "200m", fc.Resources.CPURequest)
	assert.Equal(t, "256Mi", fc.Resources.MemoryRequest)
	assert.Equal(t, "1", fc.Resources.CPULimit)
	assert.Equal(t, "1Gi", fc.Resources.MemoryLimit)
}

func TestParseFileConfig_NoHardenSection(t *testing.T) {
	data := []byte(`log-level: info`)
	fc, err := ParseFileConfig(data)
	require.NoError(t, err)
	assert.Nil(t, fc)
}

func TestParseFileConfig_InvalidYAML(t *testing.T) {
	data := []byte(`harden: [invalid`)
	_, err := ParseFileConfig(data)
	require.Error(t, err)
}

func TestFileConfig_ToImagePolicyConfig(t *testing.T) {
	fc := &FileConfig{
		Images: &FileImageConfig{
			DenyLatestTag:     true,
			AllowedRegistries: []string{"gcr.io/"},
			RequireDigests:    true,
		},
	}
	ipc := fc.ToImagePolicyConfig()
	require.NotNil(t, ipc)
	assert.True(t, ipc.DenyLatestTag)
	assert.Equal(t, []string{"gcr.io/"}, ipc.AllowedRegistries)
	assert.True(t, ipc.RequireDigests)
}

func TestFileConfig_ToImagePolicyConfig_Nil(t *testing.T) {
	fc := &FileConfig{}
	assert.Nil(t, fc.ToImagePolicyConfig())
}

func TestFileConfig_ToResourceDefaultsConfig(t *testing.T) {
	fc := &FileConfig{
		Resources: &FileResourceConfig{
			CPURequest:    "100m",
			MemoryRequest: "128Mi",
			CPULimit:      "500m",
			MemoryLimit:   "512Mi",
		},
	}
	rdc := fc.ToResourceDefaultsConfig()
	require.NotNil(t, rdc)
	assert.Equal(t, "100m", rdc.CPURequest)
	assert.Equal(t, "128Mi", rdc.MemoryRequest)
}

func TestFileConfig_ToResourceDefaultsConfig_Nil(t *testing.T) {
	fc := &FileConfig{}
	assert.Nil(t, fc.ToResourceDefaultsConfig())
}
