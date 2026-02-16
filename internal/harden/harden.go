// Package harden provides security hardening policies for Kubernetes resources.
//
// It implements Pod Security Standards (baseline/restricted), NetworkPolicy
// generation, image policy enforcement, resource requirements injection,
// RBAC generation, and SLSA provenance annotations.
package harden

import (
	"context"
	"fmt"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// SecurityLevel controls which Pod Security Standards are enforced.
type SecurityLevel string

const (
	// SecurityLevelNone disables PSS enforcement.
	SecurityLevelNone SecurityLevel = "none"

	// SecurityLevelBaseline enforces Kubernetes Baseline PSS.
	SecurityLevelBaseline SecurityLevel = "baseline"

	// SecurityLevelRestricted enforces Kubernetes Restricted PSS.
	SecurityLevelRestricted SecurityLevel = "restricted"
)

// ParseSecurityLevel parses a security level string.
func ParseSecurityLevel(s string) (SecurityLevel, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none":
		return SecurityLevelNone, nil
	case "baseline":
		return SecurityLevelBaseline, nil
	case "restricted":
		return SecurityLevelRestricted, nil
	default:
		return "", fmt.Errorf("invalid security level %q: must be none, baseline, or restricted", s)
	}
}

// HardenChange records a single modification made by the hardening engine.
type HardenChange struct {
	ResourceID string `json:"resourceId"`
	FieldPath  string `json:"fieldPath"`
	OldValue   string `json:"oldValue,omitempty"`
	NewValue   string `json:"newValue"`
	Reason     string `json:"reason"`
}

// HardenResult holds the full output of the hardening pipeline.
type HardenResult struct {
	// Resources is the modified resource list (includes any generated resources).
	Resources []*k8s.Resource

	// Changes is the list of modifications made.
	Changes []HardenChange

	// Warnings are non-fatal issues (e.g., conflicts with existing settings).
	Warnings []string
}

// ImagePolicyConfig configures image policy enforcement.
type ImagePolicyConfig struct {
	// DenyLatestTag warns on `:latest` image tags.
	DenyLatestTag bool

	// AllowedRegistries is a list of allowed image registries.
	AllowedRegistries []string

	// RequireDigests warns when images use tags instead of sha256 digests.
	RequireDigests bool
}

// ResourceDefaultsConfig configures default resource requirements injection.
type ResourceDefaultsConfig struct {
	// CPURequest is the default CPU request (e.g., "100m").
	CPURequest string

	// MemoryRequest is the default memory request (e.g., "128Mi").
	MemoryRequest string

	// CPULimit is the default CPU limit (e.g., "500m").
	CPULimit string

	// MemoryLimit is the default memory limit (e.g., "512Mi").
	MemoryLimit string

	// RequireLimits makes missing limits an error instead of using defaults.
	// When true, the policy returns an error if any container is missing
	// resource limits and no defaults are configured.
	RequireLimits bool
}

// Config configures the hardening pipeline.
type Config struct {
	// SecurityLevel controls PSS enforcement.
	SecurityLevel SecurityLevel

	// GenerateNetworkPolicies enables NetworkPolicy generation from the dependency graph.
	GenerateNetworkPolicies bool

	// GenerateRBAC enables ServiceAccount + Role + RoleBinding generation.
	GenerateRBAC bool

	// ResolveDigests replaces image tags with sha256 digests from the registry.
	ResolveDigests bool

	// RegistryClient is used when ResolveDigests is true. If nil, a default
	// HTTPRegistryClient is created.
	RegistryClient RegistryClient

	// ImagePolicy configures image policy enforcement.
	ImagePolicy *ImagePolicyConfig

	// ResourceDefaults configures default resource requirements.
	ResourceDefaults *ResourceDefaultsConfig

	// ResourceIDs maps resources to their assigned IDs (needed for netpol/RBAC).
	ResourceIDs map[*k8s.Resource]string
}

// Policy is a single hardening policy that can be applied to resources.
type Policy interface {
	// Name returns the policy name for logging/reporting.
	Name() string

	// Apply applies this policy to the given resources, returning changes and warnings.
	Apply(ctx context.Context, resources []*k8s.Resource, result *HardenResult) error
}

// Hardener orchestrates all hardening policies.
type Hardener struct {
	policies []Policy
}

// New creates a Hardener configured according to the given Config.
func New(cfg Config) *Hardener {
	var policies []Policy

	// 1. Pod Security Standards.
	if cfg.SecurityLevel != SecurityLevelNone && cfg.SecurityLevel != "" {
		policies = append(policies, NewPSSPolicy(cfg.SecurityLevel))
	}

	// 2. Resource requirements.
	if cfg.ResourceDefaults != nil {
		policies = append(policies, NewResourceRequirementsPolicy(cfg.ResourceDefaults))
	}

	// 3. Image policy.
	if cfg.ImagePolicy != nil {
		policies = append(policies, NewImagePolicy(cfg.ImagePolicy))
	}

	// 3b. Digest resolution.
	if cfg.ResolveDigests {
		client := cfg.RegistryClient
		if client == nil {
			client = NewHTTPRegistryClient(nil)
		}

		policies = append(policies, NewDigestResolverPolicy(client))
	}

	// 4. NetworkPolicy generation.
	if cfg.GenerateNetworkPolicies {
		policies = append(policies, NewNetworkPolicyGenerator(cfg.ResourceIDs))
	}

	// 5. RBAC generation.
	if cfg.GenerateRBAC {
		policies = append(policies, NewRBACGenerator(cfg.ResourceIDs))
	}

	return &Hardener{policies: policies}
}

// Harden applies all configured policies to the resources.
func (h *Hardener) Harden(ctx context.Context, resources []*k8s.Resource) (*HardenResult, error) {
	result := &HardenResult{
		Resources: resources,
	}

	for _, p := range h.policies {
		if err := p.Apply(ctx, result.Resources, result); err != nil {
			return nil, fmt.Errorf("policy %s: %w", p.Name(), err)
		}
	}

	return result, nil
}

// FileConfig represents the harden section of a .chart2kro.yaml config file.
type FileConfig struct {
	Enabled                 bool                `json:"enabled" yaml:"enabled"`
	SecurityLevel           string              `json:"security-level" yaml:"security-level"`
	GenerateNetworkPolicies bool                `json:"generate-network-policies" yaml:"generate-network-policies"`
	GenerateRBAC            bool                `json:"generate-rbac" yaml:"generate-rbac"`
	Images                  *FileImageConfig    `json:"images,omitempty" yaml:"images,omitempty"`
	Resources               *FileResourceConfig `json:"resources,omitempty" yaml:"resources,omitempty"`
}

// FileImageConfig is the images subsection of harden config.
type FileImageConfig struct {
	DenyLatestTag     bool     `json:"deny-latest-tag" yaml:"deny-latest-tag"`
	AllowedRegistries []string `json:"allowed-registries" yaml:"allowed-registries"`
	RequireDigests    bool     `json:"require-digests" yaml:"require-digests"`
}

// FileResourceConfig is the resources subsection of harden config.
type FileResourceConfig struct {
	CPURequest    string `json:"cpu-request" yaml:"cpu-request"`
	MemoryRequest string `json:"memory-request" yaml:"memory-request"`
	CPULimit      string `json:"cpu-limit" yaml:"cpu-limit"`
	MemoryLimit   string `json:"memory-limit" yaml:"memory-limit"`
	RequireLimits bool   `json:"require-limits" yaml:"require-limits"`
}

// ParseFileConfig extracts harden config from raw .chart2kro.yaml bytes.
// Returns nil if no harden section is present.
func ParseFileConfig(data []byte) (*FileConfig, error) {
	var raw struct {
		Harden *FileConfig `json:"harden" yaml:"harden"`
	}

	if err := sigsyaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing harden config: %w", err)
	}

	return raw.Harden, nil
}

// ToImagePolicyConfig converts file config to ImagePolicyConfig.
// Returns nil if no image policy is configured.
func (f *FileConfig) ToImagePolicyConfig() *ImagePolicyConfig {
	if f.Images == nil {
		return nil
	}

	return &ImagePolicyConfig{
		DenyLatestTag:     f.Images.DenyLatestTag,
		AllowedRegistries: f.Images.AllowedRegistries,
		RequireDigests:    f.Images.RequireDigests,
	}
}

// ToResourceDefaultsConfig converts file config to ResourceDefaultsConfig.
// Returns nil if no resource defaults are configured.
func (f *FileConfig) ToResourceDefaultsConfig() *ResourceDefaultsConfig {
	if f.Resources == nil {
		return nil
	}

	return &ResourceDefaultsConfig{
		CPURequest:    f.Resources.CPURequest,
		MemoryRequest: f.Resources.MemoryRequest,
		CPULimit:      f.Resources.CPULimit,
		MemoryLimit:   f.Resources.MemoryLimit,
		RequireLimits: f.Resources.RequireLimits,
	}
}
