package k8s_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

func TestIsWorkload(t *testing.T) {
	tests := []struct {
		name string
		gvk  schema.GroupVersionKind
		want bool
	}{
		{"Deployment", schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, true},
		{"StatefulSet", schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, true},
		{"DaemonSet", schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}, true},
		{"ReplicaSet", schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, true},
		{"Job", schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}, true},
		{"CronJob", schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}, true},
		{"Service is not workload", schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}, false},
		{"ConfigMap is not workload", schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, k8s.IsWorkload(tt.gvk))
		})
	}
}

func TestIsService(t *testing.T) {
	assert.True(t, k8s.IsService(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}))
	assert.True(t, k8s.IsService(schema.GroupVersionKind{Group: "core", Version: "v1", Kind: "Service"}))
	assert.False(t, k8s.IsService(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Service"}))
	assert.False(t, k8s.IsService(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Deployment"}))
}

func TestIsConfig(t *testing.T) {
	assert.True(t, k8s.IsConfig(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}))
	assert.True(t, k8s.IsConfig(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}))
	assert.False(t, k8s.IsConfig(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}))
}

func TestIsStorage(t *testing.T) {
	assert.True(t, k8s.IsStorage(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}))
	assert.True(t, k8s.IsStorage(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolume"}))
	assert.False(t, k8s.IsStorage(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}))
}

func TestIsNetworking(t *testing.T) {
	assert.True(t, k8s.IsNetworking(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"}))
	assert.True(t, k8s.IsNetworking(schema.GroupVersionKind{Group: "extensions", Version: "v1beta1", Kind: "Ingress"}))
	assert.True(t, k8s.IsNetworking(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}))
	assert.False(t, k8s.IsNetworking(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}))
}

func TestIsCRD(t *testing.T) {
	assert.True(t, k8s.IsCRD(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}))
	assert.False(t, k8s.IsCRD(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}))
}

func TestIsRBAC(t *testing.T) {
	for _, kind := range []string{"Role", "ClusterRole", "RoleBinding", "ClusterRoleBinding"} {
		t.Run(kind, func(t *testing.T) {
			gvk := schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: kind}
			assert.True(t, k8s.IsRBAC(gvk))
		})
	}

	assert.False(t, k8s.IsRBAC(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Role"}))
}

func TestIsServiceAccount(t *testing.T) {
	assert.True(t, k8s.IsServiceAccount(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceAccount"}))
	assert.True(t, k8s.IsServiceAccount(schema.GroupVersionKind{Group: "core", Version: "v1", Kind: "ServiceAccount"}))
	assert.False(t, k8s.IsServiceAccount(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}))
}

func TestIsDeployment(t *testing.T) {
	assert.True(t, k8s.IsDeployment(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}))
	assert.False(t, k8s.IsDeployment(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Deployment"}))
	assert.False(t, k8s.IsDeployment(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}))
}

func TestIsStatefulSet(t *testing.T) {
	assert.True(t, k8s.IsStatefulSet(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}))
	assert.False(t, k8s.IsStatefulSet(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "StatefulSet"}))
	assert.False(t, k8s.IsStatefulSet(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}))
}

func TestIsDaemonSet(t *testing.T) {
	assert.True(t, k8s.IsDaemonSet(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}))
	assert.False(t, k8s.IsDaemonSet(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "DaemonSet"}))
	assert.False(t, k8s.IsDaemonSet(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}))
}

func TestIsJob(t *testing.T) {
	assert.True(t, k8s.IsJob(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}))
	assert.False(t, k8s.IsJob(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Job"}))
	assert.False(t, k8s.IsJob(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}))
}

func TestIsPVC(t *testing.T) {
	assert.True(t, k8s.IsPVC(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}))
	assert.True(t, k8s.IsPVC(schema.GroupVersionKind{Group: "core", Version: "v1", Kind: "PersistentVolumeClaim"}))
	assert.False(t, k8s.IsPVC(schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "PersistentVolumeClaim"}))
	assert.False(t, k8s.IsPVC(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolume"}))
}

func TestAPIVersion(t *testing.T) {
	tests := []struct {
		name     string
		gvk      schema.GroupVersionKind
		expected string
	}{
		{"core", schema.GroupVersionKind{Version: "v1", Kind: "Service"}, "v1"},
		{"apps", schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, "apps/v1"},
		{"networking", schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"}, "networking.k8s.io/v1"},
		{"batch", schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}, "batch/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, k8s.APIVersion(tt.gvk))
		})
	}
}
