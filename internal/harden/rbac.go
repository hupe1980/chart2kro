package harden

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// rbacAPIGroup is the RBAC API group.
const rbacAPIGroup = "rbac.authorization.k8s.io"

// kindToAPIGroup maps resource kinds to their API group for RBAC rule generation.
var kindToAPIGroup = map[string]string{
	"Deployment":              "apps",
	"StatefulSet":             "apps",
	"DaemonSet":               "apps",
	"ReplicaSet":              "apps",
	"ConfigMap":               "",
	"Secret":                  "",
	"Service":                 "",
	"ServiceAccount":          "",
	"PersistentVolumeClaim":   "",
	"Job":                     "batch",
	"CronJob":                 "batch",
	"Ingress":                 "networking.k8s.io",
	"NetworkPolicy":           "networking.k8s.io",
	"HorizontalPodAutoscaler": "autoscaling",
}

// kindToResource maps resource kinds to their API resource names (pluralized, lowercase).
var kindToResource = map[string]string{
	"Deployment":              "deployments",
	"StatefulSet":             "statefulsets",
	"DaemonSet":               "daemonsets",
	"ReplicaSet":              "replicasets",
	"ConfigMap":               "configmaps",
	"Secret":                  "secrets",
	"Service":                 "services",
	"ServiceAccount":          "serviceaccounts",
	"PersistentVolumeClaim":   "persistentvolumeclaims",
	"Job":                     "jobs",
	"CronJob":                 "cronjobs",
	"Ingress":                 "ingresses",
	"NetworkPolicy":           "networkpolicies",
	"HorizontalPodAutoscaler": "horizontalpodautoscalers",
}

// RBACGenerator generates ServiceAccount, Role, and RoleBinding resources for workloads.
type RBACGenerator struct {
	resourceIDs map[*k8s.Resource]string
}

// NewRBACGenerator creates a generator that produces least-privilege RBAC resources.
func NewRBACGenerator(resourceIDs map[*k8s.Resource]string) *RBACGenerator {
	return &RBACGenerator{resourceIDs: resourceIDs}
}

// Name returns the policy name.
func (g *RBACGenerator) Name() string {
	return "rbac-generator"
}

// Apply generates RBAC resources for each workload.
func (g *RBACGenerator) Apply(ctx context.Context, resources []*k8s.Resource, result *Result) error {
	// Build a per-workload affinity map: for each workload, determine which
	// resource kinds it actually references (via env var secretRef/configMapRef,
	// volume mounts, etc.). This implements least-privilege RBAC.
	workloadKindAffinity := buildWorkloadAffinities(resources)

	// Generate RBAC for each workload.
	for _, res := range resources {
		if !isWorkload(res) {
			continue
		}

		saName := res.Name + "-sa"
		roleName := res.Name + "-role"
		bindingName := res.Name + "-rolebinding"

		// 1. ServiceAccount.
		sa := createServiceAccount(saName)
		result.Resources = append(result.Resources, sa)
		result.Changes = append(result.Changes, Change{
			ResourceID: sa.QualifiedName(),
			FieldPath:  "",
			NewValue:   "generated",
			Reason:     "rbac-generator",
		})

		// 2. Role with least-privilege permissions scoped to referenced kinds.
		kindSet := workloadKindAffinity[res.Name]
		if len(kindSet) == 0 {
			// Fallback: if no affinity detected, grant minimal self-read.
			kindSet = map[string]bool{res.Kind(): true}
		}

		role := createRole(roleName, kindSet)
		result.Resources = append(result.Resources, role)
		result.Changes = append(result.Changes, Change{
			ResourceID: role.QualifiedName(),
			FieldPath:  "",
			NewValue:   "generated",
			Reason:     "rbac-generator",
		})

		// 3. RoleBinding.
		binding := createRoleBinding(bindingName, saName, roleName)
		result.Resources = append(result.Resources, binding)
		result.Changes = append(result.Changes, Change{
			ResourceID: binding.QualifiedName(),
			FieldPath:  "",
			NewValue:   "generated",
			Reason:     "rbac-generator",
		})

		// 4. Set serviceAccountName on the workload.
		setServiceAccountName(res, saName, result)
	}

	return nil
}

// createServiceAccount creates a ServiceAccount resource.
func createServiceAccount(name string) *k8s.Resource {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}

	return &k8s.Resource{
		GVK: schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ServiceAccount",
		},
		Name:   name,
		Object: obj,
	}
}

// createRole creates a Role with least-privilege permissions inferred from the resource kinds.
func createRole(name string, kindSet map[string]bool) *k8s.Resource {
	// Group rules by API group.
	groupRules := make(map[string][]string)

	for kind := range kindSet {
		group := inferAPIGroup(kind)
		resource := inferResourceName(kind)
		groupRules[group] = append(groupRules[group], resource)
	}

	var rules []interface{}

	for group, resourceList := range groupRules {
		resources := make([]interface{}, len(resourceList))
		for i, r := range resourceList {
			resources[i] = r
		}

		rules = append(rules, map[string]interface{}{
			"apiGroups": []interface{}{group},
			"resources": resources,
			"verbs":     []interface{}{"get", "list", "watch"},
		})
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": rbacAPIGroup + "/v1",
			"kind":       "Role",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"rules": rules,
		},
	}

	return &k8s.Resource{
		GVK: schema.GroupVersionKind{
			Group:   rbacAPIGroup,
			Version: "v1",
			Kind:    "Role",
		},
		Name:   name,
		Object: obj,
	}
}

// createRoleBinding creates a RoleBinding linking a ServiceAccount to a Role.
func createRoleBinding(name, saName, roleName string) *k8s.Resource {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": rbacAPIGroup + "/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      saName,
					"namespace": "",
				},
			},
			"roleRef": map[string]interface{}{
				"apiGroup": rbacAPIGroup,
				"kind":     "Role",
				"name":     roleName,
			},
		},
	}

	return &k8s.Resource{
		GVK: schema.GroupVersionKind{
			Group:   rbacAPIGroup,
			Version: "v1",
			Kind:    "RoleBinding",
		},
		Name:   name,
		Object: obj,
	}
}

// setServiceAccountName sets spec.template.spec.serviceAccountName on a workload.
func setServiceAccountName(res *k8s.Resource, saName string, result *Result) {
	podSpec := getPodSpec(res)
	if podSpec == nil {
		return
	}

	if _, exists := podSpec["serviceAccountName"]; exists {
		return
	}

	podSpec["serviceAccountName"] = saName

	result.Changes = append(result.Changes, Change{
		ResourceID: res.QualifiedName(),
		FieldPath:  "spec.template.spec.serviceAccountName",
		NewValue:   saName,
		Reason:     "rbac-generator",
	})
}

// inferAPIGroup returns the API group for a given resource kind.
func inferAPIGroup(kind string) string {
	if group, ok := kindToAPIGroup[kind]; ok {
		return group
	}

	return ""
}

// inferResourceName returns the API resource name for a given kind.
func inferResourceName(kind string) string {
	if resource, ok := kindToResource[kind]; ok {
		return resource
	}

	return strings.ToLower(kind) + "s"
}

// buildWorkloadAffinities inspects each workload's pod spec to determine which
// resource kinds it actually references. This produces a workload-name→kind-set
// map for scoping RBAC rules per workload (least-privilege).
//
// Detected references:
//   - env[].valueFrom.secretKeyRef → Secret
//   - env[].valueFrom.configMapKeyRef → ConfigMap
//   - envFrom[].secretRef → Secret
//   - envFrom[].configMapRef → ConfigMap
//   - volumes[].secret → Secret
//   - volumes[].configMap → ConfigMap
//   - volumes[].persistentVolumeClaim → PersistentVolumeClaim
func buildWorkloadAffinities(resources []*k8s.Resource) map[string]map[string]bool {
	result := make(map[string]map[string]bool)

	for _, res := range resources {
		if !isWorkload(res) {
			continue
		}

		podSpec := getPodSpec(res)
		if podSpec == nil {
			continue
		}

		kinds := make(map[string]bool)

		// Scan containers and initContainers.
		for _, key := range []string{"containers", "initContainers"} {
			containers, _ := podSpec[key].([]interface{})
			for _, c := range containers {
				container, _ := c.(map[string]interface{})
				scanContainerRefs(container, kinds)
			}
		}

		// Scan volumes.
		volumes, _ := podSpec["volumes"].([]interface{})
		for _, v := range volumes {
			vol, _ := v.(map[string]interface{})
			scanVolumeRefs(vol, kinds)
		}

		result[res.Name] = kinds
	}

	return result
}

// scanContainerRefs inspects env, envFrom in a container for Secret/ConfigMap refs.
func scanContainerRefs(container map[string]interface{}, kinds map[string]bool) {
	if container == nil {
		return
	}

	// env[].valueFrom.secretKeyRef / configMapKeyRef
	envList, _ := container["env"].([]interface{})
	for _, e := range envList {
		envItem, _ := e.(map[string]interface{})
		valueFrom, _ := envItem["valueFrom"].(map[string]interface{})

		if valueFrom != nil {
			if _, ok := valueFrom["secretKeyRef"]; ok {
				kinds["Secret"] = true
			}

			if _, ok := valueFrom["configMapKeyRef"]; ok {
				kinds["ConfigMap"] = true
			}
		}
	}

	// envFrom[].secretRef / configMapRef
	envFrom, _ := container["envFrom"].([]interface{})
	for _, e := range envFrom {
		ef, _ := e.(map[string]interface{})
		if _, ok := ef["secretRef"]; ok {
			kinds["Secret"] = true
		}

		if _, ok := ef["configMapRef"]; ok {
			kinds["ConfigMap"] = true
		}
	}
}

// scanVolumeRefs inspects a volume spec for Secret/ConfigMap/PVC references.
func scanVolumeRefs(vol map[string]interface{}, kinds map[string]bool) {
	if vol == nil {
		return
	}

	if _, ok := vol["secret"]; ok {
		kinds["Secret"] = true
	}

	if _, ok := vol["configMap"]; ok {
		kinds["ConfigMap"] = true
	}

	if _, ok := vol["persistentVolumeClaim"]; ok {
		kinds["PersistentVolumeClaim"] = true
	}
}
