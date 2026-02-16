package harden

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// makeDeployment creates a minimal Deployment resource for testing.
func makeDeployment(name string, containers []interface{}) *k8s.Resource {
	podSpec := map[string]interface{}{
		"containers": containers,
	}

	return &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: name,
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": name},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": podSpec,
					},
				},
			},
		},
	}
}

// makeContainer creates a container map for testing.
func makeContainer(name, image string) map[string]interface{} {
	return map[string]interface{}{
		"name":  name,
		"image": image,
	}
}

// deploymentGVK returns the standard Deployment GVK for tests.
func deploymentGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
}

// makeUnstructuredWorkload creates an unstructured workload object for testing.
func makeUnstructuredWorkload(kind, name string, podSpecOverrides map[string]interface{}) *unstructured.Unstructured {
	podSpec := map[string]interface{}{}
	for k, v := range podSpecOverrides {
		podSpec[k] = v
	}

	spec := map[string]interface{}{
		"template": map[string]interface{}{
			"spec": podSpec,
		},
	}

	if kind == "CronJob" {
		spec = map[string]interface{}{
			"jobTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": podSpec,
					},
				},
			},
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       kind,
			"metadata":   map[string]interface{}{"name": name},
			"spec":       spec,
		},
	}
}

// makeService creates a Service resource for testing.
func makeService(name string, selector map[string]interface{}, ports []interface{}) *k8s.Resource {
	spec := map[string]interface{}{}
	if selector != nil {
		spec["selector"] = selector
	}

	if ports != nil {
		spec["ports"] = ports
	}

	return &k8s.Resource{
		GVK:  schema.GroupVersionKind{Version: "v1", Kind: "Service"},
		Name: name,
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata":   map[string]interface{}{"name": name},
				"spec":       spec,
			},
		},
	}
}

// makeDeploymentWithSelector creates a Deployment with spec.selector.matchLabels.
func makeDeploymentWithSelector(name string, labels map[string]interface{}, containers []interface{}) *k8s.Resource {
	return &k8s.Resource{
		GVK:  deploymentGVK(),
		Name: name,
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": name},
				"spec": map[string]interface{}{
					"selector": map[string]interface{}{
						"matchLabels": labels,
					},
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": containers,
						},
					},
				},
			},
		},
	}
}
