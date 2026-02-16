package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/hupe1980/chart2kro/internal/filter"
	"github.com/hupe1980/chart2kro/internal/helm/chartmeta"
	"github.com/hupe1980/chart2kro/internal/helm/deps"
	"github.com/hupe1980/chart2kro/internal/helm/hooks"
	"github.com/hupe1980/chart2kro/internal/helm/loader"
	"github.com/hupe1980/chart2kro/internal/helm/renderer"
	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/k8s/parser"
	"github.com/hupe1980/chart2kro/internal/logging"
	"github.com/hupe1980/chart2kro/internal/transform"
)

type inspectOptions struct {
	// Chart loading.
	repoURL  string
	version  string
	username string
	password string

	// Rendering.
	releaseName string
	namespace   string
	timeout     time.Duration

	// Values.
	valueFiles   []string
	values       []string
	stringValues []string

	// Output control.
	showResources bool
	showValues    bool
	showDeps      bool
	showSchema    bool
	format        string
}

func newInspectCommand() *cobra.Command {
	opts := &inspectOptions{}

	cmd := &cobra.Command{
		Use:   "inspect <chart-reference>",
		Short: "Inspect a Helm chart without converting",
		Long: `Inspect a Helm chart to preview what resources would be generated,
which values would be exposed as API fields, and what transformations
would be applied — without performing an actual conversion.

Displays chart metadata, resource table, schema preview, dependency graph,
and excludable infrastructure with suggested flags.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInspect(cmd.Context(), cmd, args[0], opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.repoURL, "repo-url", "", "Helm repository URL")
	f.StringVar(&opts.version, "version", "", "chart version constraint")
	f.StringVar(&opts.username, "username", "", "repository/registry username")
	f.StringVar(&opts.password, "password", "", "repository/registry password")

	f.StringVar(&opts.releaseName, "release-name", "release", "Helm release name for rendering")
	f.StringVar(&opts.namespace, "namespace", "default", "Kubernetes namespace for rendering")
	f.DurationVar(&opts.timeout, "timeout", 30*time.Second, "template rendering timeout")

	f.StringArrayVarP(&opts.valueFiles, "values", "f", nil, "values YAML files")
	f.StringArrayVar(&opts.values, "set", nil, "set values (key=value)")
	f.StringArrayVar(&opts.stringValues, "set-string", nil, "set string values")

	f.BoolVar(&opts.showResources, "show-resources", false, "show only resource table")
	f.BoolVar(&opts.showValues, "show-values", false, "show only values/schema")
	f.BoolVar(&opts.showDeps, "show-deps", false, "show only dependency graph")
	f.BoolVar(&opts.showSchema, "show-schema", false, "show only generated schema")
	f.StringVar(&opts.format, "format", "table", "output format: table, json, yaml")

	return cmd
}

// inspectResult is the structured output of the inspect command.
type inspectResult struct {
	Chart            chartInfo      `json:"chart"`
	Resources        []resourceInfo `json:"resources"`
	SchemaFields     []schemaInfo   `json:"schemaFields,omitempty"`
	Dependencies     []string       `json:"dependencies,omitempty"`
	Subcharts        []subchartInfo `json:"subcharts,omitempty"`
	ExternalPatterns []patternInfo  `json:"externalPatterns,omitempty"`
}

type chartInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	AppVersion  string `json:"appVersion,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

type resourceInfo struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Subchart string `json:"subchart,omitempty"`
}

type schemaInfo struct {
	Field   string `json:"field"`
	Type    string `json:"type"`
	Default string `json:"default,omitempty"`
}

type subchartInfo struct {
	Name           string `json:"name"`
	ResourceCount  int    `json:"resourceCount"`
	SuggestedFlags string `json:"suggestedFlags"`
}

type patternInfo struct {
	SubchartName      string   `json:"subchartName"`
	Condition         string   `json:"condition"`
	ExternalValuesKey string   `json:"externalValuesKey"`
	DetectedFields    []string `json:"detectedFields"`
}

func runInspect(ctx context.Context, cmd *cobra.Command, ref string, opts *inspectOptions) error {
	logger := logging.FromContext(ctx)

	// 1. Load chart.
	logger.Info("loading chart", slog.String("ref", ref))

	multiLoader := loader.NewMultiLoader()
	ch, err := multiLoader.Load(ctx, ref, loader.LoadOptions{
		Version:  opts.version,
		RepoURL:  opts.repoURL,
		Username: opts.username,
		Password: opts.password,
	})
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("loading chart: %w", err)}
	}

	meta := chartmeta.FromChart(ch)

	if meta.IsLibrary() {
		return &ExitError{Code: 1, Err: fmt.Errorf("chart %q is a library chart", meta.Name)}
	}

	// 2. Merge values and render.
	valOpts := renderer.ValuesOptions{
		ValueFiles:   opts.valueFiles,
		Values:       opts.values,
		StringValues: opts.stringValues,
	}

	mergedVals, err := renderer.MergeValues(ch, valOpts)
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("merging values: %w", err)}
	}

	renderCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	helmRenderer := renderer.New(renderer.RenderOptions{
		ReleaseName: opts.releaseName,
		Namespace:   opts.namespace,
	})

	// Use RenderWithSources to get source path info.
	sourced, err := helmRenderer.RenderWithSources(renderCtx, ch, mergedVals)
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("rendering templates: %w", err)}
	}

	// Combine for parsing.
	combinedYAML := renderer.CombineSourcedManifests(sourced)

	// 3. Filter hooks and parse.
	hookResult, err := hooks.Filter(combinedYAML, false, logger)
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("filtering hooks: %w", err)}
	}

	combined := hooks.CombineResources(hookResult)

	k8sParser := parser.NewParser()

	resources, err := k8sParser.Parse(ctx, combined)
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("parsing resources: %w", err)}
	}

	// Assign source paths to resources.
	assignResourceSourcePaths(resources, sourced)

	// 4. Assign IDs and analyze.
	resourceIDs, err := transform.AssignResourceIDs(resources, nil)
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("assigning IDs: %w", err)}
	}

	// 5. Detect dependencies.
	if meta.HasDependencies() {
		deps.Analyze(ch, logger)
	}

	// 6. Build result.
	result := buildInspectResult(meta, resources, resourceIDs, mergedVals)

	// 7. Render output.
	w := cmd.OutOrStdout()
	showAll := !opts.showResources && !opts.showValues && !opts.showDeps && !opts.showSchema

	switch opts.format {
	case "json":
		return renderJSON(w, result)
	case "yaml":
		return renderYAML(w, result)
	case "table":
		return renderTable(w, result, showAll, opts)
	default:
		return &ExitError{Code: 2, Err: fmt.Errorf("unknown format %q: expected table, json, yaml", opts.format)}
	}
}

func buildInspectResult(
	meta *chartmeta.ChartMeta,
	resources []*k8s.Resource,
	resourceIDs map[*k8s.Resource]string,
	values map[string]interface{},
) inspectResult {
	result := inspectResult{
		Chart: chartInfo{
			Name:        meta.Name,
			Version:     meta.Version,
			AppVersion:  meta.AppVersion,
			Description: meta.Description,
			Type:        meta.Type,
		},
	}

	// Build resource table.
	subchartResources := make(map[string]int)

	for _, r := range resources {
		id := resourceIDs[r]
		sc := r.SourceChart()

		result.Resources = append(result.Resources, resourceInfo{
			ID:       id,
			Kind:     r.Kind(),
			Name:     r.Name,
			Subchart: sc,
		})

		if sc != "" {
			subchartResources[sc]++
		}
	}

	// Build subchart info.
	for name, count := range subchartResources {
		result.Subcharts = append(result.Subcharts, subchartInfo{
			Name:           name,
			ResourceCount:  count,
			SuggestedFlags: fmt.Sprintf("--exclude-subcharts %s", name),
		})
	}

	sort.Slice(result.Subcharts, func(i, j int) bool {
		return result.Subcharts[i].Name < result.Subcharts[j].Name
	})

	// Build dependency order.
	depGraph := transform.BuildDependencyGraph(resourceIDs)

	order, err := depGraph.TopologicalSort()
	if err == nil {
		result.Dependencies = order
	}

	// Detect external patterns.
	patterns := filter.DetectExternalPatterns(meta, values)
	for _, p := range patterns {
		result.ExternalPatterns = append(result.ExternalPatterns, patternInfo{
			SubchartName:      p.SubchartName,
			Condition:         p.Condition,
			ExternalValuesKey: p.ExternalValuesKey,
			DetectedFields:    p.DetectedFields,
		})
	}

	// Extract schema fields preview.
	extractor := transform.NewSchemaExtractor(false, false, nil)
	fields := extractor.Extract(values, nil)

	for _, f := range fields {
		result.SchemaFields = append(result.SchemaFields, schemaInfo{
			Field:   f.Path,
			Type:    f.Type,
			Default: fmt.Sprintf("%v", f.Default),
		})
	}

	return result
}

func renderJSON(w io.Writer, result inspectResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(result)
}

func renderYAML(w io.Writer, result inspectResult) error {
	data, err := sigsyaml.Marshal(result)
	if err != nil {
		return err
	}

	_, err = w.Write(data)

	return err
}

func renderTable(w io.Writer, result inspectResult, showAll bool, opts *inspectOptions) error {
	if showAll || opts.showResources {
		printChartInfo(w, result)
		printResourceTable(w, result)
	}

	if showAll || opts.showDeps {
		printDependencyOrder(w, result)
	}

	if showAll || opts.showSchema {
		printSchemaPreview(w, result)
	}

	if showAll || opts.showValues {
		printSubchartInfo(w, result)
		printExternalPatterns(w, result)
	}

	return nil
}

func printChartInfo(w io.Writer, result inspectResult) {
	_, _ = fmt.Fprintf(w, "\n=== Chart: %s ===\n", result.Chart.Name)
	_, _ = fmt.Fprintf(w, "Version:     %s\n", result.Chart.Version)

	if result.Chart.AppVersion != "" {
		_, _ = fmt.Fprintf(w, "App Version: %s\n", result.Chart.AppVersion)
	}

	if result.Chart.Description != "" {
		_, _ = fmt.Fprintf(w, "Description: %s\n", result.Chart.Description)
	}
}

func printResourceTable(w io.Writer, result inspectResult) {
	_, _ = fmt.Fprintf(w, "\n--- Resources (%d) ---\n", len(result.Resources))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKIND\tNAME\tSUBCHART")

	for _, r := range result.Resources {
		sc := r.Subchart
		if sc == "" {
			sc = "(root)"
		}

		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.Kind, r.Name, sc)
	}

	_ = tw.Flush()
}

func printDependencyOrder(w io.Writer, result inspectResult) {
	if len(result.Dependencies) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "\n--- Dependency Order ---\n")

	for i, id := range result.Dependencies {
		_, _ = fmt.Fprintf(w, "  %d. %s\n", i+1, id)
	}
}

func printSchemaPreview(w io.Writer, result inspectResult) {
	if len(result.SchemaFields) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "\n--- Schema Fields (%d) ---\n", len(result.SchemaFields))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FIELD\tTYPE\tDEFAULT")

	for _, f := range result.SchemaFields {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", f.Field, f.Type, f.Default)
	}

	_ = tw.Flush()
}

func printSubchartInfo(w io.Writer, result inspectResult) {
	if len(result.Subcharts) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "\n--- Excludable Infrastructure ---\n")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SUBCHART\tRESOURCES\tSUGGESTED FLAGS")

	for _, s := range result.Subcharts {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\n", s.Name, s.ResourceCount, s.SuggestedFlags)
	}

	_ = tw.Flush()
}

func printExternalPatterns(w io.Writer, result inspectResult) {
	if len(result.ExternalPatterns) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "\n--- Detected External Patterns ---\n")

	for _, p := range result.ExternalPatterns {
		_, _ = fmt.Fprintf(w, "  %s: condition=%s, values=%s, fields=%s\n",
			p.SubchartName, p.Condition, p.ExternalValuesKey,
			strings.Join(p.DetectedFields, ","))
		_, _ = fmt.Fprintf(w, "    Suggested: --use-external-pattern %s\n", p.SubchartName)
	}
}
