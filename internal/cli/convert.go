package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/hupe1980/chart2kro/internal/config"
	"github.com/hupe1980/chart2kro/internal/filter"
	"github.com/hupe1980/chart2kro/internal/harden"
	"github.com/hupe1980/chart2kro/internal/helm/chartmeta"
	"github.com/hupe1980/chart2kro/internal/helm/hooks"
	"github.com/hupe1980/chart2kro/internal/helm/loader"
	"github.com/hupe1980/chart2kro/internal/helm/renderer"
	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/k8s/parser"
	"github.com/hupe1980/chart2kro/internal/logging"
	"github.com/hupe1980/chart2kro/internal/output"
	"github.com/hupe1980/chart2kro/internal/transform"
)

type convertOptions struct {
	// Chart loading.
	repoURL  string
	version  string
	username string
	password string
	caFile   string
	certFile string
	keyFile  string

	// Template rendering.
	releaseName string
	namespace   string
	strict      bool
	timeout     time.Duration

	// Values merging.
	valueFiles   []string
	values       []string
	stringValues []string
	fileValues   []string

	// Hook handling.
	includeHooks bool

	// Transformation.
	kind             string
	apiVersion       string
	group            string
	includeAllValues bool
	flatSchema       bool
	readyConditions  string
	fast             bool

	// Output.
	output         string
	dryRun         bool
	comments       bool
	split          bool
	outputDir      string
	embedTimestamp bool

	// Resource filtering.
	excludeKinds       []string
	excludeResources   []string
	excludeSubcharts   []string
	excludeLabels      string
	externalizeSecret  []string
	externalizeService []string
	useExternalPattern []string
	profile            string

	// Hardening.
	harden                  bool
	securityLevel           string
	generateNetworkPolicies bool
	generateRBAC            bool
	resolveDigests          bool
}

func newConvertCommand() *cobra.Command {
	opts := &convertOptions{}

	cmd := &cobra.Command{
		Use:   "convert <chart-reference>",
		Short: "Convert a Helm chart to a KRO ResourceGraphDefinition",
		Long: `Convert a Helm chart into a KRO ResourceGraphDefinition (RGD).

Supports loading charts from local directories, .tgz archives,
OCI registries, and Helm repositories.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConvert(cmd.Context(), cmd, args[0], opts)
		},
	}

	f := cmd.Flags()

	// Chart loading flags.
	f.StringVar(&opts.repoURL, "repo-url", "", "Helm repository URL")
	f.StringVar(&opts.version, "version", "", "chart version constraint")
	f.StringVar(&opts.username, "username", "", "repository/registry username")
	f.StringVar(&opts.password, "password", "", "repository/registry password")
	f.StringVar(&opts.caFile, "ca-file", "", "TLS CA certificate file")
	f.StringVar(&opts.certFile, "cert-file", "", "TLS client certificate file")
	f.StringVar(&opts.keyFile, "key-file", "", "TLS client key file")

	// Rendering flags.
	f.StringVar(&opts.releaseName, "release-name", "release", "Helm release name for rendering")
	f.StringVar(&opts.namespace, "namespace", "default", "Kubernetes namespace for rendering")
	f.BoolVar(&opts.strict, "strict", false, "fail on missing template values")
	f.DurationVar(&opts.timeout, "timeout", 30*time.Second, "template rendering timeout")

	// Values flags.
	f.StringArrayVarP(&opts.valueFiles, "values", "f", nil, "values YAML files (can specify multiple)")
	f.StringArrayVar(&opts.values, "set", nil, "set values (key=value, can specify multiple)")
	f.StringArrayVar(&opts.stringValues, "set-string", nil, "set string values (key=value)")
	f.StringArrayVar(&opts.fileValues, "set-file", nil, "set values from files (key=filepath)")

	// Hook handling flags.
	f.BoolVar(&opts.includeHooks, "include-hooks", false, "include hook resources (strip hook annotations)")

	// Transformation flags.
	f.StringVar(&opts.kind, "kind", "", "override generated CRD kind (default: PascalCase chart name)")
	f.StringVar(&opts.apiVersion, "api-version", "v1alpha1", "KRO schema apiVersion")
	f.StringVar(&opts.group, "group", "kro.run", "KRO schema group")
	f.BoolVar(&opts.includeAllValues, "include-all-values", false, "include all values in schema")
	f.BoolVar(&opts.flatSchema, "flat-schema", false, "use flat camelCase schema field names")
	f.StringVar(&opts.readyConditions, "ready-conditions", "", "path to a YAML file with custom readiness conditions per Kind")
	f.BoolVar(&opts.fast, "fast", false, "use template AST analysis instead of sentinel rendering (faster, may miss complex expressions)")

	// Output flags.
	f.StringVarP(&opts.output, "output", "o", "", "output file path (default: stdout)")
	f.BoolVar(&opts.dryRun, "dry-run", false, "preview output without writing files")
	f.BoolVar(&opts.comments, "comments", false, "add inline comments on CEL expressions")
	f.BoolVar(&opts.split, "split", false, "write one YAML file per resource (requires --output-dir)")
	f.StringVar(&opts.outputDir, "output-dir", "", "output directory for --split")
	f.BoolVar(&opts.embedTimestamp, "embed-timestamp", false, "add chart2kro.io/generated-at annotation")

	// Resource filtering flags.
	f.StringSliceVar(&opts.excludeKinds, "exclude-kinds", nil, "exclude resources by kind (comma-separated)")
	f.StringSliceVar(&opts.excludeResources, "exclude-resources", nil, "exclude resources by assigned ID (comma-separated)")
	f.StringSliceVar(&opts.excludeSubcharts, "exclude-subcharts", nil, "exclude all resources from named subcharts (comma-separated)")
	f.StringVar(&opts.excludeLabels, "exclude-labels", "", "exclude resources matching label selector (e.g., component=database)")
	f.StringArrayVar(&opts.externalizeSecret, "externalize-secret", nil, "externalize a Secret (name=schemaField)")
	f.StringArrayVar(&opts.externalizeService, "externalize-service", nil, "externalize a Service (name=schemaField)")
	f.StringSliceVar(&opts.useExternalPattern, "use-external-pattern", nil, "auto-detect and apply external pattern for subchart")
	f.StringVar(&opts.profile, "profile", "", "apply a conversion profile (enterprise, minimal, app-only, or custom)")

	// Hardening flags.
	f.BoolVar(&opts.harden, "harden", false, "enable security hardening pipeline")
	f.StringVar(&opts.securityLevel, "security-level", "restricted", "PSS enforcement level (none, baseline, restricted)")
	f.BoolVar(&opts.generateNetworkPolicies, "generate-network-policies", false, "generate NetworkPolicies from dependency graph")
	f.BoolVar(&opts.generateRBAC, "generate-rbac", false, "generate least-privilege ServiceAccount + Role + RoleBinding")
	f.BoolVar(&opts.resolveDigests, "resolve-digests", false, "resolve image tags to sha256 digests from container registries")

	return cmd
}

func runConvert(ctx context.Context, cmd *cobra.Command, ref string, opts *convertOptions) error {
	logger := logging.FromContext(ctx)

	// Detect source type (informational).
	if sourceType, err := loader.Detect(ref); err == nil {
		logger.Info("detected chart source type", slog.String("type", sourceType.String()))
	}

	// Run core pipeline (steps 1-10: load → render → detect → transform → generate RGD).
	res, err := runPipeline(ctx, ref, opts)
	if err != nil {
		return err
	}

	// Print interactive summaries (convert-only).
	if res.HookResult != nil {
		hooks.PrintHookSummary(cmd.ErrOrStderr(), res.HookResult)
	}

	if res.FilterResult != nil && (len(res.FilterResult.Excluded) > 0 || len(res.FilterResult.Externalized) > 0) {
		printFilterSummary(cmd.ErrOrStderr(), res.FilterResult)
	}

	logger.Info("transformation complete",
		slog.Int("resources", len(res.Result.Resources)),
		slog.Int("schema_fields", len(res.Result.SchemaFields)),
		slog.Int("status_fields", len(res.Result.StatusFields)),
		slog.Int("field_mappings", len(res.Result.FieldMappings)),
	)

	// 11. Finalize RGD map with convert-specific annotations.
	rgdMap := res.RGDMap

	if opts.embedTimestamp {
		metaMap, _ := rgdMap["metadata"].(map[string]interface{})
		if metaMap != nil {
			annotations, _ := metaMap["annotations"].(map[string]interface{})
			if annotations == nil {
				annotations = make(map[string]interface{})
			}

			annotations["chart2kro.io/generated-at"] = time.Now().UTC().Format(time.RFC3339)
			metaMap["annotations"] = annotations
		}
	}

	if opts.harden {
		provAnnotations, provErr := harden.GenerateProvenanceAnnotations(harden.ProvenanceConfig{
			ChartRef:          ref,
			Profile:           opts.profile,
			HardeningLevel:    harden.SecurityLevel(opts.securityLevel),
			ExcludedSubcharts: opts.excludeSubcharts,
			EmbedTimestamp:    opts.embedTimestamp,
		})
		if provErr != nil {
			return &ExitError{Code: 1, Err: fmt.Errorf("generating provenance annotations: %w", provErr)}
		}

		metaMap, _ := rgdMap["metadata"].(map[string]interface{})
		if metaMap != nil {
			annotations, _ := metaMap["annotations"].(map[string]interface{})
			if annotations == nil {
				annotations = make(map[string]interface{})
			}

			for k, v := range provAnnotations {
				annotations[k] = v
			}

			metaMap["annotations"] = annotations
		}
	}

	// 12. Serialize and output.
	if err := output.ValidateSplitFlags(opts.split, opts.outputDir); err != nil {
		return &ExitError{Code: 2, Err: err}
	}

	serOpts := output.SerializeOptions{
		Comments: opts.comments,
		Indent:   2,
	}

	if opts.split {
		splitResult, splitErr := output.Split(rgdMap, serOpts)
		if splitErr != nil {
			return &ExitError{Code: 1, Err: fmt.Errorf("splitting RGD: %w", splitErr)}
		}

		if err := output.WriteSplit(opts.outputDir, splitResult); err != nil {
			return &ExitError{Code: 6, Err: fmt.Errorf("writing split output: %w", err)}
		}

		logger.Info("split output written", slog.String("dir", opts.outputDir), slog.Int("files", len(splitResult.Files)))
		printConvertSummary(cmd.ErrOrStderr(), res.Result, res.HookResult, res.HardenResult)

		return nil
	}

	yamlBytes, err := output.Serialize(rgdMap, serOpts)
	if err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("serializing RGD: %w", err)}
	}

	if opts.dryRun {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "# Dry-run mode — output preview:")
	}

	if opts.output != "" && !opts.dryRun {
		w := output.NewFileWriter(opts.output, output.WithLogger(logger))
		if err := w.Write(yamlBytes); err != nil {
			return &ExitError{Code: 6, Err: fmt.Errorf("writing output: %w", err)}
		}

		logger.Info("RGD written", slog.String("path", opts.output))
	} else {
		if _, err := cmd.OutOrStdout().Write(yamlBytes); err != nil {
			return &ExitError{Code: 6, Err: fmt.Errorf("writing output: %w", err)}
		}
	}

	// 13. Print summary.
	printConvertSummary(cmd.ErrOrStderr(), res.Result, res.HookResult, res.HardenResult)

	return nil
}

// printConvertSummary prints a human-readable summary of the conversion.
func printConvertSummary(w io.Writer, result *transform.Result, hookResult *hooks.FilterResult, hardenResult *harden.HardenResult) {
	_, _ = fmt.Fprintf(w, "\n--- Conversion Summary ---\n")
	_, _ = fmt.Fprintf(w, "Resources:     %d\n", len(result.Resources))
	_, _ = fmt.Fprintf(w, "Schema fields: %d\n", len(result.SchemaFields))
	_, _ = fmt.Fprintf(w, "Status fields: %d\n", len(result.StatusFields))

	if len(hookResult.DroppedHooks) > 0 {
		_, _ = fmt.Fprintf(w, "Hooks dropped:  %d\n", len(hookResult.DroppedHooks))
	}

	if len(hookResult.IncludedHooks) > 0 {
		_, _ = fmt.Fprintf(w, "Hooks included: %d\n", len(hookResult.IncludedHooks))
	}

	if hardenResult != nil {
		_, _ = fmt.Fprintf(w, "Hardening:      %d changes, %d warnings\n",
			len(hardenResult.Changes), len(hardenResult.Warnings))
	}

	_, _ = fmt.Fprintf(w, "--------------------------\n")
}

// buildFilterChain assembles a filter chain from CLI flags and profiles.
// Returns nil if no filters are configured.
func buildFilterChain(
	ctx context.Context,
	opts *convertOptions,
	meta *chartmeta.ChartMeta,
	values map[string]interface{},
	resources []*k8s.Resource,
) (*filter.Chain, error) {
	var filters []filter.Filter

	// 1. Profile-based filters.
	if opts.profile != "" {
		if filter.IsMinimalProfile(opts.profile) {
			filters = append(filters, filter.NewMinimalFilter())
		} else {
			// Try loading custom profiles from config file.
			var customProfiles map[string]filter.ProfileConfig

			configData, err := tryReadConfigFile(ctx)
			if err == nil && configData != nil {
				customProfiles, _ = filter.ParseCustomProfiles(configData)
			}

			p, err := filter.ResolveProfile(opts.profile, customProfiles)
			if err != nil {
				return nil, err
			}

			profileFilters, err := filter.BuildFiltersFromProfile(p)
			if err != nil {
				return nil, fmt.Errorf("building filters from profile %q: %w", opts.profile, err)
			}

			filters = append(filters, profileFilters...)
		}
	}

	// 2. Smart external pattern detection.
	for _, name := range opts.useExternalPattern {
		pattern, found := filter.DetectExternalPatternsForSubchart(meta, values, name)
		if !found {
			return nil, fmt.Errorf("no external pattern detected for subchart %q", name)
		}

		patternFilters := filter.BuildFiltersFromPattern(pattern)
		filters = append(filters, patternFilters...)
	}

	// 3. Explicit subchart exclusion.
	if len(opts.excludeSubcharts) > 0 {
		filters = append(filters, filter.NewSubchartFilter(opts.excludeSubcharts))
	}

	// 4. Kind exclusion.
	if len(opts.excludeKinds) > 0 {
		filters = append(filters, filter.NewKindFilter(opts.excludeKinds))
	}

	// 5. Label exclusion.
	if opts.excludeLabels != "" {
		lf, err := filter.NewLabelFilter(opts.excludeLabels)
		if err != nil {
			return nil, fmt.Errorf("parsing label selector: %w", err)
		}

		filters = append(filters, lf)
	}

	// 6. Resource ID exclusion (needs IDs assigned first).
	if len(opts.excludeResources) > 0 {
		tempIDs, err := transform.AssignResourceIDs(resources, nil)
		if err != nil {
			return nil, fmt.Errorf("assigning IDs for resource exclusion: %w", err)
		}

		filters = append(filters, filter.NewResourceIDFilter(opts.excludeResources, tempIDs))
	}

	// 7. ExternalRef promotion.
	var mappings []filter.ExternalMapping

	for _, expr := range opts.externalizeSecret {
		m, err := filter.ParseExternalMapping("Secret", expr)
		if err != nil {
			return nil, err
		}

		mappings = append(mappings, m)
	}

	for _, expr := range opts.externalizeService {
		m, err := filter.ParseExternalMapping("Service", expr)
		if err != nil {
			return nil, err
		}

		mappings = append(mappings, m)
	}

	if len(mappings) > 0 {
		filters = append(filters, filter.NewExternalRefFilter(mappings))
	}

	if len(filters) == 0 {
		return nil, nil //nolint:nilnil // intentional: no filters configured
	}

	return filter.NewChain(filters...), nil
}

// assignResourceSourcePaths maps parsed resources to their source template paths.
// A single template file can produce multiple YAML documents, so we parse each
// sourced manifest's content and match resources by GVK + name.
func assignResourceSourcePaths(resources []*k8s.Resource, sourced []renderer.SourcedManifest) {
	// Build a lookup from "Kind/Name" → template path.
	pathLookup := make(map[string]string)

	for _, sm := range sourced {
		// Split each sourced manifest into individual YAML docs.
		docs := parser.SplitDocuments([]byte(sm.Content))
		for _, doc := range docs {
			var obj map[string]interface{}
			if err := sigsyaml.Unmarshal(doc, &obj); err != nil {
				continue
			}

			kind, _ := obj["kind"].(string)

			meta, _ := obj["metadata"].(map[string]interface{})
			name, _ := meta["name"].(string)

			if kind != "" && name != "" {
				pathLookup[kind+"/"+name] = sm.TemplatePath
			}
		}
	}

	// Assign source paths by matching Kind/Name.
	for _, res := range resources {
		key := res.Kind() + "/" + res.Name
		if path, ok := pathLookup[key]; ok {
			res.SourcePath = path
		}
	}
}

// printFilterSummary prints a summary of filtered resources to stderr.
func printFilterSummary(w io.Writer, result *filter.Result) {
	_, _ = fmt.Fprintf(w, "\n--- Filter Summary ---\n")

	for _, ex := range result.Excluded {
		_, _ = fmt.Fprintf(w, "  Excluded: %s (%s)\n", ex.Resource.QualifiedName(), ex.Reason)
	}

	for _, ext := range result.Externalized {
		_, _ = fmt.Fprintf(w, "  Externalized: %s\n", ext.Resource.QualifiedName())
	}

	_, _ = fmt.Fprintf(w, "  Excluded: %d, Externalized: %d, Remaining: %d\n",
		len(result.Excluded), len(result.Externalized), len(result.Included))
	_, _ = fmt.Fprintf(w, "----------------------\n")
}

// tryReadConfigFile attempts to read the config file. It first checks for a
// path resolved by the --config flag (stored in the Config struct); if not set,
// it falls back to .chart2kro.yaml in the current directory.
func tryReadConfigFile(ctx context.Context) ([]byte, error) {
	// Prefer the config file path resolved by viper (respects --config flag).
	cfg := config.FromContext(ctx)
	if cfg.ConfigFile != "" {
		data, err := os.ReadFile(cfg.ConfigFile)
		if err != nil {
			return nil, err
		}

		return data, nil
	}

	// Fallback: auto-discover .chart2kro.yaml in current directory.
	data, err := os.ReadFile(".chart2kro.yaml")
	if err != nil {
		return nil, err
	}

	return data, nil
}
