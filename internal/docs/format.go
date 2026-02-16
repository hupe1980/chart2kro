package docs

import (
	"fmt"
	"html/template"
	"io"
	"strings"
	"text/tabwriter"
)

// Formatter renders a DocModel to a writer.
type Formatter interface {
	Format(w io.Writer, model *DocModel) error
}

// NewFormatter returns a formatter for the given format name.
func NewFormatter(format string) (Formatter, error) {
	switch strings.ToLower(format) {
	case "markdown", "md":
		return &MarkdownFormatter{}, nil
	case "html":
		return &HTMLFormatter{}, nil
	case "asciidoc", "adoc":
		return &AsciiDocFormatter{}, nil
	default:
		return nil, fmt.Errorf("unsupported docs format: %s", format)
	}
}

// ---------------------------------------------------------------------------
// Markdown
// ---------------------------------------------------------------------------

// MarkdownFormatter renders documentation as Markdown.
type MarkdownFormatter struct{}

func (f *MarkdownFormatter) Format(w io.Writer, model *DocModel) error {
	title := model.Title
	if title == "" {
		title = model.Kind + " API Reference"
	}

	fmt.Fprintf(w, "# %s\n\n", title)
	fmt.Fprintf(w, "**API Version:** `%s`  \n", model.APIVersion)
	fmt.Fprintf(w, "**Kind:** `%s`  \n", model.Kind)

	if model.Name != "" {
		fmt.Fprintf(w, "**RGD Name:** `%s`  \n", model.Name)
	}

	fmt.Fprintln(w)

	// Spec fields.
	if len(model.SpecFields) > 0 {
		fmt.Fprintf(w, "## Spec Fields\n\n")
		fmt.Fprintln(w, "| Field | Type | Default | Path |")
		fmt.Fprintln(w, "|-------|------|---------|------|")

		writeMarkdownFieldRows(w, model.SpecFields)

		fmt.Fprintln(w)
	}

	// Status fields.
	if len(model.StatusFields) > 0 {
		fmt.Fprintf(w, "## Status Fields\n\n")
		fmt.Fprintln(w, "| Field | Expression |")
		fmt.Fprintln(w, "|-------|------------|")

		for _, s := range model.StatusFields {
			fmt.Fprintf(w, "| `%s` | `%s` |\n", s.Name, s.Expression)
		}

		fmt.Fprintln(w)
	}

	// Resources.
	if len(model.Resources) > 0 {
		fmt.Fprintf(w, "## Resource Graph\n\n")

		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

		fmt.Fprintln(tw, "| ID\t| Kind\t| API Version\t| Depends On\t| Ready When\t|")
		fmt.Fprintln(tw, "|---\t|------\t|-------------\t|------------\t|------------\t|")

		for _, r := range model.Resources {
			deps := "-"
			if len(r.DependsOn) > 0 {
				deps = strings.Join(r.DependsOn, ", ")
			}

			ready := "-"
			if len(r.ReadyWhen) > 0 {
				ready = strings.Join(r.ReadyWhen, "; ")
			}

			fmt.Fprintf(tw, "| %s\t| %s\t| %s\t| %s\t| %s\t|\n",
				r.ID, r.Kind, r.APIVersion, deps, ready)
		}

		tw.Flush()

		fmt.Fprintln(w)
	}

	// Example YAML.
	if model.IncludeExamples {
		example := GenerateExampleYAML(model)
		fmt.Fprintf(w, "## Example\n\n```yaml\n%s```\n", example)
	}

	return nil
}

func writeMarkdownFieldRows(w io.Writer, fields []FieldInfo) {
	for _, f := range fields {
		def := f.Default
		if def == "" {
			def = "-"
		}

		fmt.Fprintf(w, "| `%s` | `%s` | %s | `%s` |\n", f.Name, f.Type, def, f.Path)

		if len(f.Children) > 0 {
			writeMarkdownFieldRows(w, f.Children)
		}
	}
}

// ---------------------------------------------------------------------------
// HTML
// ---------------------------------------------------------------------------

// HTMLFormatter renders documentation as a standalone HTML page.
type HTMLFormatter struct{}

var htmlTpl = template.Must(template.New("docs").Funcs(template.FuncMap{
	"join": strings.Join,
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.Title}}</title>
<style>
body{font-family:sans-serif;margin:2em;line-height:1.6}
table{border-collapse:collapse;width:100%;margin-bottom:1em}
th,td{border:1px solid #ddd;padding:8px;text-align:left}
th{background:#f5f5f5}
code{background:#f0f0f0;padding:2px 4px;border-radius:3px}
pre{background:#f5f5f5;padding:1em;border-radius:4px;overflow-x:auto}
</style>
</head>
<body>
<h1>{{.Title}}</h1>
<p><strong>API Version:</strong> <code>{{.APIVersion}}</code></p>
<p><strong>Kind:</strong> <code>{{.Kind}}</code></p>
{{if .Name}}<p><strong>RGD Name:</strong> <code>{{.Name}}</code></p>{{end}}

{{if .SpecFields}}
<h2>Spec Fields</h2>
<table>
<tr><th>Field</th><th>Type</th><th>Default</th><th>Path</th></tr>
{{range .FlatSpecFields}}<tr><td><code>{{.Name}}</code></td><td><code>{{.Type}}</code></td><td>{{if .Default}}{{.Default}}{{else}}-{{end}}</td><td><code>{{.Path}}</code></td></tr>
{{end}}
</table>
{{end}}

{{if .StatusFields}}
<h2>Status Fields</h2>
<table>
<tr><th>Field</th><th>Expression</th></tr>
{{range .StatusFields}}<tr><td><code>{{.Name}}</code></td><td><code>{{.Expression}}</code></td></tr>
{{end}}
</table>
{{end}}

{{if .Resources}}
<h2>Resource Graph</h2>
<table>
<tr><th>ID</th><th>Kind</th><th>API Version</th><th>Depends On</th><th>Ready When</th></tr>
{{range .Resources}}<tr><td>{{.ID}}</td><td>{{.Kind}}</td><td>{{.APIVersion}}</td><td>{{if .DependsOn}}{{join .DependsOn ", "}}{{else}}-{{end}}</td><td>{{if .ReadyWhen}}{{join .ReadyWhen "; "}}{{else}}-{{end}}</td></tr>
{{end}}
</table>
{{end}}

{{if .ExampleYAML}}
<h2>Example</h2>
<pre><code>{{.ExampleYAML}}</code></pre>
{{end}}

</body>
</html>
`))

// htmlModel wraps DocModel with helper methods for the HTML template.
type htmlModel struct {
	*DocModel
	FlatSpecFields []FieldInfo
	ExampleYAML    string
}

func (f *HTMLFormatter) Format(w io.Writer, model *DocModel) error {
	title := model.Title
	if title == "" {
		title = model.Kind + " API Reference"
	}

	m := htmlModel{
		DocModel:       model,
		FlatSpecFields: flattenFields(model.SpecFields),
	}
	m.Title = title

	if model.IncludeExamples {
		m.ExampleYAML = GenerateExampleYAML(model)
	}

	return htmlTpl.Execute(w, m)
}

func flattenFields(fields []FieldInfo) []FieldInfo {
	var flat []FieldInfo

	for _, f := range fields {
		flat = append(flat, f)

		if len(f.Children) > 0 {
			flat = append(flat, flattenFields(f.Children)...)
		}
	}

	return flat
}

// ---------------------------------------------------------------------------
// AsciiDoc
// ---------------------------------------------------------------------------

// AsciiDocFormatter renders documentation as AsciiDoc.
type AsciiDocFormatter struct{}

func (f *AsciiDocFormatter) Format(w io.Writer, model *DocModel) error {
	title := model.Title
	if title == "" {
		title = model.Kind + " API Reference"
	}

	fmt.Fprintf(w, "= %s\n\n", title)
	fmt.Fprintf(w, "*API Version:* `%s` +\n", model.APIVersion)
	fmt.Fprintf(w, "*Kind:* `%s` +\n", model.Kind)

	if model.Name != "" {
		fmt.Fprintf(w, "*RGD Name:* `%s` +\n", model.Name)
	}

	fmt.Fprintln(w)

	// Spec fields.
	if len(model.SpecFields) > 0 {
		fmt.Fprintf(w, "== Spec Fields\n\n")
		fmt.Fprintln(w, "[cols=\"1,1,1,2\", options=\"header\"]")
		fmt.Fprintln(w, "|===")
		fmt.Fprintln(w, "| Field | Type | Default | Path")

		writeAsciiDocFieldRows(w, model.SpecFields)

		fmt.Fprintln(w, "|===")
		fmt.Fprintln(w)
	}

	// Status fields.
	if len(model.StatusFields) > 0 {
		fmt.Fprintf(w, "== Status Fields\n\n")
		fmt.Fprintln(w, "[cols=\"1,2\", options=\"header\"]")
		fmt.Fprintln(w, "|===")
		fmt.Fprintln(w, "| Field | Expression")

		for _, s := range model.StatusFields {
			fmt.Fprintf(w, "\n| `%s`\n| `%s`\n", s.Name, s.Expression)
		}

		fmt.Fprintln(w, "|===")
		fmt.Fprintln(w)
	}

	// Resources.
	if len(model.Resources) > 0 {
		fmt.Fprintf(w, "== Resource Graph\n\n")
		fmt.Fprintln(w, "[cols=\"1,1,1,1,2\", options=\"header\"]")
		fmt.Fprintln(w, "|===")
		fmt.Fprintln(w, "| ID | Kind | API Version | Depends On | Ready When")

		for _, r := range model.Resources {
			deps := "-"
			if len(r.DependsOn) > 0 {
				deps = strings.Join(r.DependsOn, ", ")
			}

			ready := "-"
			if len(r.ReadyWhen) > 0 {
				ready = strings.Join(r.ReadyWhen, "; ")
			}

			fmt.Fprintf(w, "\n| %s\n| %s\n| %s\n| %s\n| %s\n", r.ID, r.Kind, r.APIVersion, deps, ready)
		}

		fmt.Fprintln(w, "|===")
		fmt.Fprintln(w)
	}

	// Example YAML.
	if model.IncludeExamples {
		example := GenerateExampleYAML(model)
		fmt.Fprintf(w, "== Example\n\n[source,yaml]\n----\n%s----\n", example)
	}

	return nil
}

func writeAsciiDocFieldRows(w io.Writer, fields []FieldInfo) {
	for _, f := range fields {
		def := f.Default
		if def == "" {
			def = "-"
		}

		fmt.Fprintf(w, "\n| `%s`\n| `%s`\n| %s\n| `%s`\n", f.Name, f.Type, def, f.Path)

		if len(f.Children) > 0 {
			writeAsciiDocFieldRows(w, f.Children)
		}
	}
}
