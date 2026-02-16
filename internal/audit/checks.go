package audit

import (
	"context"
	"fmt"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// isWorkload returns true if the resource is a pod-bearing workload.
func isWorkload(res *k8s.Resource) bool {
	return k8s.IsWorkloadKind(res.Kind())
}

// getPodSpec navigates to the pod template spec of a workload.
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
		jt, ok := spec["jobTemplate"].(map[string]interface{})
		if !ok {
			return nil
		}

		js, ok := jt["spec"].(map[string]interface{})
		if !ok {
			return nil
		}

		tpl, ok := js["template"].(map[string]interface{})
		if !ok {
			return nil
		}

		ps, ok := tpl["spec"].(map[string]interface{})
		if !ok {
			return nil
		}

		return ps
	}

	tpl, ok := spec["template"].(map[string]interface{})
	if !ok {
		return nil
	}

	ps, ok := tpl["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	return ps
}

// getContainers returns the containers list from podSpec under the given key.
func getContainers(podSpec map[string]interface{}, key string) []map[string]interface{} {
	containers, ok := podSpec[key].([]interface{})
	if !ok {
		return nil
	}

	var result []map[string]interface{}

	for _, c := range containers {
		if cm, ok := c.(map[string]interface{}); ok {
			result = append(result, cm)
		}
	}

	return result
}

// allContainers returns all containers + initContainers from a podSpec.
func allContainers(podSpec map[string]interface{}) []map[string]interface{} {
	result := getContainers(podSpec, "containers")
	result = append(result, getContainers(podSpec, "initContainers")...)

	return result
}

// containerName returns the name of a container, or a fallback index.
func containerName(c map[string]interface{}, index int) string {
	if name, ok := c["name"].(string); ok && name != "" {
		return name
	}

	return fmt.Sprintf("[%d]", index)
}

// containerVisitor is called for each container with its encompassing resource,
// podSpec, container map, and positional index.
type containerVisitor func(res *k8s.Resource, podSpec, container map[string]interface{}, index int)

// forEachContainer iterates over all containers (including initContainers) in
// workload resources, invoking fn for each one. Non-workload resources and
// workloads with nil Objects are silently skipped.
func forEachContainer(resources []*k8s.Resource, fn containerVisitor) {
	for _, res := range resources {
		if !isWorkload(res) {
			continue
		}

		podSpec := getPodSpec(res)
		if podSpec == nil {
			continue
		}

		for i, container := range allContainers(podSpec) {
			fn(res, podSpec, container, i)
		}
	}
}

// podSpecVisitor is called for each workload with its resource and podSpec.
type podSpecVisitor func(res *k8s.Resource, podSpec map[string]interface{})

// forEachWorkload iterates over workload resources, invoking fn with the
// resource and its podSpec. Non-workloads and nil podSpecs are skipped.
func forEachWorkload(resources []*k8s.Resource, fn podSpecVisitor) {
	for _, res := range resources {
		if !isWorkload(res) {
			continue
		}

		podSpec := getPodSpec(res)
		if podSpec == nil {
			continue
		}

		fn(res, podSpec)
	}
}

// --- SEC-001: Container runs as root ---

// RunAsRootCheck flags containers without runAsNonRoot: true.
type RunAsRootCheck struct{}

// ID returns the check identifier.
func (c *RunAsRootCheck) ID() string { return "SEC-001" }

// Run executes the check against the given resources.
func (c *RunAsRootCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	forEachContainer(resources, func(res *k8s.Resource, podSpec, container map[string]interface{}, i int) {
		// Check pod-level securityContext.
		podSC, _ := podSpec["securityContext"].(map[string]interface{})
		podRunAsNonRoot, _ := podSC["runAsNonRoot"].(bool)

		sc, _ := container["securityContext"].(map[string]interface{})
		containerRunAsNonRoot, hasField := sc["runAsNonRoot"].(bool)

		if !podRunAsNonRoot && (!hasField || !containerRunAsNonRoot) {
			findings = append(findings, Finding{
				RuleID:       c.ID(),
				Severity:     SeverityCritical,
				ResourceID:   res.QualifiedName(),
				ResourceKind: res.Kind(),
				Message:      fmt.Sprintf("container %s does not set runAsNonRoot: true", containerName(container, i)),
				Remediation:  "Set spec.template.spec.containers[*].securityContext.runAsNonRoot to true",
			})
		}
	})

	return findings
}

// --- SEC-002: Privileged container ---

// PrivilegedCheck flags containers with privileged: true.
type PrivilegedCheck struct{}

// ID returns the check identifier.
func (c *PrivilegedCheck) ID() string { return "SEC-002" }

// Run executes the check against the given resources.
func (c *PrivilegedCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	forEachContainer(resources, func(res *k8s.Resource, _, container map[string]interface{}, i int) {
		sc, _ := container["securityContext"].(map[string]interface{})
		if priv, ok := sc["privileged"].(bool); ok && priv {
			findings = append(findings, Finding{
				RuleID:       c.ID(),
				Severity:     SeverityCritical,
				ResourceID:   res.QualifiedName(),
				ResourceKind: res.Kind(),
				Message:      fmt.Sprintf("container %s is privileged", containerName(container, i)),
				Remediation:  "Set securityContext.privileged to false or remove it",
			})
		}
	})

	return findings
}

// --- SEC-003: No resource limits ---

// ResourceLimitsCheck flags containers without resource limits.
type ResourceLimitsCheck struct{}

// ID returns the check identifier.
func (c *ResourceLimitsCheck) ID() string { return "SEC-003" }

// Run executes the check against the given resources.
func (c *ResourceLimitsCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	forEachContainer(resources, func(res *k8s.Resource, _, container map[string]interface{}, i int) {
		resources, _ := container["resources"].(map[string]interface{})
		limits, _ := resources["limits"].(map[string]interface{})

		if len(limits) == 0 {
			findings = append(findings, Finding{
				RuleID:       c.ID(),
				Severity:     SeverityHigh,
				ResourceID:   res.QualifiedName(),
				ResourceKind: res.Kind(),
				Message:      fmt.Sprintf("container %s has no resource limits defined", containerName(container, i)),
				Remediation:  "Add spec.template.spec.containers[*].resources.limits with cpu and memory",
			})
		}
	})

	return findings
}

// --- SEC-004: Image uses :latest tag ---

// LatestTagCheck flags images that use :latest or have no explicit tag.
type LatestTagCheck struct{}

// ID returns the check identifier.
func (c *LatestTagCheck) ID() string { return "SEC-004" }

// Run executes the check against the given resources.
func (c *LatestTagCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	forEachContainer(resources, func(res *k8s.Resource, _, container map[string]interface{}, i int) {
		image, _ := container["image"].(string)
		if image == "" {
			return
		}

		if hasLatestTag(image) {
			findings = append(findings, Finding{
				RuleID:       c.ID(),
				Severity:     SeverityHigh,
				ResourceID:   res.QualifiedName(),
				ResourceKind: res.Kind(),
				Message:      fmt.Sprintf("container %s uses :latest tag (%s)", containerName(container, i), image),
				Remediation:  "Pin image to a specific version tag or sha256 digest",
			})
		}
	})

	return findings
}

// --- SEC-005: Host networking/PID/IPC ---

// HostNamespaceCheck flags pods using host namespaces.
type HostNamespaceCheck struct{}

// ID returns the check identifier.
func (c *HostNamespaceCheck) ID() string { return "SEC-005" }

// Run executes the check against the given resources.
func (c *HostNamespaceCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	hostFields := []struct {
		key  string
		desc string
	}{
		{"hostNetwork", "host networking"},
		{"hostPID", "host PID namespace"},
		{"hostIPC", "host IPC namespace"},
	}

	forEachWorkload(resources, func(res *k8s.Resource, podSpec map[string]interface{}) {
		for _, hf := range hostFields {
			if val, ok := podSpec[hf.key].(bool); ok && val {
				findings = append(findings, Finding{
					RuleID:       c.ID(),
					Severity:     SeverityHigh,
					ResourceID:   res.QualifiedName(),
					ResourceKind: res.Kind(),
					Message:      fmt.Sprintf("%s is enabled", hf.desc),
					Remediation:  fmt.Sprintf("Set spec.template.spec.%s to false", hf.key),
				})
			}
		}
	})

	return findings
}

// --- SEC-006: readOnlyRootFilesystem not set ---

// ReadOnlyRootFSCheck flags containers without readOnlyRootFilesystem.
type ReadOnlyRootFSCheck struct{}

// ID returns the check identifier.
func (c *ReadOnlyRootFSCheck) ID() string { return "SEC-006" }

// Run executes the check against the given resources.
func (c *ReadOnlyRootFSCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	forEachContainer(resources, func(res *k8s.Resource, _, container map[string]interface{}, i int) {
		sc, _ := container["securityContext"].(map[string]interface{})
		if roRFS, ok := sc["readOnlyRootFilesystem"].(bool); !ok || !roRFS {
			findings = append(findings, Finding{
				RuleID:       c.ID(),
				Severity:     SeverityMedium,
				ResourceID:   res.QualifiedName(),
				ResourceKind: res.Kind(),
				Message:      fmt.Sprintf("container %s does not set readOnlyRootFilesystem: true", containerName(container, i)),
				Remediation:  "Set securityContext.readOnlyRootFilesystem to true",
			})
		}
	})

	return findings
}

// --- SEC-007: No NetworkPolicies ---

// NetworkPolicyCheck flags when the resource set contains no NetworkPolicies.
type NetworkPolicyCheck struct{}

// ID returns the check identifier.
func (c *NetworkPolicyCheck) ID() string { return "SEC-007" }

// Run executes the check against the given resources.
func (c *NetworkPolicyCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	hasWorkload := false
	hasNetPol := false

	for _, res := range resources {
		if isWorkload(res) {
			hasWorkload = true
		}

		if res.Kind() == "NetworkPolicy" {
			hasNetPol = true
		}
	}

	if hasWorkload && !hasNetPol {
		return []Finding{{
			RuleID:       c.ID(),
			Severity:     SeverityMedium,
			ResourceID:   "(global)",
			ResourceKind: "",
			Message:      "no NetworkPolicies found for workloads",
			Remediation:  "Add NetworkPolicies or use --generate-network-policies with --harden",
		}}
	}

	return nil
}

// --- SEC-008: Dangerous capabilities ---

// DangerousCapabilitiesCheck flags containers with dangerous capabilities.
type DangerousCapabilitiesCheck struct{}

// ID returns the check identifier.
func (c *DangerousCapabilitiesCheck) ID() string { return "SEC-008" }

// dangerousCaps is the set of capabilities considered dangerous.
var dangerousCaps = map[string]bool{
	"SYS_ADMIN":       true,
	"NET_ADMIN":       true,
	"SYS_PTRACE":      true,
	"SYS_RAWIO":       true,
	"SYS_MODULE":      true,
	"SYS_BOOT":        true,
	"DAC_READ_SEARCH": true,
	"NET_RAW":         true,
	"MKNOD":           true,
	"ALL":             true,
}

// Run executes the check against the given resources.
func (c *DangerousCapabilitiesCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	forEachContainer(resources, func(res *k8s.Resource, _, container map[string]interface{}, i int) {
		sc, _ := container["securityContext"].(map[string]interface{})
		caps, _ := sc["capabilities"].(map[string]interface{})
		addList, _ := caps["add"].([]interface{})

		for _, cap := range addList {
			capStr, ok := cap.(string)
			if !ok {
				continue
			}

			if dangerousCaps[capStr] {
				findings = append(findings, Finding{
					RuleID:       c.ID(),
					Severity:     SeverityMedium,
					ResourceID:   res.QualifiedName(),
					ResourceKind: res.Kind(),
					Message:      fmt.Sprintf("container %s adds dangerous capability %s", containerName(container, i), capStr),
					Remediation:  fmt.Sprintf("Remove %s from securityContext.capabilities.add", capStr),
				})
			}
		}
	})

	return findings
}

// --- SEC-009: Overly broad Service selector ---

// BroadSelectorCheck flags Services with fewer than 2 selector labels.
type BroadSelectorCheck struct{}

// ID returns the check identifier.
func (c *BroadSelectorCheck) ID() string { return "SEC-009" }

// Run executes the check against the given resources.
func (c *BroadSelectorCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	for _, res := range resources {
		if res.Kind() != "Service" || res.Object == nil {
			continue
		}

		spec, _ := res.Object.Object["spec"].(map[string]interface{})

		// ExternalName services have no selector by design — skip them.
		if svcType, _ := spec["type"].(string); svcType == "ExternalName" {
			continue
		}

		selector, _ := spec["selector"].(map[string]interface{})

		if len(selector) < 2 {
			findings = append(findings, Finding{
				RuleID:       c.ID(),
				Severity:     SeverityLow,
				ResourceID:   res.QualifiedName(),
				ResourceKind: res.Kind(),
				Message:      fmt.Sprintf("Service selector has %d label(s), consider using at least 2 for specificity", len(selector)),
				Remediation:  "Add more specific labels to spec.selector (e.g., app and component)",
			})
		}
	}

	return findings
}

// --- SEC-010: Missing probes ---

// ProbeCheck flags containers without liveness or readiness probes.
type ProbeCheck struct{}

// ID returns the check identifier.
func (c *ProbeCheck) ID() string { return "SEC-010" }

// Run executes the check against the given resources.
func (c *ProbeCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	// Only check main containers, not init containers.
	forEachWorkload(resources, func(res *k8s.Resource, podSpec map[string]interface{}) {
		for i, container := range getContainers(podSpec, "containers") {
			_, hasLiveness := container["livenessProbe"]
			_, hasReadiness := container["readinessProbe"]

			if !hasLiveness {
				findings = append(findings, Finding{
					RuleID:       c.ID(),
					Severity:     SeverityLow,
					ResourceID:   res.QualifiedName(),
					ResourceKind: res.Kind(),
					Message:      fmt.Sprintf("container %s has no liveness probe", containerName(container, i)),
					Remediation:  "Add spec.template.spec.containers[*].livenessProbe",
				})
			}

			if !hasReadiness {
				findings = append(findings, Finding{
					RuleID:       c.ID(),
					Severity:     SeverityLow,
					ResourceID:   res.QualifiedName(),
					ResourceKind: res.Kind(),
					Message:      fmt.Sprintf("container %s has no readiness probe", containerName(container, i)),
					Remediation:  "Add spec.template.spec.containers[*].readinessProbe",
				})
			}
		}
	})

	return findings
}

// --- SEC-011: Ingress without TLS ---

// IngressTLSCheck flags Ingress resources without TLS configuration.
type IngressTLSCheck struct{}

// ID returns the check identifier.
func (c *IngressTLSCheck) ID() string { return "SEC-011" }

// Run executes the check against the given resources.
func (c *IngressTLSCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	for _, res := range resources {
		if res.Kind() != "Ingress" || res.Object == nil {
			continue
		}

		spec, _ := res.Object.Object["spec"].(map[string]interface{})
		tls, hasTLS := spec["tls"]

		if !hasTLS || tls == nil {
			findings = append(findings, Finding{
				RuleID:       c.ID(),
				Severity:     SeverityInfo,
				ResourceID:   res.QualifiedName(),
				ResourceKind: res.Kind(),
				Message:      "Ingress has no TLS configuration",
				Remediation:  "Add spec.tls with a secret and hosts list",
			})
		}
	}

	return findings
}

// --- SEC-012: No seccompProfile ---

// SeccompProfileCheck flags containers without a seccomp profile.
type SeccompProfileCheck struct{}

// ID returns the check identifier.
func (c *SeccompProfileCheck) ID() string { return "SEC-012" }

// Run executes the check against the given resources.
func (c *SeccompProfileCheck) Run(_ context.Context, resources []*k8s.Resource) []Finding {
	var findings []Finding

	forEachContainer(resources, func(res *k8s.Resource, podSpec, container map[string]interface{}, i int) {
		// Check pod-level.
		podSC, _ := podSpec["securityContext"].(map[string]interface{})
		_, podHasSeccomp := podSC["seccompProfile"]

		sc, _ := container["securityContext"].(map[string]interface{})
		_, containerHasSeccomp := sc["seccompProfile"]

		if !podHasSeccomp && !containerHasSeccomp {
			findings = append(findings, Finding{
				RuleID:       c.ID(),
				Severity:     SeverityInfo,
				ResourceID:   res.QualifiedName(),
				ResourceKind: res.Kind(),
				Message:      fmt.Sprintf("container %s has no seccomp profile set", containerName(container, i)),
				Remediation:  "Set securityContext.seccompProfile.type to RuntimeDefault",
			})
		}
	})

	return findings
}

// --- Image tag helper (shared with harden) ---

// hasLatestTag returns true if the image uses :latest or has no explicit tag.
// Delegates to the shared k8s.HasLatestTag for consistent behavior across
// audit and harden packages.
func hasLatestTag(image string) bool {
	return k8s.HasLatestTag(image)
}
