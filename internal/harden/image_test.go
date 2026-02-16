package harden

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

func TestImagePolicy_DenyLatestTag(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		wantWarn bool
	}{
		{"explicit latest", "nginx:latest", true},
		{"no tag (implicit latest)", "nginx", true},
		{"specific tag", "nginx:1.25", false},
		{"digest", "nginx@sha256:abc123", false},
		{"registry with latest", "registry.example.com/nginx:latest", true},
		{"registry without tag", "registry.example.com/nginx", true},
		{"registry with tag", "registry.example.com/nginx:1.25", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deploy := makeDeployment("app", []interface{}{makeContainer("web", tt.image)})

			policy := NewImagePolicy(&ImagePolicyConfig{DenyLatestTag: true})
			result := &HardenResult{Resources: []*k8s.Resource{deploy}}

			err := policy.Apply(context.Background(), result.Resources, result)
			require.NoError(t, err)

			if tt.wantWarn {
				assert.True(t, len(result.Warnings) > 0, "expected warning for image %s", tt.image)
			} else {
				assert.Empty(t, result.Warnings, "expected no warning for image %s", tt.image)
			}
		})
	}
}

func TestImagePolicy_AllowedRegistries(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		allowed  []string
		wantWarn bool
	}{
		{"allowed", "gcr.io/project/nginx:1.25", []string{"gcr.io/project"}, false},
		{"not allowed", "docker.io/nginx:1.25", []string{"gcr.io/project"}, true},
		{"multiple allowed", "quay.io/org/app:1.0", []string{"gcr.io", "quay.io/org"}, false},
		{"short name", "nginx:1.25", []string{"gcr.io"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deploy := makeDeployment("app", []interface{}{makeContainer("web", tt.image)})

			policy := NewImagePolicy(&ImagePolicyConfig{AllowedRegistries: tt.allowed})
			result := &HardenResult{Resources: []*k8s.Resource{deploy}}

			err := policy.Apply(context.Background(), result.Resources, result)
			require.NoError(t, err)

			if tt.wantWarn {
				assert.True(t, len(result.Warnings) > 0, "expected warning for image %s", tt.image)
			} else {
				assert.Empty(t, result.Warnings, "expected no warning for image %s", tt.image)
			}
		})
	}
}

func TestImagePolicy_RequireDigests(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		wantWarn bool
	}{
		{"tag only", "nginx:1.25", true},
		{"no tag", "nginx", true},
		{"digest", "nginx@sha256:abc123def456", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deploy := makeDeployment("app", []interface{}{makeContainer("web", tt.image)})

			policy := NewImagePolicy(&ImagePolicyConfig{RequireDigests: true})
			result := &HardenResult{Resources: []*k8s.Resource{deploy}}

			err := policy.Apply(context.Background(), result.Resources, result)
			require.NoError(t, err)

			if tt.wantWarn {
				assert.True(t, len(result.Warnings) > 0, "expected warning for %s", tt.image)
			} else {
				assert.Empty(t, result.Warnings, "expected no warning for %s", tt.image)
			}
		})
	}
}

func TestImagePolicy_AllPoliciesCombined(t *testing.T) {
	deploy := makeDeployment("app", []interface{}{
		makeContainer("web", "docker.io/nginx:latest"),
	})

	policy := NewImagePolicy(&ImagePolicyConfig{
		DenyLatestTag:     true,
		AllowedRegistries: []string{"gcr.io"},
		RequireDigests:    true,
	})
	result := &HardenResult{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Should have 3 warnings: latest tag, non-allowed registry, no digest.
	assert.Len(t, result.Warnings, 3)
}

func TestImagePolicy_Name(t *testing.T) {
	policy := NewImagePolicy(&ImagePolicyConfig{})
	assert.Equal(t, "image-policy", policy.Name())
}

func TestImagePolicy_EmptyImage(t *testing.T) {
	deploy := makeDeployment("app", []interface{}{
		map[string]interface{}{"name": "web"},
	})

	policy := NewImagePolicy(&ImagePolicyConfig{DenyLatestTag: true})
	result := &HardenResult{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)
	assert.Empty(t, result.Warnings)
}

func TestHasLatestTag(t *testing.T) {
	assert.True(t, hasLatestTag("nginx"))
	assert.True(t, hasLatestTag("nginx:latest"))
	assert.True(t, hasLatestTag("registry.io/nginx:latest"))
	assert.False(t, hasLatestTag("nginx:1.25"))
	assert.False(t, hasLatestTag("nginx@sha256:abc"))
}

func TestHasDigest(t *testing.T) {
	assert.True(t, hasDigest("nginx@sha256:abc123"))
	assert.False(t, hasDigest("nginx:1.25"))
	assert.False(t, hasDigest("nginx"))
}

func TestIsAllowedRegistry(t *testing.T) {
	assert.True(t, isAllowedRegistry("gcr.io/project/img", []string{"gcr.io/project"}))
	assert.False(t, isAllowedRegistry("docker.io/img", []string{"gcr.io"}))
	// Trailing slash should not break matching.
	assert.True(t, isAllowedRegistry("gcr.io/project/img", []string{"gcr.io/project/"}))
	assert.True(t, isAllowedRegistry("gcr.io/myimage:v1", []string{"gcr.io/"}))
	// Case-insensitive hostname comparison.
	assert.True(t, isAllowedRegistry("Docker.io/library/nginx", []string{"docker.io"}))
	assert.True(t, isAllowedRegistry("GCR.IO/project/img", []string{"gcr.io/project"}))
	assert.True(t, isAllowedRegistry("docker.io/library/nginx", []string{"Docker.io"}))
}
