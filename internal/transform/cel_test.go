package transform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/transform"
)

func TestSchemaRef(t *testing.T) {
	tests := []struct {
		name     string
		path     []string
		expected string
	}{
		{"single", []string{"replicas"}, "${schema.replicas}"},
		{"nested", []string{"spec", "replicas"}, "${schema.spec.replicas}"},
		{"deep", []string{"spec", "container", "image"}, "${schema.spec.container.image}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, transform.SchemaRef(tt.path...))
		})
	}
}

func TestResourceRef(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
		path       []string
		expected   string
	}{
		{"status field", "deployment", []string{"status", "availableReplicas"}, "${deployment.status.availableReplicas}"},
		{"spec field", "service", []string{"spec", "clusterIP"}, "${service.spec.clusterIP}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, transform.ResourceRef(tt.resourceID, tt.path...))
		})
	}
}

func TestSelfRef(t *testing.T) {
	result := transform.SelfRef("status", "availableReplicas")
	assert.Equal(t, "${self.status.availableReplicas}", result)
}

func TestInterpolate(t *testing.T) {
	result := transform.Interpolate("${schema.spec.image}", ":", "${schema.spec.tag}")
	assert.Equal(t, "${schema.spec.image}:${schema.spec.tag}", result)
}

func TestReadyWhenCondition_String(t *testing.T) {
	c := transform.ReadyWhenCondition{
		Key:      "self.status.availableReplicas",
		Operator: "==",
		Value:    "self.status.replicas",
	}
	assert.Equal(t, "${self.status.availableReplicas == self.status.replicas}", c.String())
}

func TestDefaultReadyWhen_Deployment(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	conditions := transform.DefaultReadyWhen(gvk)
	assert.Len(t, conditions, 1)
	assert.Equal(t, "self.status.availableReplicas", conditions[0].Key)
	assert.Equal(t, "==", conditions[0].Operator)
	assert.Equal(t, "self.status.replicas", conditions[0].Value)
}

func TestDefaultReadyWhen_StatefulSet(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}
	conditions := transform.DefaultReadyWhen(gvk)
	assert.Len(t, conditions, 1)
	assert.Equal(t, "self.status.readyReplicas", conditions[0].Key)
}

func TestDefaultReadyWhen_Service(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
	conditions := transform.DefaultReadyWhen(gvk)
	assert.Len(t, conditions, 1)
	assert.Equal(t, "self.spec.clusterIP", conditions[0].Key)
	assert.Equal(t, "!=", conditions[0].Operator)
	assert.Equal(t, `""`, conditions[0].Value)
}

func TestDefaultReadyWhen_Job(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}
	conditions := transform.DefaultReadyWhen(gvk)
	assert.Len(t, conditions, 1)
	assert.Equal(t, ">", conditions[0].Operator)
}

func TestDefaultReadyWhen_PVC(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}
	conditions := transform.DefaultReadyWhen(gvk)
	assert.Len(t, conditions, 1)
	assert.Equal(t, "==", conditions[0].Operator)
	assert.Contains(t, conditions[0].Value, "Bound")
}

func TestDefaultReadyWhen_Unknown(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	conditions := transform.DefaultReadyWhen(gvk)
	assert.Nil(t, conditions)
}

func TestDefaultReadyWhen_DaemonSet(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}
	conditions := transform.DefaultReadyWhen(gvk)
	assert.Len(t, conditions, 1)
	assert.Equal(t, "self.status.numberReady", conditions[0].Key)
	assert.Equal(t, "==", conditions[0].Operator)
	assert.Equal(t, "self.status.desiredNumberScheduled", conditions[0].Value)
}

func TestDefaultReadyWhen_CronJob(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}
	conditions := transform.DefaultReadyWhen(gvk)
	assert.Nil(t, conditions, "CronJobs should have no readiness conditions")
}

func TestDefaultStatusProjections_Deployment(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	fields := transform.DefaultStatusProjections(gvk, "webDeployment")
	assert.Len(t, fields, 2)
	assert.Equal(t, "webDeploymentAvailableReplicas", fields[0].Name)
	assert.Equal(t, "${webDeployment.status.availableReplicas}", fields[0].CELExpression)
	assert.Equal(t, "webDeploymentReadyReplicas", fields[1].Name)
}

func TestDefaultStatusProjections_StatefulSet(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}
	fields := transform.DefaultStatusProjections(gvk, "db")
	assert.Len(t, fields, 2)
	assert.Equal(t, "dbReadyReplicas", fields[0].Name)
	assert.Equal(t, "dbCurrentReplicas", fields[1].Name)
}

func TestDefaultStatusProjections_Service(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
	fields := transform.DefaultStatusProjections(gvk, "svc")
	assert.Len(t, fields, 2)
	assert.Equal(t, "svcClusterIP", fields[0].Name)
	assert.Equal(t, "svcLoadBalancerIP", fields[1].Name)
	assert.Contains(t, fields[1].CELExpression, ".?ingress[0].?ip")
}

func TestDefaultStatusProjections_Job(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}
	fields := transform.DefaultStatusProjections(gvk, "migrate")
	assert.Len(t, fields, 3)
	assert.Equal(t, "migrateSucceeded", fields[0].Name)
	assert.Equal(t, "migrateFailed", fields[1].Name)
	assert.Equal(t, "migrateCompletionTime", fields[2].Name)
	assert.Contains(t, fields[2].CELExpression, ".?completionTime")
}

func TestDefaultStatusProjections_Unknown(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	fields := transform.DefaultStatusProjections(gvk, "cm")
	assert.Nil(t, fields)
}

func TestDefaultStatusProjections_DaemonSet(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}
	fields := transform.DefaultStatusProjections(gvk, "ds")
	assert.Len(t, fields, 2)
	assert.Equal(t, "dsNumberReady", fields[0].Name)
	assert.Equal(t, "dsDesiredScheduled", fields[1].Name)
}

func TestDefaultStatusProjections_PVC(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}
	fields := transform.DefaultStatusProjections(gvk, "data")
	assert.Len(t, fields, 1)
	assert.Equal(t, "dataPhase", fields[0].Name)
	assert.Contains(t, fields[0].CELExpression, "status.phase")
}

func TestSchemaRef_EmptyPath(t *testing.T) {
	assert.Equal(t, "${schema}", transform.SchemaRef())
}

func TestResourceRef_EmptyPath(t *testing.T) {
	assert.Equal(t, "${deployment}", transform.ResourceRef("deployment"))
}

func TestResourceRefWithOptional(t *testing.T) {
	t.Run("no segments", func(t *testing.T) {
		assert.Equal(t, "${svc}", transform.ResourceRefWithOptional("svc"))
	})

	t.Run("all required", func(t *testing.T) {
		result := transform.ResourceRefWithOptional("dep",
			transform.PathSegment{Name: "status"},
			transform.PathSegment{Name: "availableReplicas"},
		)
		assert.Equal(t, "${dep.status.availableReplicas}", result)
	})

	t.Run("optional fields", func(t *testing.T) {
		result := transform.ResourceRefWithOptional("svc",
			transform.PathSegment{Name: "status"},
			transform.PathSegment{Name: "loadBalancer"},
			transform.PathSegment{Name: "ingress[0]", Optional: true},
			transform.PathSegment{Name: "ip", Optional: true},
		)
		assert.Equal(t, "${svc.status.loadBalancer.?ingress[0].?ip}", result)
	})

	t.Run("single optional", func(t *testing.T) {
		result := transform.ResourceRefWithOptional("job",
			transform.PathSegment{Name: "status"},
			transform.PathSegment{Name: "completionTime", Optional: true},
		)
		assert.Equal(t, "${job.status.?completionTime}", result)
	})
}

func TestSelfRef_EmptyPath(t *testing.T) {
	assert.Equal(t, "${self}", transform.SelfRef())
}

func TestIncludeWhenExpression(t *testing.T) {
	result := transform.IncludeWhenExpression("spec", "monitoring", "enabled")
	assert.Equal(t, "${schema.spec.monitoring.enabled}", result)
}

func TestCompoundIncludeWhen(t *testing.T) {
	t.Run("empty returns empty", func(t *testing.T) {
		assert.Equal(t, "", transform.CompoundIncludeWhen(nil))
	})

	t.Run("single truthiness", func(t *testing.T) {
		result := transform.CompoundIncludeWhen([]transform.IncludeCondition{
			{Path: "monitoring.enabled"},
		})
		assert.Equal(t, "${schema.spec.monitoring.enabled}", result)
	})

	t.Run("single with operator", func(t *testing.T) {
		result := transform.CompoundIncludeWhen([]transform.IncludeCondition{
			{Path: "name", Operator: "!=", Value: `""`},
		})
		assert.Equal(t, `${schema.spec.name != ""}`, result)
	})

	t.Run("two truthiness conditions", func(t *testing.T) {
		result := transform.CompoundIncludeWhen([]transform.IncludeCondition{
			{Path: "monitoring.enabled"},
			{Path: "metrics.enabled"},
		})
		assert.Equal(t, "${schema.spec.monitoring.enabled && schema.spec.metrics.enabled}", result)
	})

	t.Run("mixed conditions", func(t *testing.T) {
		result := transform.CompoundIncludeWhen([]transform.IncludeCondition{
			{Path: "ingress.enabled"},
			{Path: "ingress.host", Operator: "!=", Value: `""`},
		})
		assert.Equal(t, `${schema.spec.ingress.enabled && schema.spec.ingress.host != ""}`, result)
	})

	t.Run("three conditions", func(t *testing.T) {
		result := transform.CompoundIncludeWhen([]transform.IncludeCondition{
			{Path: "a"},
			{Path: "b"},
			{Path: "c"},
		})
		assert.Equal(t, "${schema.spec.a && schema.spec.b && schema.spec.c}", result)
	})
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, k8s.APIVersion(tt.gvk))
		})
	}
}

func TestValidateExpression(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
		errMsg  string
	}{
		{"valid simple", "${schema.spec.replicas}", false, ""},
		{"valid interpolation", "${schema.spec.image.repo}:${schema.spec.image.tag}", false, ""},
		{"valid self ref", "${self.status.availableReplicas == self.status.replicas}", false, ""},
		{"valid compound", "${schema.spec.a && schema.spec.b}", false, ""},
		{"empty string", "", true, "empty CEL expression"},
		{"no expression", "plain text", true, "no CEL expression found"},
		{"unbalanced open", "${schema.spec.replicas", true, "unbalanced CEL expression delimiters"},
		{"unbalanced close", "schema.spec.replicas}", true, "no CEL expression found"},
		{"missing closing brace", "${foo ${bar}", true, "unbalanced CEL expression delimiters"},
		{"valid literal around expr", "prefix-${schema.spec.name}-suffix", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := transform.ValidateExpression(tt.expr)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
