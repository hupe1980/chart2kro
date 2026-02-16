package harden

import (
	"context"
	"fmt"
	"reflect"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// isWorkload returns true if the resource is a pod-bearing workload.
func isWorkload(res *k8s.Resource) bool {
	return k8s.IsWorkloadKind(res.Kind())
}

// dangerousCapabilities are capabilities that baseline PSS disallows.
var dangerousCapabilities = map[string]bool{
	"ALL":             true,
	"SYS_ADMIN":       true,
	"NET_ADMIN":       true,
	"SYS_PTRACE":      true,
	"SYS_RAWIO":       true,
	"SYS_MODULE":      true,
	"SYS_BOOT":        true,
	"DAC_READ_SEARCH": true,
	"NET_RAW":         true,
	"MKNOD":           true,
}

// PSSPolicy enforces Kubernetes Pod Security Standards on workload resources.
type PSSPolicy struct {
	level SecurityLevel
}

// NewPSSPolicy creates a PSS enforcement policy.
func NewPSSPolicy(level SecurityLevel) *PSSPolicy {
	return &PSSPolicy{level: level}
}

// Name returns the policy name.
func (p *PSSPolicy) Name() string {
	return "pod-security-standards"
}

// Apply enforces PSS at the configured level on all workload resources.
func (p *PSSPolicy) Apply(ctx context.Context, resources []*k8s.Resource, result *Result) error {
	for _, res := range resources {
		if !isWorkload(res) {
			continue
		}

		switch p.level {
		case SecurityLevelRestricted:
			p.applyRestricted(res, result)
		case SecurityLevelBaseline:
			p.applyBaseline(res, result)
		case SecurityLevelNone:
			// No hardening applied.
		}
	}

	return nil
}

// applyRestricted enforces Restricted PSS: injects full security context into
// all containers (including init containers).
func (p *PSSPolicy) applyRestricted(res *k8s.Resource, result *Result) {
	podSpec := getPodSpec(res)
	if podSpec == nil {
		return
	}

	resID := res.QualifiedName()

	// Pod-level security context.
	podSC := getOrCreateMap(podSpec, "securityContext")
	setIfMissing(podSC, "runAsNonRoot", true, resID, "spec.template.spec.securityContext.runAsNonRoot", "restricted PSS", result)
	setSeccompProfile(podSC, resID, "spec.template.spec.securityContext.seccompProfile", result)

	// Defense-in-depth: disable automatic service account token mounting.
	setIfMissing(podSpec, "automountServiceAccountToken", false, resID, "spec.template.spec.automountServiceAccountToken", "restricted PSS", result)

	// Container-level security contexts.
	hardenContainers(podSpec, "containers", resID, "spec.template.spec.containers", true, result)
	hardenContainers(podSpec, "initContainers", resID, "spec.template.spec.initContainers", true, result)
}

// applyBaseline enforces Baseline PSS: prevents privileged containers,
// host namespaces, and dangerous capabilities.
func (p *PSSPolicy) applyBaseline(res *k8s.Resource, result *Result) {
	podSpec := getPodSpec(res)
	if podSpec == nil {
		return
	}

	resID := res.QualifiedName()

	// Pod-level: ensure no host namespaces.
	enforceNotTrue(podSpec, "hostNetwork", resID, "spec.template.spec.hostNetwork", "baseline PSS", result)
	enforceNotTrue(podSpec, "hostPID", resID, "spec.template.spec.hostPID", "baseline PSS", result)
	enforceNotTrue(podSpec, "hostIPC", resID, "spec.template.spec.hostIPC", "baseline PSS", result)

	// Container-level: no privileged, drop dangerous caps.
	hardenContainers(podSpec, "containers", resID, "spec.template.spec.containers", false, result)
	hardenContainers(podSpec, "initContainers", resID, "spec.template.spec.initContainers", false, result)
}

// hardenContainers applies security context hardening to a list of containers.
// If restricted is true, applies full restricted PSS; otherwise baseline only.
func hardenContainers(podSpec map[string]interface{}, key, resID, basePath string, restricted bool, result *Result) {
	containers, ok := podSpec[key].([]interface{})
	if !ok {
		return
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
		sc := getOrCreateMap(container, "securityContext")

		if restricted {
			// Full restricted PSS.
			setIfMissing(sc, "runAsNonRoot", true, resID, containerPath+".securityContext.runAsNonRoot", "restricted PSS", result)
			setIfMissing(sc, "readOnlyRootFilesystem", true, resID, containerPath+".securityContext.readOnlyRootFilesystem", "restricted PSS", result)
			setIfMissing(sc, "allowPrivilegeEscalation", false, resID, containerPath+".securityContext.allowPrivilegeEscalation", "restricted PSS", result)
			setDropAllCapabilities(sc, resID, containerPath+".securityContext.capabilities", result)
			setSeccompProfile(sc, resID, containerPath+".securityContext.seccompProfile", result)
		} else {
			// Baseline PSS.
			enforceNotTrue(sc, "privileged", resID, containerPath+".securityContext.privileged", "baseline PSS", result)
			dropDangerousCapabilities(sc, resID, containerPath+".securityContext.capabilities", result)
		}
	}
}

// setIfMissing sets a field only if it's not already present. If the existing
// value conflicts, a warning is emitted instead of overriding.
func setIfMissing(m map[string]interface{}, key string, value interface{}, resID, fieldPath, reason string, result *Result) {
	existing, exists := m[key]
	if exists {
		if !reflect.DeepEqual(existing, value) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: %s is %v (expected %v for %s)", resID, fieldPath, existing, value, reason))
		}

		return
	}

	m[key] = value

	result.Changes = append(result.Changes, Change{
		ResourceID: resID,
		FieldPath:  fieldPath,
		NewValue:   fmt.Sprintf("%v", value),
		Reason:     reason,
	})
}

// enforceNotTrue emits a warning if a boolean field is explicitly true.
func enforceNotTrue(m map[string]interface{}, key, resID, fieldPath, reason string, result *Result) {
	val, exists := m[key]
	if !exists {
		return
	}

	if b, ok := val.(bool); ok && b {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%s: %s is true (violates %s)", resID, fieldPath, reason))
	}
}

// setDropAllCapabilities sets capabilities.drop = ["ALL"] if not already set.
func setDropAllCapabilities(sc map[string]interface{}, resID, fieldPath string, result *Result) {
	caps := getOrCreateMap(sc, "capabilities")

	if _, exists := caps["drop"]; exists {
		return
	}

	caps["drop"] = []interface{}{"ALL"}

	result.Changes = append(result.Changes, Change{
		ResourceID: resID,
		FieldPath:  fieldPath + ".drop",
		NewValue:   `["ALL"]`,
		Reason:     "restricted PSS",
	})
}

// dropDangerousCapabilities drops dangerous capabilities for baseline PSS.
func dropDangerousCapabilities(sc map[string]interface{}, resID, fieldPath string, result *Result) {
	caps, ok := sc["capabilities"].(map[string]interface{})
	if !ok {
		return
	}

	addList, ok := caps["add"].([]interface{})
	if !ok {
		return
	}

	var kept []interface{}

	for _, cap := range addList {
		capStr, ok := cap.(string)
		if !ok {
			kept = append(kept, cap)
			continue
		}

		if isDangerousCapability(capStr) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: %s.add contains dangerous capability %s (violates baseline PSS)", resID, fieldPath, capStr))
		} else {
			kept = append(kept, cap)
		}
	}

	if len(kept) != len(addList) {
		caps["add"] = kept
	}
}

// setSeccompProfile sets the seccomp profile to RuntimeDefault if not already set.
func setSeccompProfile(m map[string]interface{}, resID, fieldPath string, result *Result) {
	if _, exists := m["seccompProfile"]; exists {
		return
	}

	m["seccompProfile"] = map[string]interface{}{
		"type": "RuntimeDefault",
	}

	result.Changes = append(result.Changes, Change{
		ResourceID: resID,
		FieldPath:  fieldPath + ".type",
		NewValue:   "RuntimeDefault",
		Reason:     "restricted PSS",
	})
}

// getPodSpec navigates to the pod spec of a workload resource.
// For CronJob, it's spec.jobTemplate.spec.template.spec.
// For other workloads, it's spec.template.spec.
func getPodSpec(res *k8s.Resource) map[string]interface{} {
	if res.Object == nil {
		return nil
	}

	obj := res.Object.Object

	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	if res.Kind() == "CronJob" {
		jobTemplate, ok := spec["jobTemplate"].(map[string]interface{})
		if !ok {
			return nil
		}

		jobSpec, ok := jobTemplate["spec"].(map[string]interface{})
		if !ok {
			return nil
		}

		template, ok := jobSpec["template"].(map[string]interface{})
		if !ok {
			return nil
		}

		podSpec, ok := template["spec"].(map[string]interface{})
		if !ok {
			return nil
		}

		return podSpec
	}

	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		return nil
	}

	podSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	return podSpec
}

// getOrCreateMap returns the map at key, creating it if absent.
func getOrCreateMap(parent map[string]interface{}, key string) map[string]interface{} {
	if v, ok := parent[key].(map[string]interface{}); ok {
		return v
	}

	m := map[string]interface{}{}
	parent[key] = m

	return m
}

// isDangerousCapability checks if a capability is in the dangerous set.
func isDangerousCapability(cap string) bool {
	return dangerousCapabilities[cap]
}
