package harden

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// RegistryClient resolves image tags to content digests.
type RegistryClient interface {
	// ResolveDigest resolves an image reference (e.g. "nginx:1.25") to its
	// manifest digest (e.g. "sha256:abc123..."). Returns an error if the
	// registry is unreachable or the image does not exist.
	ResolveDigest(ctx context.Context, image string) (string, error)
}

// HTTPRegistryClient resolves digests by querying OCI-compliant container
// registries over HTTP(S). It performs a HEAD request against the registry's
// manifest endpoint and reads the Docker-Content-Digest header.
type HTTPRegistryClient struct {
	client *http.Client
}

// NewHTTPRegistryClient creates a registry client backed by the given HTTP client.
// If client is nil, http.DefaultClient is used.
func NewHTTPRegistryClient(client *http.Client) *HTTPRegistryClient {
	if client == nil {
		client = http.DefaultClient
	}

	return &HTTPRegistryClient{client: client}
}

// ResolveDigest resolves an image reference to its sha256 digest by querying
// the registry. It supports Docker Hub, GCR, and any OCI-compliant registry.
func (c *HTTPRegistryClient) ResolveDigest(ctx context.Context, image string) (string, error) {
	registry, repo, tag := parseImageRef(image)

	// Get an auth token for Docker Hub.
	token, err := c.getToken(ctx, registry, repo)
	if err != nil {
		return "", fmt.Errorf("authenticating with registry: %w", err)
	}

	// HEAD the manifest to get the digest.
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.list.v2+json")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("querying registry %s: %w", registry, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry returned %d for %s/%s:%s", resp.StatusCode, registry, repo, tag)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("registry did not return a digest for %s/%s:%s", registry, repo, tag)
	}

	return digest, nil
}

// getToken obtains a bearer token for Docker Hub registries. For non-Hub
// registries, returns an empty token (anonymous access).
func (c *HTTPRegistryClient) getToken(ctx context.Context, registry, repo string) (string, error) {
	if registry != "registry-1.docker.io" {
		return "", nil
	}

	url := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit for auth responses
	if err != nil {
		return "", err
	}

	var result struct {
		Token string `json:"token"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	return result.Token, nil
}

// parseImageRef splits an image reference into registry, repository, and tag.
// Docker Hub images without a registry prefix are normalized.
func parseImageRef(image string) (registry, repo, tag string) {
	// Strip digest if present (should not happen when resolving, but be safe).
	if idx := strings.Index(image, "@"); idx >= 0 {
		image = image[:idx]
	}

	// Split tag.
	tag = "latest"

	if colonIdx := strings.LastIndex(image, ":"); colonIdx >= 0 {
		// Only treat as tag if the colon is after the last slash (not a port).
		afterColon := image[colonIdx+1:]
		if !strings.Contains(afterColon, "/") {
			tag = afterColon
			image = image[:colonIdx]
		}
	}

	// Split registry from repository.
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		// Bare image name → Docker Hub official library.
		registry = "registry-1.docker.io"
		repo = "library/" + parts[0]
	} else if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
		// First part looks like a hostname → use as registry.
		registry = parts[0]
		repo = parts[1]
	} else {
		// Docker Hub user/image.
		registry = "registry-1.docker.io"
		repo = image
	}

	return registry, repo, tag
}

// DigestResolverPolicy replaces image tags with sha256 digests in all containers.
type DigestResolverPolicy struct {
	client RegistryClient
}

// NewDigestResolverPolicy creates a policy that resolves image tags to digests.
func NewDigestResolverPolicy(client RegistryClient) *DigestResolverPolicy {
	return &DigestResolverPolicy{client: client}
}

// Name returns the policy name.
func (p *DigestResolverPolicy) Name() string {
	return "digest-resolver"
}

// Apply resolves all image tags to sha256 digests.
func (p *DigestResolverPolicy) Apply(ctx context.Context, resources []*k8s.Resource, result *HardenResult) error {
	for _, res := range resources {
		if !isWorkload(res) {
			continue
		}

		podSpec := getPodSpec(res)
		if podSpec == nil {
			continue
		}

		resID := res.QualifiedName()

		if err := p.resolveContainerImages(ctx, podSpec, "containers", resID, "spec.template.spec.containers", result); err != nil {
			return err
		}

		if err := p.resolveContainerImages(ctx, podSpec, "initContainers", resID, "spec.template.spec.initContainers", result); err != nil {
			return err
		}
	}

	return nil
}

// resolveContainerImages resolves images for a list of containers.
func (p *DigestResolverPolicy) resolveContainerImages(
	ctx context.Context,
	podSpec map[string]interface{},
	key, resID, basePath string,
	result *HardenResult,
) error {
	containers, ok := podSpec[key].([]interface{})
	if !ok {
		return nil
	}

	for i, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		image, _ := container["image"].(string)
		if image == "" || hasDigest(image) {
			continue // Already pinned or empty.
		}

		name, _ := container["name"].(string)
		if name == "" {
			name = fmt.Sprintf("[%d]", i)
		}

		digest, err := p.client.ResolveDigest(ctx, image)
		if err != nil {
			return fmt.Errorf("resolving digest for %s (container %s in %s): %w", image, name, resID, err)
		}

		// Build the new reference: strip the tag and append the digest.
		newImage := imageWithDigest(image, digest)
		container["image"] = newImage

		result.Changes = append(result.Changes, HardenChange{
			ResourceID: resID,
			FieldPath:  fmt.Sprintf("%s[%s].image", basePath, name),
			OldValue:   image,
			NewValue:   newImage,
			Reason:     "digest-resolver",
		})
	}

	return nil
}

// imageWithDigest replaces the tag in an image reference with a digest.
// "nginx:1.25" → "nginx@sha256:abc..."
// "gcr.io/proj/img:v1" → "gcr.io/proj/img@sha256:abc..."
func imageWithDigest(image, digest string) string {
	// Remove existing tag.
	ref := image
	if idx := strings.LastIndex(ref, ":"); idx >= 0 {
		// Only strip if the colon is a tag separator (after last slash).
		after := ref[idx+1:]
		if !strings.Contains(after, "/") {
			ref = ref[:idx]
		}
	}

	return ref + "@" + digest
}
