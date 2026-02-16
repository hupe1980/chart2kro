package k8s

import "k8s.io/apimachinery/pkg/runtime/schema"

// GVK classification functions for branching by resource kind.

// IsWorkload returns true for workload resources (pod-bearing kinds).
// This must stay consistent with WorkloadKinds in resource.go.
func IsWorkload(gvk schema.GroupVersionKind) bool {
	switch gvk.Kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet":
		return gvk.Group == "apps"
	case "Job", "CronJob":
		return gvk.Group == "batch"
	}

	return false
}

// IsService returns true for Service resources.
func IsService(gvk schema.GroupVersionKind) bool {
	return (gvk.Group == "" || gvk.Group == "core") && gvk.Kind == "Service"
}

// IsConfig returns true for configuration resources (ConfigMap, Secret).
func IsConfig(gvk schema.GroupVersionKind) bool {
	if gvk.Group != "" && gvk.Group != "core" {
		return false
	}

	return gvk.Kind == "ConfigMap" || gvk.Kind == "Secret"
}

// IsStorage returns true for storage resources.
func IsStorage(gvk schema.GroupVersionKind) bool {
	if gvk.Group != "" && gvk.Group != "core" {
		return false
	}

	return gvk.Kind == "PersistentVolumeClaim" || gvk.Kind == "PersistentVolume"
}

// IsNetworking returns true for networking resources.
func IsNetworking(gvk schema.GroupVersionKind) bool {
	if gvk.Kind == "Ingress" && (gvk.Group == "networking.k8s.io" || gvk.Group == "extensions") {
		return true
	}

	if gvk.Kind == "NetworkPolicy" && gvk.Group == "networking.k8s.io" {
		return true
	}

	return false
}

// IsCRD returns true for CustomResourceDefinition resources.
func IsCRD(gvk schema.GroupVersionKind) bool {
	return gvk.Kind == "CustomResourceDefinition" && gvk.Group == "apiextensions.k8s.io"
}

// IsRBAC returns true for RBAC resources.
func IsRBAC(gvk schema.GroupVersionKind) bool {
	if gvk.Group != "rbac.authorization.k8s.io" {
		return false
	}

	switch gvk.Kind {
	case "Role", "ClusterRole", "RoleBinding", "ClusterRoleBinding":
		return true
	}

	return false
}

// IsServiceAccount returns true for ServiceAccount resources.
func IsServiceAccount(gvk schema.GroupVersionKind) bool {
	return (gvk.Group == "" || gvk.Group == "core") && gvk.Kind == "ServiceAccount"
}

// --- Individual kind classifiers ---

// IsDeployment returns true for Deployment resources.
func IsDeployment(gvk schema.GroupVersionKind) bool {
	return gvk.Kind == "Deployment" && gvk.Group == "apps"
}

// IsStatefulSet returns true for StatefulSet resources.
func IsStatefulSet(gvk schema.GroupVersionKind) bool {
	return gvk.Kind == "StatefulSet" && gvk.Group == "apps"
}

// IsDaemonSet returns true for DaemonSet resources.
func IsDaemonSet(gvk schema.GroupVersionKind) bool {
	return gvk.Kind == "DaemonSet" && gvk.Group == "apps"
}

// IsJob returns true for Job resources.
func IsJob(gvk schema.GroupVersionKind) bool {
	return gvk.Kind == "Job" && gvk.Group == "batch"
}

// IsPVC returns true for PersistentVolumeClaim resources.
func IsPVC(gvk schema.GroupVersionKind) bool {
	return gvk.Kind == "PersistentVolumeClaim" && (gvk.Group == "" || gvk.Group == "core")
}

// APIVersion converts a GVK to an apiVersion string (e.g., "apps/v1" or "v1").
func APIVersion(gvk schema.GroupVersionKind) string {
	if gvk.Group == "" {
		return gvk.Version
	}

	return gvk.Group + "/" + gvk.Version
}
