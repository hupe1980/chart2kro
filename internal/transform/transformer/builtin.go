package transformer

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/transform"
)

// ---------------------------------------------------------------------------
// DeploymentTransformer
// ---------------------------------------------------------------------------

// DeploymentTransformer handles Deployment and StatefulSet resources with
// replica CEL, container image CEL, readyWhen, and status projections.
type DeploymentTransformer struct{}

// Name returns the transformer name.
func (t *DeploymentTransformer) Name() string { return "deployment" }

// Matches returns true if this transformer handles the given GVK.
func (t *DeploymentTransformer) Matches(gvk schema.GroupVersionKind) bool {
	return isDeploymentLike(gvk)
}

// Transform applies the transformation.
func (t *DeploymentTransformer) Transform(_ context.Context, input TransformInput) (*TransformOutput, error) {
	gvk := input.Resource.GVK

	readyWhen := transform.DefaultReadyWhen(gvk)
	statusFields := transform.DefaultStatusProjections(gvk, input.ResourceID)

	return &TransformOutput{
		ReadyWhen:    readyWhenToStrings(readyWhen),
		StatusFields: statusFields,
	}, nil
}

// ---------------------------------------------------------------------------
// ServiceTransformer
// ---------------------------------------------------------------------------

// ServiceTransformer handles Service resources with selector sharing,
// port CEL, readyWhen, and status projections.
type ServiceTransformer struct{}

// Name returns the transformer name.
func (t *ServiceTransformer) Name() string { return "service" }

// Matches returns true if this transformer handles the given GVK.
func (t *ServiceTransformer) Matches(gvk schema.GroupVersionKind) bool {
	return k8s.IsService(gvk)
}

// Transform applies the transformation.
func (t *ServiceTransformer) Transform(_ context.Context, input TransformInput) (*TransformOutput, error) {
	gvk := input.Resource.GVK

	readyWhen := transform.DefaultReadyWhen(gvk)
	statusFields := transform.DefaultStatusProjections(gvk, input.ResourceID)

	return &TransformOutput{
		ReadyWhen:    readyWhenToStrings(readyWhen),
		StatusFields: statusFields,
	}, nil
}

// ---------------------------------------------------------------------------
// ConfigMapTransformer
// ---------------------------------------------------------------------------

// ConfigMapTransformer handles ConfigMap resources with data field CEL mapping.
type ConfigMapTransformer struct{}

// Name returns the transformer name.
func (t *ConfigMapTransformer) Name() string { return "configmap" }

// Matches returns true if this transformer handles the given GVK.
func (t *ConfigMapTransformer) Matches(gvk schema.GroupVersionKind) bool {
	return gvk.Kind == "ConfigMap" && gvk.Group == ""
}

// Transform applies the transformation.
func (t *ConfigMapTransformer) Transform(_ context.Context, input TransformInput) (*TransformOutput, error) {
	// ConfigMaps have no default ready conditions or status projections.
	return &TransformOutput{}, nil
}

// ---------------------------------------------------------------------------
// DefaultTransformer
// ---------------------------------------------------------------------------

// DefaultTransformer is the fallback for resource kinds that have no
// specific transformer. It applies default readyWhen and status projections
// based on GVK.
type DefaultTransformer struct{}

// Name returns the transformer name.
func (t *DefaultTransformer) Name() string { return "default" }

// Matches returns true for any GVK (fallback transformer).
func (t *DefaultTransformer) Matches(_ schema.GroupVersionKind) bool {
	return true // always matches as fallback
}

// Transform applies the transformation.
func (t *DefaultTransformer) Transform(_ context.Context, input TransformInput) (*TransformOutput, error) {
	gvk := input.Resource.GVK

	readyWhen := transform.DefaultReadyWhen(gvk)
	statusFields := transform.DefaultStatusProjections(gvk, input.ResourceID)

	return &TransformOutput{
		ReadyWhen:    readyWhenToStrings(readyWhen),
		StatusFields: statusFields,
	}, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func isDeploymentLike(gvk schema.GroupVersionKind) bool {
	switch gvk.Kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		return gvk.Group == "apps" || gvk.Group == ""
	default:
		return false
	}
}

// readyWhenToStrings converts ReadyWhenCondition to CEL-style strings.
func readyWhenToStrings(conditions []transform.ReadyWhenCondition) []string {
	if len(conditions) == 0 {
		return nil
	}

	result := make([]string, 0, len(conditions))
	for _, c := range conditions {
		expr := c.Key
		if c.Operator != "" && c.Value != "" {
			expr += " " + c.Operator + " " + c.Value
		}

		result = append(result, expr)
	}

	return result
}

// Compile-time interface checks.
var (
	_ Transformer = (*DeploymentTransformer)(nil)
	_ Transformer = (*ServiceTransformer)(nil)
	_ Transformer = (*ConfigMapTransformer)(nil)
	_ Transformer = (*DefaultTransformer)(nil)
)
