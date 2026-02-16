package transformer

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/config"
	"github.com/hupe1980/chart2kro/internal/transform"
)

// configOverrideTransformer adapts a config.TransformerOverride into
// the Transformer interface so it can be prepended to the registry.
type configOverrideTransformer struct {
	override config.TransformerOverride
}

// FromConfigOverride creates a Transformer from a config-based override.
func FromConfigOverride(override config.TransformerOverride) Transformer {
	return &configOverrideTransformer{override: override}
}

func (t *configOverrideTransformer) Name() string {
	return "config:" + t.override.Match.Kind
}

func (t *configOverrideTransformer) Matches(gvk schema.GroupVersionKind) bool {
	if gvk.Kind != t.override.Match.Kind {
		return false
	}

	if t.override.Match.APIVersion != "" {
		apiVersion := gvk.GroupVersion().String()
		return apiVersion == t.override.Match.APIVersion
	}

	return true
}

func (t *configOverrideTransformer) Transform(_ context.Context, input TransformInput) (*TransformOutput, error) {
	out := &TransformOutput{}

	// Use config-specified readyWhen, or fall back to defaults.
	if len(t.override.ReadyWhen) > 0 {
		out.ReadyWhen = t.override.ReadyWhen
	} else {
		readyWhen := transform.DefaultReadyWhen(input.Resource.GVK)
		out.ReadyWhen = readyWhenToStrings(readyWhen)
	}

	// Use config-specified status fields, or fall back to defaults.
	if len(t.override.StatusFields) > 0 {
		for _, sf := range t.override.StatusFields {
			out.StatusFields = append(out.StatusFields, transform.StatusField{
				Name:          sf.Name,
				CELExpression: sf.CELExpression,
			})
		}
	} else {
		out.StatusFields = transform.DefaultStatusProjections(input.Resource.GVK, input.ResourceID)
	}

	return out, nil
}

// Compile-time interface check.
var _ Transformer = (*configOverrideTransformer)(nil)
