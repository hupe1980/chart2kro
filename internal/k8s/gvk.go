package k8s

import "k8s.io/apimachinery/pkg/runtime/schema"

// GVK classification functions for branching by resource kind.

// IsWorkload returns true for workload resources.
func IsWorkload(gvk schema.GroupVersionKind) bool {
	switch gvk.Kind {
	case "Deployment", "StatefulSet", "DaemonSet":
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
