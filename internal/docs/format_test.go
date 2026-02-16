package docs_test

import (
	"bytes"
	"testing"

	"github.com/hupe1980/chart2kro/internal/docs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleDocModel() *docs.DocModel {
	return &docs.DocModel{
		Name:       "my-app",
		APIVersion: "myapp.kro.run/v1alpha1",
		Kind:       "MyApp",
		SpecFields: []docs.FieldInfo{
			{Name: "name", Path: "spec.name", Type: "string"},
			{Name: "replicas", Path: "spec.replicas", Type: "integer", Default: "3"},
			{
				Name: "image",
				Path: "spec.image",
				Type: "object",
				Children: []docs.FieldInfo{
					{Name: "repository", Path: "spec.image.repository", Type: "string", Default: `"nginx"`},
				},
			},
		},
		StatusFields: []docs.StatusInfo{
			{Name: "ready", Expression: "${deployment.status.availableReplicas}"},
		},
		Resources: []docs.ResourceInfo{
			{
				ID:         "deployment",
				Kind:       "Deployment",
				APIVersion: "apps/v1",
				ReadyWhen:  []string{"${deployment.status.readyReplicas == deployment.status.replicas}"},
			},
			{
				ID:         "service",
				Kind:       "Service",
				APIVersion: "v1",
				DependsOn:  []string{"deployment"},
			},
		},
	}
}

func TestNewFormatter(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{"markdown", "markdown", false},
		{"md", "md", false},
		{"html", "html", false},
		{"asciidoc", "asciidoc", false},
		{"adoc", "adoc", false},
		{"unknown", "pdf", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := docs.NewFormatter(tt.format)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, f)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, f)
			}
		})
	}
}

func TestMarkdownFormatter(t *testing.T) {
	f, _ := docs.NewFormatter("markdown")
	var buf bytes.Buffer
	err := f.Format(&buf, sampleDocModel())
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "# MyApp API Reference")
	assert.Contains(t, out, "**API Version:** `myapp.kro.run/v1alpha1`")
	assert.Contains(t, out, "**Kind:** `MyApp`")
	assert.Contains(t, out, "**RGD Name:** `my-app`")
	assert.Contains(t, out, "## Spec Fields")
	assert.Contains(t, out, "| `name` | `string` |")
	assert.Contains(t, out, "| `replicas` | `integer` | 3 |")
	assert.Contains(t, out, "| `repository` | `string` |")
	assert.Contains(t, out, "## Status Fields")
	assert.Contains(t, out, "| `ready` |")
	assert.Contains(t, out, "## Resource Graph")
	assert.Contains(t, out, "deployment")
	assert.Contains(t, out, "service")
}

func TestMarkdownFormatter_CustomTitle(t *testing.T) {
	model := sampleDocModel()
	model.Title = "Custom Title"

	f, _ := docs.NewFormatter("markdown")
	var buf bytes.Buffer
	err := f.Format(&buf, model)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "# Custom Title")
}

func TestMarkdownFormatter_Empty(t *testing.T) {
	model := &docs.DocModel{Kind: "Empty", APIVersion: "v1"}
	f, _ := docs.NewFormatter("markdown")
	var buf bytes.Buffer
	err := f.Format(&buf, model)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "# Empty API Reference")
	assert.NotContains(t, out, "## Spec Fields")
	assert.NotContains(t, out, "## Status Fields")
	assert.NotContains(t, out, "## Resource Graph")
}

func TestHTMLFormatter(t *testing.T) {
	f, _ := docs.NewFormatter("html")
	var buf bytes.Buffer
	err := f.Format(&buf, sampleDocModel())
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "<!DOCTYPE html>")
	assert.Contains(t, out, "<title>MyApp API Reference</title>")
	assert.Contains(t, out, "<code>myapp.kro.run/v1alpha1</code>")
	assert.Contains(t, out, "<h2>Spec Fields</h2>")
	assert.Contains(t, out, "<code>name</code>")
	assert.Contains(t, out, "<h2>Status Fields</h2>")
	assert.Contains(t, out, "<h2>Resource Graph</h2>")
	assert.Contains(t, out, "deployment")
}

func TestHTMLFormatter_NoName(t *testing.T) {
	model := &docs.DocModel{Kind: "X", APIVersion: "v1"}
	f, _ := docs.NewFormatter("html")
	var buf bytes.Buffer
	err := f.Format(&buf, model)
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "RGD Name")
}

func TestAsciiDocFormatter(t *testing.T) {
	f, _ := docs.NewFormatter("asciidoc")
	var buf bytes.Buffer
	err := f.Format(&buf, sampleDocModel())
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "= MyApp API Reference")
	assert.Contains(t, out, "*API Version:*")
	assert.Contains(t, out, "== Spec Fields")
	assert.Contains(t, out, "|===")
	assert.Contains(t, out, "| `name`")
	assert.Contains(t, out, "== Status Fields")
	assert.Contains(t, out, "== Resource Graph")
	assert.Contains(t, out, "deployment")
}

func TestAsciiDocFormatter_Empty(t *testing.T) {
	model := &docs.DocModel{Kind: "Empty", APIVersion: "v1"}
	f, _ := docs.NewFormatter("asciidoc")
	var buf bytes.Buffer
	err := f.Format(&buf, model)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "= Empty API Reference")
	assert.NotContains(t, out, "== Spec Fields")
}

func TestMarkdownFormatter_WithExamples(t *testing.T) {
	model := sampleDocModel()
	model.IncludeExamples = true

	f, _ := docs.NewFormatter("markdown")
	var buf bytes.Buffer
	require.NoError(t, f.Format(&buf, model))

	out := buf.String()
	assert.Contains(t, out, "## Example")
	assert.Contains(t, out, "```yaml")
	assert.Contains(t, out, "apiVersion: myapp.kro.run/v1alpha1")
	assert.Contains(t, out, "kind: MyApp")
}

func TestMarkdownFormatter_WithoutExamples(t *testing.T) {
	model := sampleDocModel()
	model.IncludeExamples = false

	f, _ := docs.NewFormatter("markdown")
	var buf bytes.Buffer
	require.NoError(t, f.Format(&buf, model))

	assert.NotContains(t, buf.String(), "## Example")
}

func TestHTMLFormatter_WithExamples(t *testing.T) {
	model := sampleDocModel()
	model.IncludeExamples = true

	f, _ := docs.NewFormatter("html")
	var buf bytes.Buffer
	require.NoError(t, f.Format(&buf, model))

	out := buf.String()
	assert.Contains(t, out, "<h2>Example</h2>")
	assert.Contains(t, out, "<pre><code>")
	assert.Contains(t, out, "apiVersion: myapp.kro.run/v1alpha1")
}

func TestAsciiDocFormatter_WithExamples(t *testing.T) {
	model := sampleDocModel()
	model.IncludeExamples = true

	f, _ := docs.NewFormatter("asciidoc")
	var buf bytes.Buffer
	require.NoError(t, f.Format(&buf, model))

	out := buf.String()
	assert.Contains(t, out, "== Example")
	assert.Contains(t, out, "[source,yaml]")
	assert.Contains(t, out, "----")
	assert.Contains(t, out, "apiVersion: myapp.kro.run/v1alpha1")
}
