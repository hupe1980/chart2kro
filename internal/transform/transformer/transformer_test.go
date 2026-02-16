package transformer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/config"
	"github.com/hupe1980/chart2kro/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ---------------------------------------------------------------------------
// MockTransformer
// ---------------------------------------------------------------------------

type mockTransformer struct {
	name    string
	matches func(schema.GroupVersionKind) bool
}

func (m *mockTransformer) Name() string { return m.name }

func (m *mockTransformer) Matches(gvk schema.GroupVersionKind) bool {
	return m.matches(gvk)
}

func (m *mockTransformer) Transform(_ context.Context, _ TransformInput) (*TransformOutput, error) {
	return &TransformOutput{}, nil
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

func TestRegistry_Register_And_Match(t *testing.T) {
	r := NewRegistry()

	deployT := &mockTransformer{
		name:    "deploy",
		matches: func(gvk schema.GroupVersionKind) bool { return gvk.Kind == "Deployment" },
	}
	fallback := &mockTransformer{
		name:    "fallback",
		matches: func(_ schema.GroupVersionKind) bool { return true },
	}

	r.Register(deployT)
	r.Register(fallback)

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	got := r.TransformerFor(gvk)
	require.NotNil(t, got)
	assert.Equal(t, "deploy", got.Name())

	// Non-deployment should fall through to fallback.
	cmGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	got2 := r.TransformerFor(cmGVK)
	require.NotNil(t, got2)
	assert.Equal(t, "fallback", got2.Name())
}

func TestRegistry_Prepend(t *testing.T) {
	r := NewRegistry()

	low := &mockTransformer{
		name:    "low",
		matches: func(_ schema.GroupVersionKind) bool { return true },
	}
	high := &mockTransformer{
		name:    "high",
		matches: func(_ schema.GroupVersionKind) bool { return true },
	}

	r.Register(low)
	r.Prepend(high)

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	got := r.TransformerFor(gvk)
	require.NotNil(t, got)
	assert.Equal(t, "high", got.Name(), "prepended transformer should win")
}

func TestRegistry_All(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTransformer{name: "a", matches: func(_ schema.GroupVersionKind) bool { return true }})
	r.Register(&mockTransformer{name: "b", matches: func(_ schema.GroupVersionKind) bool { return true }})

	all := r.All()
	require.Len(t, all, 2)
	assert.Equal(t, "a", all[0].Name())
	assert.Equal(t, "b", all[1].Name())
}

func TestRegistry_NoMatch(t *testing.T) {
	r := NewRegistry()

	never := &mockTransformer{
		name:    "never",
		matches: func(_ schema.GroupVersionKind) bool { return false },
	}
	r.Register(never)

	got := r.TransformerFor(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"})
	assert.Nil(t, got)
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	all := r.All()
	require.Len(t, all, 4)
	assert.Equal(t, "deployment", all[0].Name())
	assert.Equal(t, "service", all[1].Name())
	assert.Equal(t, "configmap", all[2].Name())
	assert.Equal(t, "default", all[3].Name())
}

// ---------------------------------------------------------------------------
// Built-In Transformers
// ---------------------------------------------------------------------------

func newResource(gvk schema.GroupVersionKind, name string) *k8s.Resource {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName(name)

	return &k8s.Resource{
		GVK:    gvk,
		Name:   name,
		Object: u,
	}
}

func TestDeploymentTransformer_Matches(t *testing.T) {
	dt := &DeploymentTransformer{}

	assert.True(t, dt.Matches(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}))
	assert.True(t, dt.Matches(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}))
	assert.True(t, dt.Matches(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}))
	assert.False(t, dt.Matches(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}))
}

func TestDeploymentTransformer_Transform(t *testing.T) {
	dt := &DeploymentTransformer{}
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	res := newResource(gvk, "web")

	out, err := dt.Transform(context.Background(), TransformInput{
		Resource:   res,
		ResourceID: "webDeployment",
		Values:     map[string]interface{}{"replicas": 3},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.ReadyWhen, "should have readyWhen for Deployment")
	assert.NotEmpty(t, out.StatusFields, "should have status fields for Deployment")
}

func TestServiceTransformer_Transform(t *testing.T) {
	st := &ServiceTransformer{}
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
	res := newResource(gvk, "mysvc")

	out, err := st.Transform(context.Background(), TransformInput{
		Resource:   res,
		ResourceID: "mysvc",
		Values:     map[string]interface{}{},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.ReadyWhen, "should have readyWhen for Service")
	assert.NotEmpty(t, out.StatusFields, "should have status fields for Service")
}

func TestConfigMapTransformer_Transform(t *testing.T) {
	cmt := &ConfigMapTransformer{}
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	res := newResource(gvk, "myconfig")

	assert.True(t, cmt.Matches(gvk))

	out, err := cmt.Transform(context.Background(), TransformInput{
		Resource:   res,
		ResourceID: "myconfig",
		Values:     map[string]interface{}{},
	})
	require.NoError(t, err)
	assert.Empty(t, out.ReadyWhen, "ConfigMap has no readyWhen")
	assert.Empty(t, out.StatusFields, "ConfigMap has no status fields")
}

func TestDefaultTransformer_MatchesEverything(t *testing.T) {
	dt := &DefaultTransformer{}

	assert.True(t, dt.Matches(schema.GroupVersionKind{Kind: "Anything"}))
	assert.True(t, dt.Matches(schema.GroupVersionKind{Kind: "Deployment"}))
}

func TestDefaultTransformer_Transform(t *testing.T) {
	dt := &DefaultTransformer{}
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}
	res := newResource(gvk, "mysecret")

	out, err := dt.Transform(context.Background(), TransformInput{
		Resource:   res,
		ResourceID: "mysecret",
		Values:     map[string]interface{}{},
	})
	require.NoError(t, err)
	// Secret has no default readyWhen or status projections.
	assert.Empty(t, out.ReadyWhen)
	assert.Empty(t, out.StatusFields)
}

func TestDefaultTransformer_TransformWithKnownGVK(t *testing.T) {
	dt := &DefaultTransformer{}
	gvk := schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}
	res := newResource(gvk, "migrate")

	out, err := dt.Transform(context.Background(), TransformInput{
		Resource:   res,
		ResourceID: "migrate",
		Values:     map[string]interface{}{},
	})
	require.NoError(t, err)
	// Job has default readyWhen and status projections.
	assert.NotEmpty(t, out.ReadyWhen)
	assert.NotEmpty(t, out.StatusFields)
}

// ---------------------------------------------------------------------------
// readyWhenToStrings
// ---------------------------------------------------------------------------

func TestReadyWhenToStrings_Empty(t *testing.T) {
	assert.Nil(t, readyWhenToStrings(nil))
}

// ---------------------------------------------------------------------------
// configOverrideTransformer / FromConfigOverride
// ---------------------------------------------------------------------------

func TestConfigOverride_Name(t *testing.T) {
	cot := FromConfigOverride(config.TransformerOverride{
		Match: config.TransformerMatch{Kind: "Deployment"},
	})
	assert.Equal(t, "config:Deployment", cot.Name())
}

func TestConfigOverride_Matches_KindOnly(t *testing.T) {
	cot := FromConfigOverride(config.TransformerOverride{
		Match: config.TransformerMatch{Kind: "Deployment"},
	})

	assert.True(t, cot.Matches(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}))
	assert.True(t, cot.Matches(schema.GroupVersionKind{Group: "extensions", Version: "v1beta1", Kind: "Deployment"}))
	assert.False(t, cot.Matches(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}))
}

func TestConfigOverride_Matches_KindAndAPIVersion(t *testing.T) {
	cot := FromConfigOverride(config.TransformerOverride{
		Match: config.TransformerMatch{Kind: "Deployment", APIVersion: "apps/v1"},
	})

	assert.True(t, cot.Matches(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}))
	assert.False(t, cot.Matches(schema.GroupVersionKind{Group: "extensions", Version: "v1beta1", Kind: "Deployment"}))
}

func TestConfigOverride_Transform_CustomReadyWhenAndStatus(t *testing.T) {
	cot := FromConfigOverride(config.TransformerOverride{
		Match:     config.TransformerMatch{Kind: "Deployment"},
		ReadyWhen: []string{"${self.status.ready}"},
		StatusFields: []config.StatusFieldOverride{
			{Name: "ready", CELExpression: "${deployment.status.readyReplicas}"},
		},
	})

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	res := newResource(gvk, "web")

	out, err := cot.Transform(context.Background(), TransformInput{
		Resource:   res,
		ResourceID: "webDeployment",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"${self.status.ready}"}, out.ReadyWhen)
	require.Len(t, out.StatusFields, 1)
	assert.Equal(t, "ready", out.StatusFields[0].Name)
	assert.Equal(t, "${deployment.status.readyReplicas}", out.StatusFields[0].CELExpression)
}

func TestConfigOverride_Transform_FallbackToDefaults(t *testing.T) {
	// Empty readyWhen and statusFields — should fall back to defaults.
	cot := FromConfigOverride(config.TransformerOverride{
		Match: config.TransformerMatch{Kind: "Deployment"},
	})

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	res := newResource(gvk, "web")

	out, err := cot.Transform(context.Background(), TransformInput{
		Resource:   res,
		ResourceID: "webDeployment",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.ReadyWhen, "should fall back to default readyWhen")
	assert.NotEmpty(t, out.StatusFields, "should fall back to default status fields")
}

func TestConfigOverride_RegistryPriority(t *testing.T) {
	// Config override should take priority over built-in deployment transformer.
	r := DefaultRegistry()
	override := FromConfigOverride(config.TransformerOverride{
		Match:     config.TransformerMatch{Kind: "Deployment"},
		ReadyWhen: []string{"${self.status.custom}"},
	})
	r.Prepend(override)

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	got := r.TransformerFor(gvk)
	require.NotNil(t, got)
	assert.Equal(t, "config:Deployment", got.Name(), "config override should win")
}
