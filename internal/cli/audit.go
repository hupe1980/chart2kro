package cli

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/hupe1980/chart2kro/internal/audit"
	"github.com/hupe1980/chart2kro/internal/harden"
	"github.com/hupe1980/chart2kro/internal/helm/hooks"
	"github.com/hupe1980/chart2kro/internal/helm/loader"
	"github.com/hupe1980/chart2kro/internal/helm/renderer"
	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/k8s/parser"
	"github.com/hupe1980/chart2kro/internal/logging"
)

type auditOptions struct {
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

	// Hooks.
	includeHooks bool

	// Audit-specific.
	format        string
	failOn        string
	securityLevel string
	policyPaths   []string
}

func newAuditCommand() *cobra.Command {
	opts := &auditOptions{}

	cmd := &cobra.Command{
		Use:   "audit <chart-reference>",
		Short: "Run security and best-practice audits",
		Long: `Audit a Helm chart for security issues, Kubernetes best practices,
and policy violations.

The auditor loads and renders the chart, then examines every resulting
resource against built-in rules (SEC-001 through SEC-012) and any
custom policies supplied via --policy.

Use --fail-on to set a severity threshold: the command exits with
code 9 if any finding meets or exceeds the threshold.

Output formats: table (default), json, sarif.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAudit(cmd.Context(), cmd, args[0], opts)
		},
	}

	f := cmd.Flags()

	// Chart loading flags.
	f.StringVar(&opts.repoURL, "repo-url", "", "Helm repository URL")
	f.StringVar(&opts.version, "version", "", "chart version constraint")
	f.StringVar(&opts.username, "username", "", "repository/registry username")
	f.StringVar(&opts.password, "password", "", "repository/registry password")

	// Rendering flags.
	f.StringVar(&opts.releaseName, "release-name", "release", "Helm release name for rendering")
	f.StringVar(&opts.namespace, "namespace", "default", "Kubernetes namespace for rendering")
	f.DurationVar(&opts.timeout, "timeout", 30*time.Second, "template rendering timeout")

	// Values flags.
	f.StringArrayVarP(&opts.valueFiles, "values", "f", nil, "values YAML files")
	f.StringArrayVar(&opts.values, "set", nil, "set values (key=value)")
	f.StringArrayVar(&opts.stringValues, "set-string", nil, "set string values")

	// Hook flag.
	f.BoolVar(&opts.includeHooks, "include-hooks", false, "include hook resources in audit")

	// Audit flags.
	f.StringVar(&opts.format, "format", "table", "output format: table, json, sarif")
	f.StringVar(&opts.failOn, "fail-on", "", "fail with exit code 9 if findings >= severity (critical, high, medium, low, info)")
	f.StringVar(&opts.securityLevel, "security-level", "restricted", "PSS enforcement level (none, baseline, restricted)")
	f.StringArrayVar(&opts.policyPaths, "policy", nil, "custom policy YAML files (can specify multiple)")

	return cmd
}

func runAudit(ctx context.Context, cmd *cobra.Command, ref string, opts *auditOptions) error {
	logger := logging.FromContext(ctx)

	// 1. Build formatter early so we fail fast on bad format.
	formatter, err := audit.NewFormatter(opts.format)
	if err != nil {
		return &ExitError{Code: 2, Err: err}
	}

	// 2. Parse security level.
	secLevel, err := harden.ParseSecurityLevel(opts.securityLevel)
	if err != nil {
		return &ExitError{Code: 2, Err: err}
	}

	// 3. Load and render the chart.
	resources, err := loadChartResources(ctx, ref, opts)
	if err != nil {
		return err
	}

	logger.Info("audit: loaded resources", slog.Int("count", len(resources)))

	// 4. Build checks.
	checks := audit.DefaultChecks(secLevel)

	// 5. Load custom policies.
	for _, path := range opts.policyPaths {
		pf, loadErr := audit.LoadPolicyFile(path)
		if loadErr != nil {
			return &ExitError{Code: 1, Err: fmt.Errorf("loading policy %s: %w", path, loadErr)}
		}

		checks = append(checks, pf.ToChecks()...)

		logger.Info("audit: loaded custom policy",
			slog.String("path", path),
			slog.Int("rules", len(pf.Rules)),
		)
	}

	// 6. Run audit.
	auditor := audit.New(checks...)
	result := auditor.Run(ctx, resources)

	// 7. Format output.
	if err := formatter.Format(cmd.OutOrStdout(), result); err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("formatting results: %w", err)}
	}

	// 8. Check threshold.
	if opts.failOn != "" {
		threshold, parseErr := audit.ParseSeverity(opts.failOn)
		if parseErr != nil {
			return &ExitError{Code: 2, Err: parseErr}
		}

		if !result.Passed(threshold) {
			return &ExitError{
				Code: 9,
				Err:  fmt.Errorf("audit failed: findings at or above %s severity", threshold.String()),
			}
		}
	}

	return nil
}

// loadChartResources loads a Helm chart, renders its templates, and parses
// the resulting Kubernetes resources.
func loadChartResources(ctx context.Context, ref string, opts *auditOptions) ([]*k8s.Resource, error) {
	logger := logging.FromContext(ctx)

	logger.Info("loading chart", slog.String("ref", ref))

	multiLoader := loader.NewMultiLoader()
	ch, err := multiLoader.Load(ctx, ref, loader.LoadOptions{
		Version:  opts.version,
		RepoURL:  opts.repoURL,
		Username: opts.username,
		Password: opts.password,
	})
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("loading chart: %w", err)}
	}

	// Merge values.
	valOpts := renderer.ValuesOptions{
		ValueFiles:   opts.valueFiles,
		Values:       opts.values,
		StringValues: opts.stringValues,
	}

	mergedVals, err := renderer.MergeValues(ch, valOpts)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("merging values: %w", err)}
	}

	// Render templates.
	renderCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	helmRenderer := renderer.New(renderer.RenderOptions{
		ReleaseName: opts.releaseName,
		Namespace:   opts.namespace,
	})

	rendered, err := helmRenderer.Render(renderCtx, ch, mergedVals)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("rendering templates: %w", err)}
	}

	// Filter hooks.
	hookResult, err := hooks.Filter(rendered, opts.includeHooks, logger)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("filtering hooks: %w", err)}
	}

	combined := hooks.CombineResources(hookResult)

	// Parse resources.
	k8sParser := parser.NewParser()

	resources, err := k8sParser.Parse(ctx, combined)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("parsing resources: %w", err)}
	}

	if len(resources) == 0 {
		return nil, &ExitError{Code: 1, Err: fmt.Errorf("no resources found in rendered output")}
	}

	return resources, nil
}
