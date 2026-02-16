package filter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

func makeResourceWithObject(kind, name string, obj map[string]interface{}) *k8s.Resource {
	return &k8s.Resource{
		GVK:    schema.GroupVersionKind{Kind: kind, Version: "v1"},
		Name:   name,
		Labels: map[string]string{},
		Object: &unstructured.Unstructured{Object: obj},
	}
}

// ---------------------------------------------------------------------------
// ParseExternalMapping tests
// ---------------------------------------------------------------------------

func TestParseExternalMapping_Valid(t *testing.T) {
	m, err := ParseExternalMapping("Secret", "postgresql=externalDatabase.secretName")
	require.NoError(t, err)
	assert.Equal(t, "postgresql", m.ResourceName)
	assert.Equal(t, "Secret", m.ResourceKind)
	assert.Equal(t, "externalDatabase.secretName", m.SchemaField)
}

func TestParseExternalMapping_InvalidFormat(t *testing.T) {
	_, err := ParseExternalMapping("Secret", "no-equals-sign")
	assert.Error(t, err)
}

func TestParseExternalMapping_EmptyName(t *testing.T) {
	_, err := ParseExternalMapping("Secret", "=field")
	assert.Error(t, err)
}

func TestParseExternalMapping_EmptyField(t *testing.T) {
	_, err := ParseExternalMapping("Secret", "name=")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// ExternalRefFilter tests
// ---------------------------------------------------------------------------

func TestExternalRefFilter_ExternalizesSecret(t *testing.T) {
	secret := makeResourceWithObject("Secret", "my-db-secret", map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": "my-db-secret"},
		"data":       map[string]interface{}{"password": "c2VjcmV0"},
	})

	deploy := makeResourceWithObject("Deployment", "app", map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "app"},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name": "app",
							"envFrom": []interface{}{
								map[string]interface{}{
									"secretRef": map[string]interface{}{
										"name": "my-db-secret",
									},
								},
							},
						},
					},
				},
			},
		},
	})

	mappings := []ExternalMapping{
		{ResourceName: "my-db-secret", ResourceKind: "Secret", SchemaField: "externalDatabase.secretName"},
	}

	f := NewExternalRefFilter(mappings)
	result, err := f.Apply(context.Background(), []*k8s.Resource{secret, deploy})
	require.NoError(t, err)

	// Secret should be externalized, not included.
	assert.Len(t, result.Included, 1)
	assert.Equal(t, "app", result.Included[0].Name)
	require.Len(t, result.Externalized, 1)

	ext := result.Externalized[0]
	assert.Equal(t, "my-db-secret", ext.Resource.Name)

	// Check externalRef template.
	assert.Equal(t, "v1", ext.ExternalRef["apiVersion"])
	assert.Equal(t, "Secret", ext.ExternalRef["kind"])

	meta, _ := ext.ExternalRef["metadata"].(map[string]interface{})
	assert.Equal(t, "${schema.spec.externalDatabase.secretName}", meta["name"])

	// Check schema additions.
	assert.Equal(t, "string", result.SchemaAdditions["externalDatabase.secretName"])

	// Check rewiring in Deployment.
	containers, _ := result.Included[0].Object.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	envFrom := container["envFrom"].([]interface{})
	secretRef := envFrom[0].(map[string]interface{})["secretRef"].(map[string]interface{})
	assert.Equal(t, "${schema.spec.externalDatabase.secretName}", secretRef["name"])
}

func TestExternalRefFilter_ExternalizesService(t *testing.T) {
	svc := makeResourceWithObject("Service", "postgresql", map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]interface{}{"name": "postgresql"},
	})

	deploy := makeResourceWithObject("Deployment", "app", map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "app"},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name": "app",
							"env": []interface{}{
								map[string]interface{}{
									"name":  "DB_HOST",
									"value": "postgresql",
								},
							},
						},
					},
				},
			},
		},
	})

	mappings := []ExternalMapping{
		{ResourceName: "postgresql", ResourceKind: "Service", SchemaField: "externalDatabase.serviceName"},
	}

	f := NewExternalRefFilter(mappings)
	result, err := f.Apply(context.Background(), []*k8s.Resource{svc, deploy})
	require.NoError(t, err)

	assert.Len(t, result.Included, 1)
	require.Len(t, result.Externalized, 1)

	// Check rewiring in Deployment's env var.
	containers, _ := result.Included[0].Object.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	envVars := container["env"].([]interface{})
	env := envVars[0].(map[string]interface{})
	assert.Equal(t, "${schema.spec.externalDatabase.serviceName}", env["value"])
}

func TestExternalRefFilter_NoMatch(t *testing.T) {
	deploy := makeResource("Deployment", "app")

	mappings := []ExternalMapping{
		{ResourceName: "nonexistent", ResourceKind: "Secret", SchemaField: "ext.secretName"},
	}

	f := NewExternalRefFilter(mappings)
	result, err := f.Apply(context.Background(), []*k8s.Resource{deploy})
	require.NoError(t, err)

	assert.Len(t, result.Included, 1)
	assert.Empty(t, result.Externalized)
}

func TestExternalRefFilter_MultipleExternalizations(t *testing.T) {
	secret := makeResourceWithObject("Secret", "db-secret", map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": "db-secret"},
	})

	svc := makeResourceWithObject("Service", "db-svc", map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]interface{}{"name": "db-svc"},
	})

	deploy := makeResource("Deployment", "app")

	mappings := []ExternalMapping{
		{ResourceName: "db-secret", ResourceKind: "Secret", SchemaField: "ext.secretName"},
		{ResourceName: "db-svc", ResourceKind: "Service", SchemaField: "ext.serviceName"},
	}

	f := NewExternalRefFilter(mappings)
	result, err := f.Apply(context.Background(), []*k8s.Resource{secret, svc, deploy})
	require.NoError(t, err)

	assert.Len(t, result.Included, 1)
	assert.Len(t, result.Externalized, 2)
	assert.Len(t, result.SchemaAdditions, 2)
}

func TestExternalRefFilter_CaseInsensitiveKind(t *testing.T) {
	secret := makeResourceWithObject("Secret", "my-secret", map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": "my-secret"},
	})

	mappings := []ExternalMapping{
		{ResourceName: "my-secret", ResourceKind: "secret", SchemaField: "ext.name"},
	}

	f := NewExternalRefFilter(mappings)
	result, err := f.Apply(context.Background(), []*k8s.Resource{secret})
	require.NoError(t, err)

	assert.Empty(t, result.Included)
	assert.Len(t, result.Externalized, 1)
}

// ---------------------------------------------------------------------------
// Rewiring tests
// ---------------------------------------------------------------------------

func TestRewireResources_SubstringMatch(t *testing.T) {
	deploy := makeResourceWithObject("Deployment", "app", map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "app"},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name": "app",
							"env": []interface{}{
								map[string]interface{}{
									"name":  "DB_URL",
									"value": "postgresql.default.svc.cluster.local",
								},
							},
						},
					},
				},
			},
		},
	})

	rewirings := map[string]string{
		"postgresql": "${schema.spec.externalDatabase.host}",
	}

	rewireResources([]*k8s.Resource{deploy}, []ExternalizedResource{
		{Rewirings: rewirings},
	})

	containers, _ := deploy.Object.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	env := container["env"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "${schema.spec.externalDatabase.host}.default.svc.cluster.local", env["value"])
}

func TestRewireResources_NoFalsePositiveOnHyphenatedNames(t *testing.T) {
	// A resource named "redis" should NOT rewrite "redis-commander" or "my-redis-proxy"
	// because hyphens are NOT segment delimiters.
	deploy := makeResourceWithObject("Deployment", "app", map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "app"},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "redis-commander:latest",
							"env": []interface{}{
								map[string]interface{}{
									"name":  "PROXY",
									"value": "my-redis-proxy",
								},
							},
						},
					},
				},
			},
		},
	})

	rewirings := map[string]string{
		"redis": "${schema.spec.externalCache.name}",
	}

	rewireResources([]*k8s.Resource{deploy}, []ExternalizedResource{
		{Rewirings: rewirings},
	})

	containers, _ := deploy.Object.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]interface{})
	container := containers[0].(map[string]interface{})

	// Image should NOT be rewritten — "redis" is part of "redis-commander".
	assert.Equal(t, "redis-commander:latest", container["image"])

	// Env var should NOT be rewritten — "redis" is a prefix of "redis-proxy".
	env := container["env"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "my-redis-proxy", env["value"])
}

// ---------------------------------------------------------------------------
// replaceAtSegmentBoundaries unit tests
// ---------------------------------------------------------------------------

func TestReplaceAtSegmentBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		old      string
		repl     string
		expected string
	}{
		{
			name:     "exact match",
			input:    "postgresql",
			old:      "postgresql",
			repl:     "${ref}",
			expected: "${ref}",
		},
		{
			name:     "DNS name with dot separator",
			input:    "postgresql.default.svc.cluster.local",
			old:      "postgresql",
			repl:     "${ref}",
			expected: "${ref}.default.svc.cluster.local",
		},
		{
			name:     "no match — hyphenated prefix",
			input:    "redis-commander",
			old:      "redis",
			repl:     "${ref}",
			expected: "redis-commander",
		},
		{
			name:     "no match — embedded in word",
			input:    "myapp-frontend",
			old:      "app",
			repl:     "${ref}",
			expected: "myapp-frontend",
		},
		{
			name:     "path separator slash",
			input:    "data/postgresql/config",
			old:      "postgresql",
			repl:     "${ref}",
			expected: "data/${ref}/config",
		},
		{
			name:     "colon separator",
			input:    "host:postgresql:5432",
			old:      "postgresql",
			repl:     "${ref}",
			expected: "host:${ref}:5432",
		},
		{
			name:     "at sign separator",
			input:    "user@postgresql",
			old:      "postgresql",
			repl:     "${ref}",
			expected: "user@${ref}",
		},
		{
			name:     "no match at all",
			input:    "totally-unrelated",
			old:      "postgresql",
			repl:     "${ref}",
			expected: "totally-unrelated",
		},
		{
			name:     "multiple segment matches",
			input:    "postgresql.postgresql.local",
			old:      "postgresql",
			repl:     "${ref}",
			expected: "${ref}.${ref}.local",
		},
		{
			name:     "match at end after delimiter",
			input:    "cluster.postgresql",
			old:      "postgresql",
			repl:     "${ref}",
			expected: "cluster.${ref}",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := replaceAtSegmentBoundaries(tc.input, tc.old, tc.repl)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestRewireResources_NilObject(_ *testing.T) {
	res := makeResource("Deployment", "app") // no Object

	// Should not panic.
	rewireResources([]*k8s.Resource{res}, []ExternalizedResource{
		{Rewirings: map[string]string{"x": "y"}},
	})
}

// ---------------------------------------------------------------------------
// Chain integration with ExternalRef
// ---------------------------------------------------------------------------

func TestChain_WithExternalRefFilter(t *testing.T) {
	secret := makeResourceWithObject("Secret", "db-secret", map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": "db-secret"},
	})

	sts := makeResource("StatefulSet", "db")
	deploy := makeResource("Deployment", "app")

	chain := NewChain(
		NewKindFilter([]string{"StatefulSet"}),
		NewExternalRefFilter([]ExternalMapping{
			{ResourceName: "db-secret", ResourceKind: "Secret", SchemaField: "ext.secretName"},
		}),
	)

	result, err := chain.Apply(context.Background(), []*k8s.Resource{secret, sts, deploy})
	require.NoError(t, err)

	assert.Len(t, result.Included, 1)
	assert.Equal(t, "app", result.Included[0].Name)
	assert.Len(t, result.Excluded, 1)
	assert.Equal(t, "db", result.Excluded[0].Resource.Name)
	assert.Len(t, result.Externalized, 1)
	assert.Equal(t, "db-secret", result.Externalized[0].Resource.Name)
}
