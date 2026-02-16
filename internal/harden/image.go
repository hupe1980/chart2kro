package harden

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// ImagePolicy enforces image security policies on all containers.
type ImagePolicy struct {
	cfg *ImagePolicyConfig
}

// NewImagePolicy creates an image policy enforcement policy.
func NewImagePolicy(cfg *ImagePolicyConfig) *ImagePolicy {
	return &ImagePolicy{cfg: cfg}
}

// Name returns the policy name.
func (p *ImagePolicy) Name() string {
	return "image-policy"
}

// Apply checks all container images against the configured policies.
func (p *ImagePolicy) Apply(ctx context.Context, resources []*k8s.Resource, result *HardenResult) error {
	for _, res := range resources {
		if !isWorkload(res) {
			continue
		}

		podSpec := getPodSpec(res)
		if podSpec == nil {
			continue
		}

		resID := res.QualifiedName()
		checkContainerImages(podSpec, "containers", resID, p.cfg, result)
		checkContainerImages(podSpec, "initContainers", resID, p.cfg, result)
	}

	return nil
}

// checkContainerImages checks image policies on a container list.
func checkContainerImages(podSpec map[string]interface{}, key, resID string, cfg *ImagePolicyConfig, result *HardenResult) {
	containers, ok := podSpec[key].([]interface{})
	if !ok {
		return
	}

	for i, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		image, _ := container["image"].(string)
		if image == "" {
			continue
		}

		name, _ := container["name"].(string)
		if name == "" {
			name = fmt.Sprintf("[%d]", i)
		}

		// Check for latest tag.
		if cfg.DenyLatestTag && hasLatestTag(image) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: container %s uses :latest tag (%s)", resID, name, image))
		}

		// Check allowed registries.
		if len(cfg.AllowedRegistries) > 0 && !isAllowedRegistry(image, cfg.AllowedRegistries) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: container %s uses image from non-allowed registry (%s)", resID, name, image))
		}

		// Check digest requirement.
		if cfg.RequireDigests && !hasDigest(image) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: container %s uses tag instead of digest (%s)", resID, name, image))
		}
	}
}

// hasLatestTag returns true if the image uses :latest or has no explicit tag.
// Delegates to the shared k8s.HasLatestTag for consistent behavior across
// audit and harden packages.
func hasLatestTag(image string) bool {
	return k8s.HasLatestTag(image)
}

// isAllowedRegistry checks if the image comes from one of the allowed registries.
func isAllowedRegistry(image string, allowed []string) bool {
	lowerImage := strings.ToLower(image)

	for _, reg := range allowed {
		// Normalize trailing slashes and case for hostname comparison.
		reg = strings.ToLower(strings.TrimRight(reg, "/"))
		if strings.HasPrefix(lowerImage, reg+"/") || strings.HasPrefix(lowerImage, reg+":") || lowerImage == reg {
			return true
		}
	}

	return false
}

// hasDigest returns true if the image reference uses a sha256 digest.
// Delegates to the shared k8s.IsImageDigest for consistent behavior.
func hasDigest(image string) bool {
	return k8s.IsImageDigest(image)
}
