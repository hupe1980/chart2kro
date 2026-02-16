// Package chart2kro provides a public Go API for converting Helm charts into
// KRO ResourceGraphDefinition YAML.
//
// This package exposes the chart2kro conversion pipeline as a library,
// allowing programmatic use without the CLI.
//
// Basic usage:
//
//	result, err := chart2kro.Convert(ctx, "path/to/chart")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(string(result.YAML))
//
// With options:
//
//	result, err := chart2kro.Convert(ctx, "path/to/chart",
//	    chart2kro.WithReleaseName("my-release"),
//	    chart2kro.WithNamespace("production"),
//	    chart2kro.WithIncludeAllValues(),
//	)
package chart2kro

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	sigsyaml "sigs.k8s.io/yaml"

	"github.com/hupe1980/chart2kro/internal/config"
	"github.com/hupe1980/chart2kro/internal/filter"
	"github.com/hupe1980/chart2kro/internal/harden"
	"github.com/hupe1980/chart2kro/internal/helm/chartmeta"
	"github.com/hupe1980/chart2kro/internal/helm/deps"
	"github.com/hupe1980/chart2kro/internal/helm/hooks"
	"github.com/hupe1980/chart2kro/internal/helm/loader"
	"github.com/hupe1980/chart2kro/internal/helm/renderer"
	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/k8s/parser"
	"github.com/hupe1980/chart2kro/internal/kro"
	"github.com/hupe1980/chart2kro/internal/output"
	"github.com/hupe1980/chart2kro/internal/transform"
	"github.com/hupe1980/chart2kro/internal/transform/transformer"
)

// discardLogger returns a logger that discards all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Option configures the chart-to-RGD conversion pipeline.
// Use the With* functions to create Options.
type Option func(*options)

// options holds the internal configuration for the conversion pipeline.
type options struct {
	// Chart loading.
	version  string
	repoURL  string
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

	// Schema generation.
	kind             string
	apiVersion       string
	group            string
	includeAllValues bool
	flatSchema       bool

	// Filtering.
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

	// Ready conditions.
	readyConditions string

	resourceIDOverrides map[string]string
	schemaOverrides     map[string]SchemaOverride
	transformConfigData []byte
	fast                bool
}

// --- Chart loading ---

// WithVersion sets the chart version constraint.
func WithVersion(v string) Option { return func(o *options) { o.version = v } }

// WithRepoURL sets the Helm repository URL.
func WithRepoURL(url string) Option { return func(o *options) { o.repoURL = url } }

// WithUsername sets the repository/registry username.
func WithUsername(u string) Option { return func(o *options) { o.username = u } }

// WithPassword sets the repository/registry password.
func WithPassword(p string) Option { return func(o *options) { o.password = p } }

// WithCaFile sets the TLS CA certificate file.
func WithCaFile(f string) Option { return func(o *options) { o.caFile = f } }

// WithCertFile sets the TLS client certificate file.
func WithCertFile(f string) Option { return func(o *options) { o.certFile = f } }

// WithKeyFile sets the TLS client key file.
func WithKeyFile(f string) Option { return func(o *options) { o.keyFile = f } }

// --- Template rendering ---

// WithReleaseName sets the Helm release name (default: "release").
func WithReleaseName(name string) Option { return func(o *options) { o.releaseName = name } }

// WithNamespace sets the Kubernetes namespace (default: "default").
func WithNamespace(ns string) Option { return func(o *options) { o.namespace = ns } }

// WithStrict enables strict template rendering (fail on missing values).
func WithStrict() Option { return func(o *options) { o.strict = true } }

// WithTimeout sets the template rendering timeout (default: 30s).
func WithTimeout(d time.Duration) Option { return func(o *options) { o.timeout = d } }

// --- Values merging ---

// WithValueFiles sets paths to additional values files.
func WithValueFiles(files []string) Option { return func(o *options) { o.valueFiles = files } }

// WithValues sets individual value overrides (key=value).
func WithValues(vals []string) Option { return func(o *options) { o.values = vals } }

// WithStringValues sets individual string value overrides (key=value).
func WithStringValues(vals []string) Option { return func(o *options) { o.stringValues = vals } }

// WithFileValues sets individual file value overrides (key=filepath).
func WithFileValues(vals []string) Option { return func(o *options) { o.fileValues = vals } }

// --- Hook handling ---

// WithIncludeHooks includes Helm hook resources in output.
func WithIncludeHooks() Option { return func(o *options) { o.includeHooks = true } }

// --- Schema generation ---

// WithKind overrides the generated CRD kind.
func WithKind(kind string) Option { return func(o *options) { o.kind = kind } }

// WithAPIVersion overrides the schema apiVersion (default: "v1alpha1").
func WithAPIVersion(v string) Option { return func(o *options) { o.apiVersion = v } }

// WithGroup overrides the schema group (default: "kro.run").
func WithGroup(g string) Option { return func(o *options) { o.group = g } }

// WithIncludeAllValues includes all values in schema, even unreferenced.
func WithIncludeAllValues() Option { return func(o *options) { o.includeAllValues = true } }

// WithFlatSchema uses flat camelCase field names.
func WithFlatSchema() Option { return func(o *options) { o.flatSchema = true } }

// --- Filtering ---

// WithExcludeKinds excludes resources by Kind.
func WithExcludeKinds(kinds []string) Option { return func(o *options) { o.excludeKinds = kinds } }

// WithExcludeResources excludes resources by name pattern.
func WithExcludeResources(names []string) Option {
	return func(o *options) { o.excludeResources = names }
}

// WithExcludeSubcharts excludes subchart resources.
func WithExcludeSubcharts(subs []string) Option {
	return func(o *options) { o.excludeSubcharts = subs }
}

// WithExcludeLabels excludes resources by label selector.
func WithExcludeLabels(selector string) Option {
	return func(o *options) { o.excludeLabels = selector }
}

// WithExternalizeSecret externalizes Secrets by name.
func WithExternalizeSecret(names []string) Option {
	return func(o *options) { o.externalizeSecret = names }
}

// WithExternalizeService externalizes Services by name.
func WithExternalizeService(names []string) Option {
	return func(o *options) { o.externalizeService = names }
}

// WithUseExternalPattern sets regex patterns for external refs.
func WithUseExternalPattern(patterns []string) Option {
	return func(o *options) { o.useExternalPattern = patterns }
}

// WithProfile sets a predefined filter profile.
func WithProfile(p string) Option { return func(o *options) { o.profile = p } }

// --- Hardening ---

// WithHarden enables security hardening.
func WithHarden() Option { return func(o *options) { o.harden = true } }

// WithSecurityLevel sets the hardening security level ("restricted", "baseline", or "none").
func WithSecurityLevel(level string) Option { return func(o *options) { o.securityLevel = level } }

// WithGenerateNetworkPolicies generates NetworkPolicy resources.
func WithGenerateNetworkPolicies() Option {
	return func(o *options) { o.generateNetworkPolicies = true }
}

// WithGenerateRBAC generates RBAC resources.
func WithGenerateRBAC() Option { return func(o *options) { o.generateRBAC = true } }

// WithResolveDigests resolves image tags to digests.
func WithResolveDigests() Option { return func(o *options) { o.resolveDigests = true } }

// --- Ready conditions ---

// WithReadyConditions sets the path to custom ready conditions file.
func WithReadyConditions(path string) Option { return func(o *options) { o.readyConditions = path } }

// --- Overrides ---

// WithResourceIDOverrides overrides auto-assigned resource IDs.
// Keys are qualified resource names (e.g., "Deployment/my-deploy").
func WithResourceIDOverrides(overrides map[string]string) Option {
	return func(o *options) { o.resourceIDOverrides = overrides }
}

// WithSchemaOverrides overrides inferred schema field types and defaults.
// Keys are dotted Helm value paths (e.g., "replicaCount", "image.tag").
func WithSchemaOverrides(overrides map[string]SchemaOverride) Option {
	return func(o *options) { o.schemaOverrides = overrides }
}

// WithTransformConfigData sets raw YAML bytes of a .chart2kro.yaml config.
// When set, it is parsed for transformer overrides, schema overrides,
// and resource ID overrides.
func WithTransformConfigData(data []byte) Option {
	return func(o *options) { o.transformConfigData = data }
}

// WithFast uses template AST analysis instead of sentinel diffing for
// parameter detection. Faster but less accurate on complex charts.
func WithFast() Option { return func(o *options) { o.fast = true } }

// SchemaOverride overrides inferred schema field types and defaults.
type SchemaOverride struct {
	Type    string      // JSON Schema type: "string", "integer", "boolean", "number"
	Default interface{} // default value
}

// Result holds the output of a successful conversion.
type Result struct {
	// YAML is the rendered RGD YAML bytes.
	YAML []byte

	// RGDMap is the structured RGD as a map, suitable for further manipulation.
	RGDMap map[string]interface{}

	// ChartName is the name of the source chart.
	ChartName string

	// ChartVersion is the version of the source chart.
	ChartVersion string

	// ResourceCount is the number of Kubernetes resources in the RGD.
	ResourceCount int

	// SchemaFieldCount is the number of extracted schema parameters.
	SchemaFieldCount int

	// DependencyEdges is the number of dependency edges in the graph.
	DependencyEdges int

	// HardenResult holds hardening details when hardening was enabled.
	HardenResult *HardenSummary
}

// HardenSummary holds a summary of hardening changes applied.
type HardenSummary struct {
	Changes  int
	Warnings []string
}

// Convert transforms a Helm chart reference into a KRO ResourceGraphDefinition.
//
// The chartRef can be a local directory, .tgz archive path, OCI registry URL,
// or a chart name (with WithRepoURL set).
//
// Pass no options to use all defaults:
//
//	result, err := chart2kro.Convert(ctx, "path/to/chart")
func Convert(ctx context.Context, chartRef string, opts ...Option) (*Result, error) {
	if chartRef == "" {
		return nil, errors.New("chart reference must not be empty")
	}

	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	o.applyDefaults()

	logger := discardLogger()

	// 1. Load the chart.
	multiLoader := loader.NewMultiLoader()

	ch, err := multiLoader.Load(ctx, chartRef, loader.LoadOptions{
		Version:  o.version,
		RepoURL:  o.repoURL,
		Username: o.username,
		Password: o.password,
		CaFile:   o.caFile,
		CertFile: o.certFile,
		KeyFile:  o.keyFile,
	})
	if err != nil {
		return nil, fmt.Errorf("loading chart: %w", err)
	}

	// 2. Extract chart metadata.
	meta := chartmeta.FromChart(ch)
	if meta.IsLibrary() {
		return nil, fmt.Errorf("chart %q is a library chart and cannot be converted", meta.Name)
	}

	// 3. Analyze dependencies.
	if meta.HasDependencies() {
		deps.Analyze(ch, logger)
	}

	// 4. Merge values.
	mergedVals, err := renderer.MergeValues(ch, renderer.ValuesOptions{
		ValueFiles:   o.valueFiles,
		Values:       o.values,
		StringValues: o.stringValues,
		FileValues:   o.fileValues,
	})
	if err != nil {
		return nil, fmt.Errorf("merging values: %w", err)
	}

	// 5. Render templates.
	renderCtx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	helmRenderer := renderer.New(renderer.RenderOptions{
		ReleaseName: o.releaseName,
		Namespace:   o.namespace,
		Strict:      o.strict,
	})

	needsSources := len(o.excludeSubcharts) > 0 || o.profile != "" || len(o.useExternalPattern) > 0

	var rendered []byte

	var sourcedManifests []renderer.SourcedManifest

	if needsSources {
		sourcedManifests, err = helmRenderer.RenderWithSources(renderCtx, ch, mergedVals)
		if err != nil {
			return nil, fmt.Errorf("rendering templates: %w", err)
		}

		rendered = renderer.CombineSourcedManifests(sourcedManifests)
	} else {
		rendered, err = helmRenderer.Render(renderCtx, ch, mergedVals)
		if err != nil {
			return nil, fmt.Errorf("rendering templates: %w", err)
		}
	}

	// 6. Filter hooks.
	hookResult, err := hooks.Filter(rendered, o.includeHooks, logger)
	if err != nil {
		return nil, fmt.Errorf("filtering hooks: %w", err)
	}

	// 7. Parse rendered YAML into K8s resources.
	combined := hooks.CombineResources(hookResult)
	k8sParser := parser.NewParser()

	resources, err := k8sParser.Parse(ctx, combined)
	if err != nil {
		return nil, fmt.Errorf("parsing resources: %w", err)
	}

	if len(resources) == 0 {
		return nil, fmt.Errorf("no resources found in rendered output")
	}

	if len(sourcedManifests) > 0 {
		assignResourceSourcePaths(resources, sourcedManifests)
	}

	// 7b. Apply resource filters.
	filterChain, err := buildFilterChain(o, meta, mergedVals, resources)
	if err != nil {
		return nil, fmt.Errorf("building filter chain: %w", err)
	}

	if filterChain != nil {
		filterResult, filterErr := filterChain.Apply(ctx, resources)
		if filterErr != nil {
			return nil, fmt.Errorf("applying filters: %w", filterErr)
		}

		resources = filterResult.Included
	}

	if len(resources) == 0 {
		return nil, fmt.Errorf("no resources remaining after filtering")
	}

	// 7c. Load extensibility config.
	var transformCfg *config.TransformConfig

	if len(o.transformConfigData) > 0 {
		parsed, parseErr := config.ParseTransformConfig(o.transformConfigData)
		if parseErr == nil && !parsed.IsEmpty() {
			transformCfg = parsed
		}
	}

	// Merge resource ID overrides from options and config.
	resourceIDOverrides := mergeResourceIDOverrides(o.resourceIDOverrides, transformCfg)

	tempIDs, err := transform.AssignResourceIDs(resources, resourceIDOverrides)
	if err != nil {
		return nil, fmt.Errorf("assigning resource IDs: %w", err)
	}

	// 8. Detect parameter mappings.
	var fieldMappings []transform.FieldMapping

	referencedPaths := make(map[string]bool)

	if o.fast {
		templateFiles := make(map[string]string)
		for _, t := range ch.Templates {
			templateFiles[t.Name] = string(t.Data)
		}

		astRefs, astErr := transform.AnalyzeTemplates(templateFiles)
		if astErr == nil {
			referencedPaths = astRefs
			fieldMappings = transform.MatchFieldsByValue(resources, tempIDs, mergedVals, referencedPaths)
		}
	} else {
		fullSentinelValues := transform.SentinelizeAll(mergedVals)

		fullSentinelRendered, fullSentErr := helmRenderer.Render(renderCtx, ch, fullSentinelValues)

		var fullSentinelResources []*k8s.Resource
		if fullSentErr == nil {
			fullHookResult, hookErr := hooks.Filter(fullSentinelRendered, o.includeHooks, logger)
			if hookErr == nil {
				fullSentinelCombined := hooks.CombineResources(fullHookResult)

				parsed, parseErr := k8sParser.Parse(ctx, fullSentinelCombined)
				if parseErr == nil {
					fullSentinelResources = parsed
				}
			}
		}

		fieldMappings = transform.ParallelDiffAllResources(
			resources, fullSentinelResources, tempIDs, transform.ParallelDiffConfig{},
		)

		for _, m := range fieldMappings {
			referencedPaths[m.ValuesPath] = true
		}
	}

	// 9. Run transformation pipeline.
	engineCfg := transform.EngineConfig{
		IncludeAllValues:    o.includeAllValues,
		FlatSchema:          o.flatSchema,
		FieldMappings:       fieldMappings,
		ReferencedPaths:     referencedPaths,
		JSONSchemaBytes:     meta.Schema,
		ResourceIDOverrides: resourceIDOverrides,
	}

	// Apply schema overrides from options.
	if len(o.schemaOverrides) > 0 {
		engineCfg.SchemaOverrides = toTransformSchemaOverrides(o.schemaOverrides)
	}

	// Apply config-based overrides.
	if transformCfg != nil {
		if len(transformCfg.SchemaOverrides) > 0 && len(engineCfg.SchemaOverrides) == 0 {
			engineCfg.SchemaOverrides = configToSchemaOverrides(transformCfg.SchemaOverrides)
		}

		registry := transformer.DefaultRegistry()
		for i := len(transformCfg.Transformers) - 1; i >= 0; i-- {
			registry.Prepend(transformer.FromConfigOverride(transformCfg.Transformers[i]))
		}

		engineCfg.TransformerRegistry = registry
	}

	engine := transform.NewEngine(engineCfg)

	result, err := engine.Transform(ctx, resources, mergedVals)
	if err != nil {
		return nil, fmt.Errorf("transformation failed: %w", err)
	}

	// 9b. Security hardening (optional).
	var hardenSummary *HardenSummary

	if o.harden {
		hardenSummary, err = applyHardening(ctx, o, result)
		if err != nil {
			return nil, fmt.Errorf("hardening failed: %w", err)
		}
	}

	// 10. Generate RGD.
	var customReadyConditions map[string][]string
	if o.readyConditions != "" {
		customReadyConditions, err = transform.LoadCustomReadyConditions(o.readyConditions)
		if err != nil {
			return nil, fmt.Errorf("loading ready conditions: %w", err)
		}
	}

	generator := kro.NewGenerator(kro.GeneratorConfig{
		Name:                  meta.Name,
		ChartName:             meta.Name,
		ChartVersion:          meta.Version,
		SchemaKind:            o.kind,
		SchemaAPIVersion:      o.apiVersion,
		SchemaGroup:           o.group,
		SchemaFields:          result.SchemaFields,
		StatusFields:          result.StatusFields,
		CustomReadyConditions: customReadyConditions,
	})

	rgd, err := generator.Generate(result.DependencyGraph)
	if err != nil {
		return nil, fmt.Errorf("generating RGD: %w", err)
	}

	rgdMap := rgd.ToMap()

	// Serialize to YAML.
	yamlBytes, err := output.Serialize(rgdMap, output.SerializeOptions{
		Comments: false,
		Indent:   2,
	})
	if err != nil {
		return nil, fmt.Errorf("serializing RGD: %w", err)
	}

	// Normalize via round-trip.
	var normalized map[string]interface{}
	if unmarshalErr := sigsyaml.Unmarshal(yamlBytes, &normalized); unmarshalErr == nil {
		rgdMap = normalized
	}

	return &Result{
		YAML:             yamlBytes,
		RGDMap:           rgdMap,
		ChartName:        meta.Name,
		ChartVersion:     meta.Version,
		ResourceCount:    len(result.Resources),
		SchemaFieldCount: len(result.SchemaFields),
		DependencyEdges:  result.DependencyGraph.EdgeCount(),
		HardenResult:     hardenSummary,
	}, nil
}

// applyDefaults sets zero-value fields to sensible defaults.
func (o *options) applyDefaults() {
	if o.releaseName == "" {
		o.releaseName = "release"
	}

	if o.namespace == "" {
		o.namespace = "default"
	}

	if o.timeout == 0 {
		o.timeout = 30 * time.Second
	}
}

// mergeResourceIDOverrides merges Options overrides with config overrides.
// Options overrides take precedence.
func mergeResourceIDOverrides(optsOverrides map[string]string, cfg *config.TransformConfig) map[string]string {
	if cfg == nil || len(cfg.ResourceIDOverrides) == 0 {
		return optsOverrides
	}

	merged := make(map[string]string, len(cfg.ResourceIDOverrides)+len(optsOverrides))
	for k, v := range cfg.ResourceIDOverrides {
		merged[k] = v
	}

	for k, v := range optsOverrides {
		merged[k] = v
	}

	return merged
}

// toTransformSchemaOverrides converts public SchemaOverride to internal.
func toTransformSchemaOverrides(overrides map[string]SchemaOverride) map[string]transform.SchemaOverride {
	result := make(map[string]transform.SchemaOverride, len(overrides))
	for path, o := range overrides {
		def := ""
		if o.Default != nil {
			def = fmt.Sprintf("%v", o.Default)
		}

		result[path] = transform.SchemaOverride{
			Type:    o.Type,
			Default: def,
		}
	}

	return result
}

// configToSchemaOverrides converts config schema overrides to internal.
func configToSchemaOverrides(overrides map[string]config.SchemaOverride) map[string]transform.SchemaOverride {
	result := make(map[string]transform.SchemaOverride, len(overrides))
	for path, o := range overrides {
		result[path] = transform.SchemaOverride{
			Type:    o.Type,
			Default: o.Default,
		}
	}

	return result
}

// applyHardening applies security hardening to the transformation result.
func applyHardening(ctx context.Context, opts *options, result *transform.Result) (*HardenSummary, error) {
	secLevel, err := harden.ParseSecurityLevel(opts.securityLevel)
	if err != nil {
		return nil, err
	}

	hardenCfg := harden.Config{
		SecurityLevel:           secLevel,
		GenerateNetworkPolicies: opts.generateNetworkPolicies,
		GenerateRBAC:            opts.generateRBAC,
		ResolveDigests:          opts.resolveDigests,
		ResourceIDs:             result.ResourceIDs,
	}

	if len(opts.transformConfigData) > 0 {
		if fileCfg, parseErr := harden.ParseFileConfig(opts.transformConfigData); parseErr == nil && fileCfg != nil {
			hardenCfg.ImagePolicy = fileCfg.ToImagePolicyConfig()
			hardenCfg.ResourceDefaults = fileCfg.ToResourceDefaultsConfig()
		}
	}

	hardener := harden.New(hardenCfg)

	hardenResult, err := hardener.Harden(ctx, result.Resources)
	if err != nil {
		return nil, err
	}

	result.Resources = hardenResult.Resources

	return &HardenSummary{
		Changes:  len(hardenResult.Changes),
		Warnings: hardenResult.Warnings,
	}, nil
}

// buildFilterChain constructs the filter chain from options.
func buildFilterChain(opts *options, meta *chartmeta.ChartMeta, mergedVals map[string]interface{}, resources []*k8s.Resource) (*filter.Chain, error) {
	var filters []filter.Filter

	// 1. Profile-based filters.
	if opts.profile != "" {
		if filter.IsMinimalProfile(opts.profile) {
			filters = append(filters, filter.NewMinimalFilter())
		} else {
			var customProfiles map[string]filter.ProfileConfig
			if len(opts.transformConfigData) > 0 {
				customProfiles, _ = filter.ParseCustomProfiles(opts.transformConfigData)
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
		pattern, found := filter.DetectExternalPatternsForSubchart(meta, mergedVals, name)
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

	// 6. Resource ID exclusion.
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
		return nil, nil //nolint:nilnil // no filters needed
	}

	return filter.NewChain(filters...), nil
}

// assignResourceSourcePaths sets SourcePath on resources that match sourced manifests.
// Each sourced manifest may contain multiple YAML documents, so we parse each
// manifest's content and build a Kind/Name → path lookup.
func assignResourceSourcePaths(resources []*k8s.Resource, sourced []renderer.SourcedManifest) {
	pathLookup := make(map[string]string)

	for _, sm := range sourced {
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

	for _, res := range resources {
		key := res.Kind() + "/" + res.Name
		if path, ok := pathLookup[key]; ok {
			res.SourcePath = path
		}
	}
}
