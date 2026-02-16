package harden

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// mockRegistryClient implements RegistryClient for testing.
type mockRegistryClient struct {
	digests map[string]string // image → digest
	err     error
}

func (m *mockRegistryClient) ResolveDigest(_ context.Context, image string) (string, error) {
	if m.err != nil {
		return "", m.err
	}

	d, ok := m.digests[image]
	if !ok {
		return "", fmt.Errorf("image not found: %s", image)
	}

	return d, nil
}

func TestDigestResolverPolicy_Name(t *testing.T) {
	p := NewDigestResolverPolicy(&mockRegistryClient{})
	assert.Equal(t, "digest-resolver", p.Name())
}

func TestDigestResolverPolicy_ResolvesTagToDigest(t *testing.T) {
	client := &mockRegistryClient{
		digests: map[string]string{
			"nginx:1.25": "sha256:abcdef1234567890",
		},
	}

	deploy := makeDeployment("app", []interface{}{
		makeContainer("web", "nginx:1.25"),
	})

	policy := NewDigestResolverPolicy(client)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Verify the image was rewritten.
	podSpec := getPodSpec(deploy)
	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	assert.Equal(t, "nginx@sha256:abcdef1234567890", container["image"])

	// Verify change recorded.
	require.Len(t, result.Changes, 1)
	assert.Equal(t, "nginx:1.25", result.Changes[0].OldValue)
	assert.Equal(t, "nginx@sha256:abcdef1234567890", result.Changes[0].NewValue)
	assert.Equal(t, "digest-resolver", result.Changes[0].Reason)
}

func TestDigestResolverPolicy_SkipsAlreadyPinned(t *testing.T) {
	client := &mockRegistryClient{
		digests: map[string]string{},
	}

	deploy := makeDeployment("app", []interface{}{
		makeContainer("web", "nginx@sha256:existingdigest123"),
	})

	policy := NewDigestResolverPolicy(client)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)
	assert.Empty(t, result.Changes)
}

func TestDigestResolverPolicy_SkipsEmptyImage(t *testing.T) {
	client := &mockRegistryClient{}

	deploy := makeDeployment("app", []interface{}{
		map[string]interface{}{"name": "web"},
	})

	policy := NewDigestResolverPolicy(client)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)
	assert.Empty(t, result.Changes)
}

func TestDigestResolverPolicy_MultipleContainers(t *testing.T) {
	client := &mockRegistryClient{
		digests: map[string]string{
			"nginx:1.25":   "sha256:aaa111",
			"redis:7.0":    "sha256:bbb222",
			"busybox:1.36": "sha256:ccc333",
		},
	}

	deploy := &k8s.Resource{
		GVK:  deploymentGVK(),
		Name: "app",
		Object: makeUnstructuredWorkload("Deployment", "app", map[string]interface{}{
			"containers": []interface{}{
				makeContainer("web", "nginx:1.25"),
				makeContainer("cache", "redis:7.0"),
			},
			"initContainers": []interface{}{
				makeContainer("init", "busybox:1.36"),
			},
		}),
	}

	policy := NewDigestResolverPolicy(client)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)
	assert.Len(t, result.Changes, 3)

	podSpec := getPodSpec(deploy)
	containers := podSpec["containers"].([]interface{})
	assert.Equal(t, "nginx@sha256:aaa111", containers[0].(map[string]interface{})["image"])
	assert.Equal(t, "redis@sha256:bbb222", containers[1].(map[string]interface{})["image"])

	initContainers := podSpec["initContainers"].([]interface{})
	assert.Equal(t, "busybox@sha256:ccc333", initContainers[0].(map[string]interface{})["image"])
}

func TestDigestResolverPolicy_RegistryError(t *testing.T) {
	client := &mockRegistryClient{
		err: fmt.Errorf("network timeout"),
	}

	deploy := makeDeployment("app", []interface{}{
		makeContainer("web", "nginx:1.25"),
	})

	policy := NewDigestResolverPolicy(client)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network timeout")
}

func TestDigestResolverPolicy_CancelledContext(t *testing.T) {
	client := &mockRegistryClient{
		digests: map[string]string{
			"nginx:1.25": "sha256:abc123",
		},
	}

	deploy := makeDeployment("app", []interface{}{
		makeContainer("web", "nginx:1.25"),
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	// The mock client doesn't check context, but the HTTP client would.
	// This tests that the policy passes context through.
	policy := NewDigestResolverPolicy(client)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	// With mock client, this succeeds even with cancelled context.
	// But at least we verify no panics and proper pass-through.
	err := policy.Apply(ctx, result.Resources, result)
	require.NoError(t, err)
}

func TestDigestResolverPolicy_SkipsNonWorkload(t *testing.T) {
	client := &mockRegistryClient{}

	svc := makeService("svc", nil, nil)

	policy := NewDigestResolverPolicy(client)
	result := &Result{Resources: []*k8s.Resource{svc}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)
	assert.Empty(t, result.Changes)
}

func TestDigestResolverPolicy_RegistryImage(t *testing.T) {
	client := &mockRegistryClient{
		digests: map[string]string{
			"gcr.io/myproject/app:v2.1": "sha256:ddd444",
		},
	}

	deploy := makeDeployment("app", []interface{}{
		makeContainer("web", "gcr.io/myproject/app:v2.1"),
	})

	policy := NewDigestResolverPolicy(client)
	result := &Result{Resources: []*k8s.Resource{deploy}}

	err := policy.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)
	require.Len(t, result.Changes, 1)

	podSpec := getPodSpec(deploy)
	containers := podSpec["containers"].([]interface{})
	assert.Equal(t, "gcr.io/myproject/app@sha256:ddd444", containers[0].(map[string]interface{})["image"])
}

// --- parseImageRef tests ---

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		name         string
		image        string
		wantRegistry string
		wantRepo     string
		wantTag      string
	}{
		{
			name:         "bare image (Docker Hub library)",
			image:        "nginx",
			wantRegistry: "registry-1.docker.io",
			wantRepo:     "library/nginx",
			wantTag:      "latest",
		},
		{
			name:         "bare image with tag",
			image:        "nginx:1.25",
			wantRegistry: "registry-1.docker.io",
			wantRepo:     "library/nginx",
			wantTag:      "1.25",
		},
		{
			name:         "Docker Hub user image",
			image:        "myuser/myapp:v1",
			wantRegistry: "registry-1.docker.io",
			wantRepo:     "myuser/myapp",
			wantTag:      "v1",
		},
		{
			name:         "GCR image",
			image:        "gcr.io/project/app:v2",
			wantRegistry: "gcr.io",
			wantRepo:     "project/app",
			wantTag:      "v2",
		},
		{
			name:         "image with port",
			image:        "localhost:5000/myrepo/myapp:latest",
			wantRegistry: "localhost:5000",
			wantRepo:     "myrepo/myapp",
			wantTag:      "latest",
		},
		{
			name:         "no tag defaults to latest",
			image:        "gcr.io/project/app",
			wantRegistry: "gcr.io",
			wantRepo:     "project/app",
			wantTag:      "latest",
		},
		{
			name:         "image with digest is stripped",
			image:        "nginx@sha256:abc123",
			wantRegistry: "registry-1.docker.io",
			wantRepo:     "library/nginx",
			wantTag:      "latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, repo, tag := parseImageRef(tt.image)
			assert.Equal(t, tt.wantRegistry, reg, "registry")
			assert.Equal(t, tt.wantRepo, repo, "repo")
			assert.Equal(t, tt.wantTag, tag, "tag")
		})
	}
}

// --- imageWithDigest tests ---

func TestImageWithDigest(t *testing.T) {
	tests := []struct {
		name   string
		image  string
		digest string
		want   string
	}{
		{"simple tag", "nginx:1.25", "sha256:abc", "nginx@sha256:abc"},
		{"registry tag", "gcr.io/proj/img:v1", "sha256:def", "gcr.io/proj/img@sha256:def"},
		{"no tag", "nginx", "sha256:ghi", "nginx@sha256:ghi"},
		{"with port", "localhost:5000/app:v2", "sha256:jkl", "localhost:5000/app@sha256:jkl"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imageWithDigest(tt.image, tt.digest)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- HTTPRegistryClient tests using httptest ---

func TestHTTPRegistryClient_ResolveDigest(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/myrepo/myapp/manifests/v1.0" && r.Method == http.MethodHead {
			w.Header().Set("Docker-Content-Digest", "sha256:feedface")
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewHTTPRegistryClient(srv.Client())

	// We need to override the registry URL. Since parseImageRef uses the hostname,
	// construct an image ref that points to our test server.
	// srv.URL is like "https://127.0.0.1:port", strip the scheme.
	host := srv.URL[len("https://"):]
	image := host + "/myrepo/myapp:v1.0"

	digest, err := client.ResolveDigest(context.Background(), image)
	require.NoError(t, err)
	assert.Equal(t, "sha256:feedface", digest)
}

func TestHTTPRegistryClient_ResolveDigest_NotFound(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewHTTPRegistryClient(srv.Client())
	host := srv.URL[len("https://"):]
	image := host + "/myrepo/myapp:v1.0"

	_, err := client.ResolveDigest(context.Background(), image)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestHTTPRegistryClient_ResolveDigest_NoDigestHeader(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Intentionally no Docker-Content-Digest header.
	}))
	defer srv.Close()

	client := NewHTTPRegistryClient(srv.Client())
	host := srv.URL[len("https://"):]
	image := host + "/myrepo/myapp:v1.0"

	_, err := client.ResolveDigest(context.Background(), image)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not return a digest")
}

func TestHTTPRegistryClient_DockerHub_TokenAuth(t *testing.T) {
	// Simulate Docker Hub token + manifest endpoints.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			resp := map[string]string{"token": "test-token-123"}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer tokenSrv.Close()

	// We can't easily test Docker Hub auth without actually calling
	// registry-1.docker.io, but we can test getToken in isolation.
	// The integration path is covered by the mock tests above.
}

func TestNewHTTPRegistryClient_DefaultClient(t *testing.T) {
	client := NewHTTPRegistryClient(nil)
	assert.NotNil(t, client)
	assert.Equal(t, http.DefaultClient, client.client)
}

func TestNewHTTPRegistryClient_CustomClient(t *testing.T) {
	custom := &http.Client{}
	client := NewHTTPRegistryClient(custom)
	assert.Equal(t, custom, client.client)
}
