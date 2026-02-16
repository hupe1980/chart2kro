package harden

import (
	"context"
	"fmt"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// DefaultResourceDefaults provides sensible default resource requirements.
var DefaultResourceDefaults = &ResourceDefaultsConfig{
	CPURequest:    "100m",
	MemoryRequest: "128Mi",
	CPULimit:      "500m",
	MemoryLimit:   "512Mi",
}

// ResourceRequirementsPolicy injects default CPU/memory requests and limits
// into containers that are missing them.
type ResourceRequirementsPolicy struct {
	defaults *ResourceDefaultsConfig
}

// NewResourceRequirementsPolicy creates a resource requirements injection policy.
func NewResourceRequirementsPolicy(defaults *ResourceDefaultsConfig) *ResourceRequirementsPolicy {
	return &ResourceRequirementsPolicy{defaults: defaults}
}

// Name returns the policy name.
func (p *ResourceRequirementsPolicy) Name() string {
	return "resource-requirements"
}

// Apply injects default resource requirements into containers missing them.
func (p *ResourceRequirementsPolicy) Apply(ctx context.Context, resources []*k8s.Resource, result *Result) error {
	for _, res := range resources {
		if !isWorkload(res) {
			continue
		}

		podSpec := getPodSpec(res)
		if podSpec == nil {
			continue
		}

		resID := res.QualifiedName()

		if err := injectResourceDefaults(podSpec, "containers", resID, "spec.template.spec.containers", p.defaults, result); err != nil {
			return err
		}

		if err := injectResourceDefaults(podSpec, "initContainers", resID, "spec.template.spec.initContainers", p.defaults, result); err != nil {
			return err
		}
	}

	return nil
}

// injectResourceDefaults injects default resources into containers that are missing them.
func injectResourceDefaults(podSpec map[string]interface{}, key, resID, basePath string, defaults *ResourceDefaultsConfig, result *Result) error {
	containers, ok := podSpec[key].([]interface{})
	if !ok {
		return nil
	}

	for i, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := container["name"].(string)
		if name == "" {
			name = fmt.Sprintf("[%d]", i)
		}

		containerPath := fmt.Sprintf("%s[%s]", basePath, name)

		// Check requireLimits before injecting anything.
		if defaults.RequireLimits {
			if err := checkRequiredLimits(container, resID, containerPath, defaults); err != nil {
				return err
			}
		}

		resources := getOrCreateMap(container, "resources")

		// Inject requests.
		requests := getOrCreateMap(resources, "requests")
		setResourceIfMissing(requests, "cpu", defaults.CPURequest, resID, containerPath+".resources.requests.cpu", result)
		setResourceIfMissing(requests, "memory", defaults.MemoryRequest, resID, containerPath+".resources.requests.memory", result)

		// Inject limits.
		limits := getOrCreateMap(resources, "limits")
		setResourceIfMissing(limits, "cpu", defaults.CPULimit, resID, containerPath+".resources.limits.cpu", result)
		setResourceIfMissing(limits, "memory", defaults.MemoryLimit, resID, containerPath+".resources.limits.memory", result)
	}

	return nil
}

// checkRequiredLimits returns an error if a container is missing resource limits
// and no defaults are configured to fill them.
func checkRequiredLimits(container map[string]interface{}, resID, containerPath string, defaults *ResourceDefaultsConfig) error {
	resources, _ := container["resources"].(map[string]interface{})
	limits, _ := resources["limits"].(map[string]interface{})

	cpuLimit, _ := limits["cpu"].(string)
	memLimit, _ := limits["memory"].(string)

	if cpuLimit == "" && defaults.CPULimit == "" {
		return fmt.Errorf("%s: %s is missing cpu limit and no default is configured (requireLimits=true)", resID, containerPath)
	}

	if memLimit == "" && defaults.MemoryLimit == "" {
		return fmt.Errorf("%s: %s is missing memory limit and no default is configured (requireLimits=true)", resID, containerPath)
	}

	return nil
}

// setResourceIfMissing sets a resource requirement field if not already present.
func setResourceIfMissing(m map[string]interface{}, key, value, resID, fieldPath string, result *Result) {
	if value == "" {
		return
	}

	if _, exists := m[key]; exists {
		return
	}

	m[key] = value

	result.Changes = append(result.Changes, Change{
		ResourceID: resID,
		FieldPath:  fieldPath,
		NewValue:   value,
		Reason:     "resource-requirements",
	})
}
