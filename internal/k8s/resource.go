// Package k8s provides Kubernetes resource abstractions for parsed manifests.
package k8s

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Resource represents a parsed Kubernetes resource with its GVK, metadata,
// and full unstructured representation.
type Resource struct {
	// GVK is the GroupVersionKind of the resource.
	GVK schema.GroupVersionKind

	// Name is metadata.name.
	Name string

	// Namespace is metadata.namespace (may be empty for cluster-scoped).
	Namespace string

	// Labels from metadata.labels.
	Labels map[string]string

	// Annotations from metadata.annotations.
	Annotations map[string]string

	// SourcePath is the Helm template path that produced this resource
	// (e.g., "my-chart/charts/postgresql/templates/statefulset.yaml").
	// Empty when the source is unknown.
	SourcePath string

	// Object is the full unstructured representation.
	Object *unstructured.Unstructured
}

// APIVersion returns the apiVersion string (e.g. "apps/v1").
func (r *Resource) APIVersion() string {
	if r.Object != nil {
		return r.Object.GetAPIVersion()
	}

	return r.GVK.GroupVersion().String()
}

// Kind returns the resource kind (e.g. "Deployment").
func (r *Resource) Kind() string {
	return r.GVK.Kind
}

// QualifiedName returns "kind/name" for display purposes.
func (r *Resource) QualifiedName() string {
	return r.GVK.Kind + "/" + r.Name
}

// SourceChart returns the subchart name that produced this resource,
// or empty string if the resource comes from the root chart.
// It detects paths like "my-chart/charts/postgresql/templates/foo.yaml".
func (r *Resource) SourceChart() string {
	if r.SourcePath == "" {
		return ""
	}

	return ExtractSubchart(r.SourcePath)
}

// ExtractSubchart extracts the subchart name from a Helm template path.
// Example: "my-chart/charts/postgresql/templates/foo.yaml" → "postgresql".
// Returns empty string for root chart templates.
func ExtractSubchart(templatePath string) string {
	// Helm template paths are like:
	//   rootChart/templates/deployment.yaml         (root chart)
	//   rootChart/charts/subchartName/templates/...  (subchart)
	const chartsDir = "/charts/"

	idx := strings.Index(templatePath, chartsDir)
	if idx < 0 {
		return ""
	}

	rest := templatePath[idx+len(chartsDir):]

	// Extract the subchart name (next path segment).
	if slashIdx := strings.Index(rest, "/"); slashIdx > 0 {
		return rest[:slashIdx]
	}

	return rest
}

// WorkloadKinds defines the Kubernetes kinds that contain pod templates.
// This is shared between audit and harden packages to maintain consistency.
var WorkloadKinds = map[string]bool{
	"Deployment":  true,
	"StatefulSet": true,
	"DaemonSet":   true,
	"Job":         true,
	"CronJob":     true,
	"ReplicaSet":  true,
}

// IsWorkloadKind returns true if the kind represents a pod-bearing workload.
func IsWorkloadKind(kind string) bool {
	return WorkloadKinds[kind]
}
