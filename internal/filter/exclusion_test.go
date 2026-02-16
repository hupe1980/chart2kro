package filter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func makeResource(kind, name string) *k8s.Resource {
	return &k8s.Resource{
		GVK:    schema.GroupVersionKind{Kind: kind},
		Name:   name,
		Labels: map[string]string{},
	}
}

func makeResourceWithLabels(kind, name string, labels map[string]string) *k8s.Resource {
	return &k8s.Resource{
		GVK:    schema.GroupVersionKind{Kind: kind},
		Name:   name,
		Labels: labels,
	}
}

func makeResourceWithSource(kind, name, sourcePath string) *k8s.Resource {
	return &k8s.Resource{
		GVK:        schema.GroupVersionKind{Kind: kind},
		Name:       name,
		Labels:     map[string]string{},
		SourcePath: sourcePath,
	}
}

// makeResources is a convenience wrapper for building resource slices
// without explicit type annotations in test files.
func makeResources(rs ...*k8s.Resource) []*k8s.Resource {
	return rs
}

// ---------------------------------------------------------------------------
// Chain tests
// ---------------------------------------------------------------------------

func TestChain_Empty(t *testing.T) {
	resources := []*k8s.Resource{makeResource("Deployment", "app")}
	result, err := NewChain().Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 1)
	assert.Empty(t, result.Excluded)
}

func TestChain_MultipleFilters(t *testing.T) {
	resources := []*k8s.Resource{
		makeResource("Deployment", "app"),
		makeResource("StatefulSet", "db"),
		makeResource("Service", "app-svc"),
		makeResource("PersistentVolumeClaim", "db-data"),
	}

	chain := NewChain(
		NewKindFilter([]string{"StatefulSet"}),
		NewKindFilter([]string{"PersistentVolumeClaim"}),
	)

	result, err := chain.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 2)
	assert.Len(t, result.Excluded, 2)
	assert.Equal(t, "Deployment", result.Included[0].Kind())
	assert.Equal(t, "Service", result.Included[1].Kind())
}

func TestChain_AccumulatesExclusions(t *testing.T) {
	resources := []*k8s.Resource{
		makeResource("Deployment", "app"),
		makeResource("StatefulSet", "db"),
		makeResource("Service", "app-svc"),
	}

	chain := NewChain(
		NewKindFilter([]string{"StatefulSet"}),
		NewKindFilter([]string{"Service"}),
	)

	result, err := chain.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 1)
	assert.Len(t, result.Excluded, 2)
	assert.Contains(t, result.Excluded[0].Reason, "StatefulSet")
	assert.Contains(t, result.Excluded[1].Reason, "Service")
}

// ---------------------------------------------------------------------------
// KindFilter tests
// ---------------------------------------------------------------------------

func TestKindFilter_ExcludesMatchingKinds(t *testing.T) {
	resources := []*k8s.Resource{
		makeResource("Deployment", "app"),
		makeResource("StatefulSet", "db"),
		makeResource("Service", "svc"),
	}

	f := NewKindFilter([]string{"StatefulSet", "Service"})

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 1)
	assert.Equal(t, "Deployment", result.Included[0].Kind())
	assert.Len(t, result.Excluded, 2)
}

func TestKindFilter_CaseInsensitive(t *testing.T) {
	resources := []*k8s.Resource{makeResource("StatefulSet", "db")}

	f := NewKindFilter([]string{"statefulset"})

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Empty(t, result.Included)
	assert.Len(t, result.Excluded, 1)
}

func TestKindFilter_NoMatch(t *testing.T) {
	resources := []*k8s.Resource{makeResource("Deployment", "app")}

	f := NewKindFilter([]string{"StatefulSet"})

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 1)
	assert.Empty(t, result.Excluded)
}

func TestKindFilter_ExcludesWithReason(t *testing.T) {
	resources := []*k8s.Resource{makeResource("StatefulSet", "db")}

	f := NewKindFilter([]string{"StatefulSet"})

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	require.Len(t, result.Excluded, 1)
	assert.Equal(t, "excluded by kind: StatefulSet", result.Excluded[0].Reason)
}

// ---------------------------------------------------------------------------
// ResourceIDFilter tests
// ---------------------------------------------------------------------------

func TestResourceIDFilter_ExcludesMatchingIDs(t *testing.T) {
	r1 := makeResource("Service", "pg-svc")
	r2 := makeResource("Deployment", "app")
	r3 := makeResource("StatefulSet", "pg-db")

	ids := map[*k8s.Resource]string{
		r1: "postgresql",
		r2: "deployment",
		r3: "postgresql-db",
	}

	f := NewResourceIDFilter([]string{"postgresql", "postgresql-db"}, ids)

	result, err := f.Apply(context.Background(), []*k8s.Resource{r1, r2, r3})
	require.NoError(t, err)
	assert.Len(t, result.Included, 1)
	assert.Equal(t, "app", result.Included[0].Name)
	assert.Len(t, result.Excluded, 2)
}

func TestResourceIDFilter_NoMatch(t *testing.T) {
	r1 := makeResource("Deployment", "app")

	ids := map[*k8s.Resource]string{r1: "deployment"}

	f := NewResourceIDFilter([]string{"postgresql"}, ids)

	result, err := f.Apply(context.Background(), []*k8s.Resource{r1})
	require.NoError(t, err)
	assert.Len(t, result.Included, 1)
	assert.Empty(t, result.Excluded)
}

func TestResourceIDFilter_ExcludesWithReason(t *testing.T) {
	r1 := makeResource("StatefulSet", "db")

	ids := map[*k8s.Resource]string{r1: "postgresql"}

	f := NewResourceIDFilter([]string{"postgresql"}, ids)

	result, err := f.Apply(context.Background(), []*k8s.Resource{r1})
	require.NoError(t, err)
	require.Len(t, result.Excluded, 1)
	assert.Equal(t, "excluded by resource ID: postgresql", result.Excluded[0].Reason)
}

// ---------------------------------------------------------------------------
// SubchartFilter tests
// ---------------------------------------------------------------------------

func TestSubchartFilter_ExcludesSubchartResources(t *testing.T) {
	resources := []*k8s.Resource{
		makeResourceWithSource("Deployment", "app", "my-chart/templates/deployment.yaml"),
		makeResourceWithSource("StatefulSet", "pg", "my-chart/charts/postgresql/templates/statefulset.yaml"),
		makeResourceWithSource("Service", "pg-svc", "my-chart/charts/postgresql/templates/svc.yaml"),
		makeResourceWithSource("Service", "app-svc", "my-chart/templates/service.yaml"),
	}

	f := NewSubchartFilter([]string{"postgresql"})

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 2)
	assert.Equal(t, "app", result.Included[0].Name)
	assert.Equal(t, "app-svc", result.Included[1].Name)
	assert.Len(t, result.Excluded, 2)
}

func TestSubchartFilter_CaseInsensitive(t *testing.T) {
	resources := []*k8s.Resource{
		makeResourceWithSource("StatefulSet", "pg", "my-chart/charts/PostgreSQL/templates/sts.yaml"),
	}

	f := NewSubchartFilter([]string{"postgresql"})

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Empty(t, result.Included)
}

func TestSubchartFilter_NestedSubchart(t *testing.T) {
	// ExtractSubchart finds the first subchart in the path — here "backend".
	// For nested, we match the first level. This is intentional: users target
	// the immediate dependency.
	resources := []*k8s.Resource{
		makeResourceWithSource("StatefulSet", "pg", "root/charts/backend/charts/postgresql/templates/sts.yaml"),
	}

	f := NewSubchartFilter([]string{"backend"})

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Empty(t, result.Included)
}

func TestSubchartFilter_NoSourcePath(t *testing.T) {
	resources := []*k8s.Resource{makeResource("Deployment", "app")}

	f := NewSubchartFilter([]string{"postgresql"})

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 1, "resources without SourcePath should not be excluded")
}

func TestSubchartFilter_ExcludesWithReason(t *testing.T) {
	resources := []*k8s.Resource{
		makeResourceWithSource("StatefulSet", "pg", "c/charts/postgresql/templates/sts.yaml"),
	}

	f := NewSubchartFilter([]string{"postgresql"})

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	require.Len(t, result.Excluded, 1)
	assert.Equal(t, "excluded by subchart: postgresql", result.Excluded[0].Reason)
}

// ---------------------------------------------------------------------------
// LabelFilter tests
// ---------------------------------------------------------------------------

func TestLabelFilter_EqualityMatch(t *testing.T) {
	resources := []*k8s.Resource{
		makeResourceWithLabels("Deployment", "app", map[string]string{"component": "web"}),
		makeResourceWithLabels("StatefulSet", "db", map[string]string{"component": "database"}),
	}

	f, err := NewLabelFilter("component=database")
	require.NoError(t, err)

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 1)
	assert.Equal(t, "app", result.Included[0].Name)
}

func TestLabelFilter_InequalityMatch(t *testing.T) {
	resources := []*k8s.Resource{
		makeResourceWithLabels("Deployment", "app", map[string]string{"component": "web"}),
		makeResourceWithLabels("StatefulSet", "db", map[string]string{"component": "database"}),
	}

	// component!=web: exclude resources where component is NOT "web".
	// Deployment has component=web → label exists and matches → selector returns false → INCLUDED
	// StatefulSet has component=database → label exists but value != web → selector returns true → EXCLUDED
	f, err := NewLabelFilter("component!=web")
	require.NoError(t, err)

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 1)
	assert.Equal(t, "app", result.Included[0].Name)
	assert.Len(t, result.Excluded, 1)
	assert.Equal(t, "db", result.Excluded[0].Resource.Name)
}

func TestLabelFilter_InMatch(t *testing.T) {
	resources := []*k8s.Resource{
		makeResourceWithLabels("Deployment", "app", map[string]string{"env": "prod"}),
		makeResourceWithLabels("Service", "svc", map[string]string{"env": "staging"}),
		makeResourceWithLabels("ConfigMap", "cm", map[string]string{"env": "dev"}),
	}

	f, err := NewLabelFilter("env in (staging,dev)")
	require.NoError(t, err)

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 1)
	assert.Equal(t, "app", result.Included[0].Name)
}

func TestLabelFilter_MultipleSelectors(t *testing.T) {
	resources := []*k8s.Resource{
		makeResourceWithLabels("StatefulSet", "db", map[string]string{
			"component": "database",
			"tier":      "backend",
		}),
		makeResourceWithLabels("Deployment", "app", map[string]string{
			"component": "web",
			"tier":      "frontend",
		}),
		makeResourceWithLabels("Service", "db-svc", map[string]string{
			"component": "database",
			"tier":      "frontend",
		}),
	}

	// Both selectors must match (AND semantics).
	f, err := NewLabelFilter("component=database,tier=backend")
	require.NoError(t, err)

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 2) // only db matches BOTH selectors
	assert.Len(t, result.Excluded, 1)
	assert.Equal(t, "db", result.Excluded[0].Resource.Name)
}

func TestLabelFilter_NoMatch(t *testing.T) {
	resources := []*k8s.Resource{
		makeResourceWithLabels("Deployment", "app", map[string]string{"component": "web"}),
	}

	f, err := NewLabelFilter("component=database")
	require.NoError(t, err)

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 1)
}

func TestLabelFilter_InvalidSelector(t *testing.T) {
	_, err := NewLabelFilter("invalid-no-operator")
	assert.Error(t, err)
}

func TestLabelFilter_MissingLabel(t *testing.T) {
	resources := []*k8s.Resource{
		makeResourceWithLabels("Deployment", "app", map[string]string{}),
	}

	f, err := NewLabelFilter("component=database")
	require.NoError(t, err)

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Included, 1, "resource without the label shouldn't match equality selector")
}

func TestLabelFilter_InequalityMissingLabel(t *testing.T) {
	// component!=web: a resource without the label → label doesn't exist → selector matches → EXCLUDED
	resources := []*k8s.Resource{
		makeResourceWithLabels("Deployment", "app", map[string]string{}),
	}

	f, err := NewLabelFilter("component!=web")
	require.NoError(t, err)

	result, err := f.Apply(context.Background(), resources)
	require.NoError(t, err)
	assert.Len(t, result.Excluded, 1, "resource missing the label is matched by != selector")
}

// ---------------------------------------------------------------------------
// Helper tests
// ---------------------------------------------------------------------------

func TestParseLabelSelector_Equal(t *testing.T) {
	sel, err := parseLabelSelector("key=value")
	require.NoError(t, err)
	assert.Equal(t, "key", sel.key)
	assert.Equal(t, labelOpEqual, sel.op)
	assert.Equal(t, []string{"value"}, sel.values)
}

func TestParseLabelSelector_NotEqual(t *testing.T) {
	sel, err := parseLabelSelector("key!=value")
	require.NoError(t, err)
	assert.Equal(t, "key", sel.key)
	assert.Equal(t, labelOpNotEqual, sel.op)
	assert.Equal(t, []string{"value"}, sel.values)
}

func TestParseLabelSelector_In(t *testing.T) {
	sel, err := parseLabelSelector("env in (prod,staging)")
	require.NoError(t, err)
	assert.Equal(t, "env", sel.key)
	assert.Equal(t, labelOpIn, sel.op)
	assert.Equal(t, []string{"prod", "staging"}, sel.values)
}

func TestSplitSelectors(t *testing.T) {
	parts := splitSelectors("a=b,c in (d,e),f!=g")
	assert.Len(t, parts, 3)
	assert.Equal(t, "a=b", parts[0])
	assert.Equal(t, "c in (d,e)", parts[1])
	assert.Equal(t, "f!=g", parts[2])
}

func TestNewResult(t *testing.T) {
	r := NewResult()
	assert.NotNil(t, r.SchemaAdditions)
	assert.Empty(t, r.Included)
	assert.Empty(t, r.Excluded)
	assert.Empty(t, r.Externalized)
}

func TestChain_ContextCancellation(t *testing.T) {
	resources := []*k8s.Resource{
		makeResource("Deployment", "app"),
		makeResource("Service", "svc"),
	}

	chain := NewChain(
		NewKindFilter([]string{"Service"}),
		NewKindFilter([]string{"Deployment"}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := chain.Apply(ctx, resources)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestChain_SchemaAdditionsAccumulated(t *testing.T) {
	secret := makeResource("Secret", "db-creds")
	svc := makeResource("Service", "redis")
	deploy := makeResource("Deployment", "app")

	chain := NewChain(
		NewExternalRefFilter([]ExternalMapping{
			{ResourceKind: "Secret", ResourceName: "db-creds", SchemaField: "externalDb.secretName"},
		}),
		NewExternalRefFilter([]ExternalMapping{
			{ResourceKind: "Service", ResourceName: "redis", SchemaField: "externalRedis.serviceName"},
		}),
	)

	result, err := chain.Apply(context.Background(), []*k8s.Resource{secret, svc, deploy})
	require.NoError(t, err)

	assert.Len(t, result.Included, 1)
	assert.Len(t, result.Externalized, 2)
	assert.Equal(t, "string", result.SchemaAdditions["externalDb.secretName"])
	assert.Equal(t, "string", result.SchemaAdditions["externalRedis.serviceName"])
}

func TestChain_Composability_ProfileAndManualFilters(t *testing.T) {
	resources := []*k8s.Resource{
		makeResourceWithSource("StatefulSet", "pg-sts", "my-chart/charts/postgresql/templates/statefulset.yaml"),
		makeResourceWithSource("Service", "pg-svc", "my-chart/charts/postgresql/templates/service.yaml"),
		makeResourceWithSource("Deployment", "app", "my-chart/templates/deployment.yaml"),
		makeResourceWithSource("ConfigMap", "app-config", "my-chart/templates/configmap.yaml"),
	}

	// Profile + manual kind filter
	chain := NewChain(
		NewSubchartFilter([]string{"postgresql"}),
		NewKindFilter([]string{"ConfigMap"}),
	)

	result, err := chain.Apply(context.Background(), resources)
	require.NoError(t, err)

	assert.Len(t, result.Included, 1)
	assert.Equal(t, "Deployment", result.Included[0].Kind())
	assert.Len(t, result.Excluded, 3)
}
