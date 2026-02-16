package parser_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/k8s/parser"
)

func TestSplitDocuments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"single document", "apiVersion: v1\nkind: Service", 1},
		{"two documents", "apiVersion: v1\nkind: Service\n---\napiVersion: apps/v1\nkind: Deployment", 2},
		{"leading separator", "---\napiVersion: v1\nkind: Service", 1},
		{"trailing separator", "apiVersion: v1\nkind: Service\n---", 1},
		{"empty between separators", "---\n---\napiVersion: v1\nkind: Service", 1},
		{"whitespace only doc", "---\n   \n---\napiVersion: v1\nkind: Service", 1},
		{"no separator", "apiVersion: v1\nkind: ConfigMap", 1},
		{"empty input", "", 0},
		{"only separators", "---\n---\n---", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs := parser.SplitDocuments([]byte(tt.input))
			assert.Len(t, docs, tt.want)
		})
	}
}

func TestSplitDocuments_PreservesContent(t *testing.T) {
	input := "apiVersion: v1\nkind: Service\nmetadata:\n  name: foo\n---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: bar"
	docs := parser.SplitDocuments([]byte(input))
	require.Len(t, docs, 2)
	assert.Contains(t, string(docs[0]), "Service")
	assert.Contains(t, string(docs[1]), "Deployment")
}

func TestParser_SingleDeployment(t *testing.T) {
	manifest := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: default
  labels:
    app: nginx
  annotations:
    note: test
spec:
  replicas: 3
`)
	p := parser.NewParser()
	resources, err := p.Parse(context.Background(), manifest)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	r := resources[0]
	assert.Equal(t, "apps", r.GVK.Group)
	assert.Equal(t, "v1", r.GVK.Version)
	assert.Equal(t, "Deployment", r.GVK.Kind)
	assert.Equal(t, "nginx", r.Name)
	assert.Equal(t, "default", r.Namespace)
	assert.Equal(t, "nginx", r.Labels["app"])
	assert.Equal(t, "test", r.Annotations["note"])
	assert.NotNil(t, r.Object)
}

func TestParser_MultipleResources(t *testing.T) {
	manifest := []byte(`apiVersion: v1
kind: Service
metadata:
  name: svc
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deploy
`)
	p := parser.NewParser()
	resources, err := p.Parse(context.Background(), manifest)
	require.NoError(t, err)
	require.Len(t, resources, 3)
	assert.Equal(t, "Service", resources[0].Kind())
	assert.Equal(t, "ConfigMap", resources[1].Kind())
	assert.Equal(t, "Deployment", resources[2].Kind())
}

func TestParser_SkipsMissingKind(t *testing.T) {
	manifest := []byte(`apiVersion: v1
metadata:
  name: noKind
`)
	p := parser.NewParser()
	resources, err := p.Parse(context.Background(), manifest)
	require.NoError(t, err)
	assert.Empty(t, resources)
}

func TestParser_SkipsMissingAPIVersion(t *testing.T) {
	manifest := []byte(`kind: ConfigMap
metadata:
  name: noAPIVersion
`)
	p := parser.NewParser()
	resources, err := p.Parse(context.Background(), manifest)
	require.NoError(t, err)
	assert.Empty(t, resources)
}

func TestParser_SkipsEmptyDocuments(t *testing.T) {
	manifest := []byte("---\n\n---\napiVersion: v1\nkind: Service\nmetadata:\n  name: svc\n---\n")
	p := parser.NewParser()
	resources, err := p.Parse(context.Background(), manifest)
	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Equal(t, "Service", resources[0].Kind())
}

func TestParser_MalformedYAML(t *testing.T) {
	manifest := []byte("apiVersion: v1\nkind: Service\n  bad indent: [")
	p := parser.NewParser()
	_, err := p.Parse(context.Background(), manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing document")
}

func TestParser_CoreGroupVersion(t *testing.T) {
	manifest := []byte(`apiVersion: v1
kind: Service
metadata:
  name: core-svc
`)
	p := parser.NewParser()
	resources, err := p.Parse(context.Background(), manifest)
	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Equal(t, "", resources[0].GVK.Group)
	assert.Equal(t, "v1", resources[0].GVK.Version)
	assert.Equal(t, "v1", resources[0].APIVersion())
}

func TestParser_VariedResourceTypes(t *testing.T) {
	manifest := []byte(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: sa
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cr
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ing
`)
	p := parser.NewParser()
	resources, err := p.Parse(context.Background(), manifest)
	require.NoError(t, err)
	require.Len(t, resources, 3)
	assert.Equal(t, "ServiceAccount", resources[0].Kind())
	assert.Equal(t, "ClusterRole", resources[1].Kind())
	assert.Equal(t, "Ingress", resources[2].Kind())
}

func TestParser_QualifiedName(t *testing.T) {
	manifest := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
`)
	p := parser.NewParser()
	resources, err := p.Parse(context.Background(), manifest)
	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Equal(t, "Deployment/web", resources[0].QualifiedName())
}
