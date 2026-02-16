package k8s_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

func TestResource_APIVersion(t *testing.T) {
	t.Run("from object", func(t *testing.T) {
		r := &k8s.Resource{
			GVK: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
				},
			},
		}
		assert.Equal(t, "apps/v1", r.APIVersion())
	})

	t.Run("nil object falls back to GVK", func(t *testing.T) {
		r := &k8s.Resource{
			GVK: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		}
		assert.Equal(t, "apps/v1", r.APIVersion())
	})

	t.Run("core group", func(t *testing.T) {
		r := &k8s.Resource{
			GVK: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
		}
		assert.Equal(t, "v1", r.APIVersion())
	})
}

func TestResource_Kind(t *testing.T) {
	r := &k8s.Resource{
		GVK: schema.GroupVersionKind{Kind: "ConfigMap"},
	}
	assert.Equal(t, "ConfigMap", r.Kind())
}

func TestResource_QualifiedName(t *testing.T) {
	r := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Kind: "Deployment"},
		Name: "nginx",
	}
	assert.Equal(t, "Deployment/nginx", r.QualifiedName())
}

func TestExtractSubchart(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"root template", "my-chart/templates/deployment.yaml", ""},
		{"subchart template", "my-chart/charts/postgresql/templates/statefulset.yaml", "postgresql"},
		{"nested subchart", "root/charts/backend/charts/postgresql/templates/sts.yaml", "backend"},
		{"no charts dir", "some/random/path/file.yaml", ""},
		{"empty path", "", ""},
		{"charts at end", "my-chart/charts/", ""},
		{"charts with name only", "my-chart/charts/redis", "redis"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, k8s.ExtractSubchart(tt.path))
		})
	}
}

func TestResource_SourceChart(t *testing.T) {
	t.Run("empty source path", func(t *testing.T) {
		r := &k8s.Resource{}
		assert.Equal(t, "", r.SourceChart())
	})

	t.Run("root chart source", func(t *testing.T) {
		r := &k8s.Resource{SourcePath: "my-chart/templates/deploy.yaml"}
		assert.Equal(t, "", r.SourceChart())
	})

	t.Run("subchart source", func(t *testing.T) {
		r := &k8s.Resource{SourcePath: "my-chart/charts/redis/templates/deploy.yaml"}
		assert.Equal(t, "redis", r.SourceChart())
	})
}
