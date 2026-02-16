package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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
	"github.com/hupe1980/chart2kro/internal/logging"
	"github.com/hupe1980/chart2kro/internal/output"
	"github.com/hupe1980/chart2kro/internal/transform"
	"github.com/hupe1980/chart2kro/internal/transform/transformer"
)

// pipelineResult holds the outputs of the chart-to-RGD generation pipeline.
type pipelineResult struct {
	RGDMap       map[string]interface{}
	Result       *transform.Result
	Meta         *chartmeta.ChartMeta
	HardenResult *harden.HardenResult
	HookResult   *hooks.FilterResult
	FilterResult *filter.Result
}

// runPipeline executes the full chart→RGD pipeline (steps 1-10 of runConvert)
// and returns the generated RGD map, transform result, and chart metadata.
// This is the shared core used by convert, diff, and plan commands.
func runPipeline(ctx context.Context, ref string, opts *convertOptions) (*pipelineResult, error) {
	logger := logging.FromContext(ctx)

	// 1. Load the chart.
	logger.Info("loading chart", slog.String("ref", ref))

	multiLoader := loader.NewMultiLoader()
	loadOpts := loader.LoadOptions{
		Version:  opts.version,
		RepoURL:  opts.repoURL,
		Username: opts.username,
		Password: opts.password,
		CaFile:   opts.caFile,
		CertFile: opts.certFile,
		KeyFile:  opts.keyFile,
	}

	ch, err := multiLoader.Load(ctx, ref, loadOpts)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("loading chart: %w", err)}
	}

	// 2. Extract chart metadata.
	meta := chartmeta.FromChart(ch)
	logger.Info("chart loaded",
		slog.String("name", meta.Name),
		slog.String("version", meta.Version),
		slog.String("appVersion", meta.AppVersion),
	)

	if meta.IsLibrary() {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("chart %q is a library chart and cannot be converted", meta.Name)}
	}

	// 3. Analyze dependencies.
	if meta.HasDependencies() {
		depResult := deps.Analyze(ch, logger)
		if !depResult.AllResolved {
			missing := deps.MissingDependencies(depResult)
			logger.Warn("some dependencies are not vendored", slog.Any("missing", missing))
		}
	}

	// 4. Merge values.
	valOpts := renderer.ValuesOptions{
		ValueFiles:   opts.valueFiles,
		Values:       opts.values,
		StringValues: opts.stringValues,
		FileValues:   opts.fileValues,
	}

	mergedVals, err := renderer.MergeValues(ch, valOpts)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("merging values: %w", err)}
	}

	// 5. Render templates.
	renderCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	helmRenderer := renderer.New(renderer.RenderOptions{
		ReleaseName: opts.releaseName,
		Namespace:   opts.namespace,
		Strict:      opts.strict,
	})

	needsSources := len(opts.excludeSubcharts) > 0 || opts.profile != "" || len(opts.useExternalPattern) > 0

	var rendered []byte

	var sourcedManifests []renderer.SourcedManifest

	if needsSources {
		sourcedManifests, err = helmRenderer.RenderWithSources(renderCtx, ch, mergedVals)
		if err != nil {
			return nil, &ExitError{Code: 1, Err: fmt.Errorf("rendering templates: %w", err)}
		}

		rendered = renderer.CombineSourcedManifests(sourcedManifests)
	} else {
		rendered, err = helmRenderer.Render(renderCtx, ch, mergedVals)
		if err != nil {
			return nil, &ExitError{Code: 1, Err: fmt.Errorf("rendering templates: %w", err)}
		}
	}

	// 6. Filter hooks.
	hookResult, err := hooks.Filter(rendered, opts.includeHooks, logger)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("filtering hooks: %w", err)}
	}

	// 7. Parse rendered YAML into K8s resources.
	combined := hooks.CombineResources(hookResult)

	k8sParser := parser.NewParser()

	resources, err := k8sParser.Parse(ctx, combined)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("parsing resources: %w", err)}
	}

	if len(resources) == 0 {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("no resources found in rendered output")}
	}

	logger.Info("parsed resources", slog.Int("count", len(resources)))

	if len(sourcedManifests) > 0 {
		assignResourceSourcePaths(resources, sourcedManifests)
	}

	// 7b. Apply resource filters.
	filterChain, err := buildFilterChain(ctx, opts, meta, mergedVals, resources)
	if err != nil {
		return nil, &ExitError{Code: 2, Err: fmt.Errorf("building filter chain: %w", err)}
	}

	var filterResult *filter.Result

	if filterChain != nil {
		filterResult, err = filterChain.Apply(ctx, resources)
		if err != nil {
			return nil, &ExitError{Code: 1, Err: fmt.Errorf("applying filters: %w", err)}
		}

		resources = filterResult.Included
	}

	if len(resources) == 0 {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("no resources remaining after filtering")}
	}

	// 7c. Load extensibility config early so resource ID overrides are available
	// for field mapping detection (step 8). This ensures the same IDs used for
	// sentinel diffing are also used by the transformation engine.
	var transformCfg *config.TransformConfig

	configData, _ := tryReadConfigFile(ctx)

	if configData != nil {
		parsed, parseErr := config.ParseTransformConfig(configData)
		if parseErr != nil {
			logger.Warn("transform config parse failed", slog.String("error", parseErr.Error()))
		} else if !parsed.IsEmpty() {
			transformCfg = parsed
			logger.Info("loaded transform config",
				slog.Int("transformers", len(transformCfg.Transformers)),
				slog.Int("schemaOverrides", len(transformCfg.SchemaOverrides)),
				slog.Int("resourceIdOverrides", len(transformCfg.ResourceIDOverrides)),
			)
		}
	}

	// 8. Detect parameter mappings.
	// Use resource ID overrides from config (if any) so IDs are consistent
	// between the pipeline's field mapping detection and the engine's transform.
	var resourceIDOverrides map[string]string
	if transformCfg != nil && len(transformCfg.ResourceIDOverrides) > 0 {
		resourceIDOverrides = transformCfg.ResourceIDOverrides
	}

	tempIDs, err := transform.AssignResourceIDs(resources, resourceIDOverrides)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("assigning resource IDs: %w", err)}
	}

	var fieldMappings []transform.FieldMapping

	referencedPaths := make(map[string]bool)

	if opts.fast {
		logger.Info("using fast mode (template AST analysis)")

		templateFiles := make(map[string]string)
		for _, t := range ch.Templates {
			templateFiles[t.Name] = string(t.Data)
		}

		astRefs, astErr := transform.AnalyzeTemplates(templateFiles)
		if astErr != nil {
			logger.Warn("template AST analysis failed", slog.String("error", astErr.Error()))
		} else {
			referencedPaths = astRefs
			fieldMappings = transform.MatchFieldsByValue(resources, tempIDs, mergedVals, referencedPaths)
		}
	} else {
		fullSentinelValues := transform.SentinelizeAll(mergedVals)

		fullSentinelRendered, fullSentErr := helmRenderer.Render(renderCtx, ch, fullSentinelValues)

		var fullSentinelResources []*k8s.Resource

		if fullSentErr != nil {
			logger.Warn("sentinel render failed", slog.String("error", fullSentErr.Error()))
		} else {
			fullHookResult, hookErr := hooks.Filter(fullSentinelRendered, opts.includeHooks, logger)
			if hookErr != nil {
				logger.Warn("sentinel hook filtering failed", slog.String("error", hookErr.Error()))
			} else {
				fullSentinelCombined := hooks.CombineResources(fullHookResult)

				parsed, parseErr := k8sParser.Parse(ctx, fullSentinelCombined)
				if parseErr != nil {
					logger.Warn("sentinel resource parsing failed", slog.String("error", parseErr.Error()))
				} else {
					fullSentinelResources = parsed
				}
			}
		}

		fieldMappings = transform.ParallelDiffAllResources(resources, fullSentinelResources, tempIDs, transform.ParallelDiffConfig{})

		for _, m := range fieldMappings {
			referencedPaths[m.ValuesPath] = true
		}
	}

	logger.Info("field mappings detected", slog.Int("count", len(fieldMappings)))

	// 8b. Prune orphaned schema fields.
	if filterResult != nil && len(filterResult.Excluded) > 0 && len(fieldMappings) > 0 {
		mappingRefs := make([]filter.FieldMappingRef, len(fieldMappings))
		for i, m := range fieldMappings {
			mappingRefs[i] = filter.FieldMappingRef{
				ResourceID: m.ResourceID,
				ValuesPath: m.ValuesPath,
			}
		}

		orphaned := filter.PruneOrphanedFields(mappingRefs, filterResult.Excluded, tempIDs)
		if len(orphaned) > 0 {
			for path := range orphaned {
				delete(referencedPaths, path)
			}
		}
	}

	// 9. Run transformation pipeline.
	engineCfg := transform.EngineConfig{
		IncludeAllValues:    opts.includeAllValues,
		FlatSchema:          opts.flatSchema,
		FieldMappings:       fieldMappings,
		ReferencedPaths:     referencedPaths,
		JSONSchemaBytes:     meta.Schema,
		ResourceIDOverrides: resourceIDOverrides,
	}

	// 9a. Apply extensibility config (transformers, schema overrides) from the
	// config loaded in step 7c.
	if transformCfg != nil {
		// Apply schema overrides.
		if len(transformCfg.SchemaOverrides) > 0 {
			engineCfg.SchemaOverrides = toSchemaOverrides(transformCfg.SchemaOverrides)
		}

		// Build transformer registry with config-based overrides prepended.
		registry := transformer.DefaultRegistry()
		for i := len(transformCfg.Transformers) - 1; i >= 0; i-- {
			registry.Prepend(transformer.FromConfigOverride(transformCfg.Transformers[i]))
		}

		engineCfg.TransformerRegistry = registry
	}

	engine := transform.NewEngine(engineCfg)

	result, err := engine.Transform(ctx, resources, mergedVals)
	if err != nil {
		var cycleErr *transform.CycleError
		if errors.As(err, &cycleErr) {
			return nil, &ExitError{Code: 5, Err: fmt.Errorf("dependency cycle detected: %w", err)}
		}

		return nil, &ExitError{Code: 1, Err: fmt.Errorf("transformation failed: %w", err)}
	}

	// 9b. Security hardening (optional).
	var hardenResult *harden.HardenResult

	if opts.harden {
		secLevel, levelErr := harden.ParseSecurityLevel(opts.securityLevel)
		if levelErr != nil {
			return nil, &ExitError{Code: 2, Err: levelErr}
		}

		hardenCfg := harden.Config{
			SecurityLevel:           secLevel,
			GenerateNetworkPolicies: opts.generateNetworkPolicies,
			GenerateRBAC:            opts.generateRBAC,
			ResolveDigests:          opts.resolveDigests,
			ResourceIDs:             result.ResourceIDs,
		}

		// Reuse the config data loaded in step 7c.
		if configData != nil {
			if fileCfg, parseErr := harden.ParseFileConfig(configData); parseErr == nil && fileCfg != nil {
				hardenCfg.ImagePolicy = fileCfg.ToImagePolicyConfig()
				hardenCfg.ResourceDefaults = fileCfg.ToResourceDefaultsConfig()
			}
		}

		hardener := harden.New(hardenCfg)

		hardenResult, err = hardener.Harden(ctx, result.Resources)
		if err != nil {
			return nil, &ExitError{Code: 1, Err: fmt.Errorf("hardening failed: %w", err)}
		}

		// Update resources with hardened versions.
		result.Resources = hardenResult.Resources

		logger.Info("hardening complete",
			slog.Int("changes", len(hardenResult.Changes)),
			slog.Int("warnings", len(hardenResult.Warnings)),
		)

		for _, w := range hardenResult.Warnings {
			logger.Warn("hardening warning", slog.String("detail", w))
		}
	}

	// 10. Generate RGD.
	rgdName := meta.Name

	var customReadyConditions map[string][]string

	if opts.readyConditions != "" {
		customReadyConditions, err = transform.LoadCustomReadyConditions(opts.readyConditions)
		if err != nil {
			return nil, &ExitError{Code: 1, Err: fmt.Errorf("loading ready conditions: %w", err)}
		}
	}

	generator := kro.NewGenerator(kro.GeneratorConfig{
		Name:                  rgdName,
		ChartName:             meta.Name,
		ChartVersion:          meta.Version,
		SchemaKind:            opts.kind,
		SchemaAPIVersion:      opts.apiVersion,
		SchemaGroup:           opts.group,
		SchemaFields:          result.SchemaFields,
		StatusFields:          result.StatusFields,
		CustomReadyConditions: customReadyConditions,
	})

	rgd, err := generator.Generate(result.DependencyGraph)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("generating RGD: %w", err)}
	}

	rgdMap := rgd.ToMap()

	// Add provenance annotations when hardening is enabled.
	if opts.harden {
		provAnnotations, provErr := harden.GenerateProvenanceAnnotations(harden.ProvenanceConfig{
			ChartRef:          ref,
			Profile:           opts.profile,
			HardeningLevel:    harden.SecurityLevel(opts.securityLevel),
			ExcludedSubcharts: opts.excludeSubcharts,
			EmbedTimestamp:    false, // pipeline (diff/plan) never embeds timestamps
		})
		if provErr == nil {
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
	}

	// Serialize to YAML and re-parse so rgdMap matches what would be written to disk.
	// This ensures the diff/plan see the same format as the output.
	serOpts := output.SerializeOptions{Comments: false, Indent: 2}

	yamlBytes, err := output.Serialize(rgdMap, serOpts)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("serializing RGD: %w", err)}
	}

	var normalized map[string]interface{}
	if unmarshalErr := sigsyaml.Unmarshal(yamlBytes, &normalized); unmarshalErr != nil {
		// Fall back to non-normalized map if unmarshal fails.
		normalized = rgdMap
	}

	return &pipelineResult{
		RGDMap:       normalized,
		Result:       result,
		Meta:         meta,
		HardenResult: hardenResult,
		HookResult:   hookResult,
		FilterResult: filterResult,
	}, nil
}

// toSchemaOverrides converts config schema overrides to transform schema overrides.
func toSchemaOverrides(overrides map[string]config.SchemaOverride) map[string]transform.SchemaOverride {
	result := make(map[string]transform.SchemaOverride, len(overrides))
	for path, override := range overrides {
		result[path] = transform.SchemaOverride{
			Type:    override.Type,
			Default: override.Default,
		}
	}

	return result
}
