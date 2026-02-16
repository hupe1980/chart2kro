package transform_test

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

func makeResource(kind, name string) *k8s.Resource {
	return &k8s.Resource{
		GVK:  schema.GroupVersionKind{Kind: kind},
		Name: name,
	}
}

func makeFullResource(apiVersion, kind, name string, obj map[string]interface{}) *k8s.Resource {
	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)
	r := &k8s.Resource{
		GVK:  gvk,
		Name: name,
	}

	if obj != nil {
		obj["apiVersion"] = apiVersion
		obj["kind"] = kind
		obj["metadata"] = map[string]interface{}{"name": name}
		r.Object = &unstructured.Unstructured{Object: obj}
	}

	return r
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}

	return -1
}
