package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// registerChartLoadingFlags adds the standard chart loading flags to a cobra command.
func registerChartLoadingFlags(cmd *cobra.Command, opts *convertOptions) {
	f := cmd.Flags()
	f.StringVar(&opts.repoURL, "repo-url", "", "Helm repository URL")
	f.StringVar(&opts.version, "version", "", "chart version constraint")
	f.StringVar(&opts.username, "username", "", "repository/registry username")
	f.StringVar(&opts.password, "password", "", "repository/registry password")
	f.StringVar(&opts.caFile, "ca-file", "", "TLS CA certificate file")
	f.StringVar(&opts.certFile, "cert-file", "", "TLS client certificate file")
	f.StringVar(&opts.keyFile, "key-file", "", "TLS client key file")
}

// registerRenderingFlags adds the standard template rendering flags to a cobra command.
func registerRenderingFlags(cmd *cobra.Command, opts *convertOptions) {
	f := cmd.Flags()
	f.StringVar(&opts.releaseName, "release-name", "release", "Helm release name for rendering")
	f.StringVar(&opts.namespace, "namespace", "default", "Kubernetes namespace for rendering")
	f.BoolVar(&opts.strict, "strict", false, "fail on missing template values")
	f.DurationVar(&opts.timeout, "timeout", 30*time.Second, "template rendering timeout")
}

// registerValuesFlags adds the Helm --values/--set family of flags to a cobra command.
func registerValuesFlags(cmd *cobra.Command, opts *convertOptions) {
	f := cmd.Flags()
	f.StringArrayVarP(&opts.valueFiles, "values", "f", nil, "values YAML files")
	f.StringArrayVar(&opts.values, "set", nil, "set values (key=value)")
	f.StringArrayVar(&opts.stringValues, "set-string", nil, "set string values")
	f.StringArrayVar(&opts.fileValues, "set-file", nil, "set values from files")
}

// registerTransformFlags adds transformation-related flags to a cobra command.
func registerTransformFlags(cmd *cobra.Command, opts *convertOptions) {
	f := cmd.Flags()
	f.StringVar(&opts.kind, "kind", "", "override generated CRD kind")
	f.StringVar(&opts.apiVersion, "api-version", "v1alpha1", "KRO schema apiVersion")
	f.StringVar(&opts.group, "group", "kro.run", "KRO schema group")
	f.BoolVar(&opts.includeAllValues, "include-all-values", false, "include all values in schema")
	f.BoolVar(&opts.flatSchema, "flat-schema", false, "use flat camelCase schema field names")
	f.StringVar(&opts.readyConditions, "ready-conditions", "", "custom readiness conditions file")
	f.BoolVar(&opts.fast, "fast", false, "use template AST analysis")
	f.BoolVar(&opts.includeHooks, "include-hooks", false, "include hook resources")
}

// registerResourceFilterFlags adds resource filtering flags to a cobra command.
func registerResourceFilterFlags(cmd *cobra.Command, opts *convertOptions) {
	f := cmd.Flags()
	f.StringSliceVar(&opts.excludeKinds, "exclude-kinds", nil, "exclude resources by kind")
	f.StringSliceVar(&opts.excludeResources, "exclude-resources", nil, "exclude resources by ID")
	f.StringSliceVar(&opts.excludeSubcharts, "exclude-subcharts", nil, "exclude subcharts")
	f.StringVar(&opts.excludeLabels, "exclude-labels", "", "exclude by label selector")
	f.StringVar(&opts.profile, "profile", "", "apply a conversion profile")
}

// registerHardenFlags adds security hardening flags to a cobra command.
func registerHardenFlags(cmd *cobra.Command, opts *convertOptions) {
	f := cmd.Flags()
	f.BoolVar(&opts.harden, "harden", false, "enable security hardening pipeline")
	f.StringVar(&opts.securityLevel, "security-level", "restricted", "PSS enforcement level (none, baseline, restricted)")
	f.BoolVar(&opts.generateNetworkPolicies, "generate-network-policies", false, "generate NetworkPolicies from dependency graph")
	f.BoolVar(&opts.generateRBAC, "generate-rbac", false, "generate least-privilege RBAC resources")
	f.BoolVar(&opts.resolveDigests, "resolve-digests", false, "resolve image tags to sha256 digests from container registries")
}

// registerPipelineFlags registers all shared pipeline flags (chart loading, rendering,
// values, transformation, resource filtering, and hardening) on a cobra command.
func registerPipelineFlags(cmd *cobra.Command, opts *convertOptions) {
	registerChartLoadingFlags(cmd, opts)
	registerRenderingFlags(cmd, opts)
	registerValuesFlags(cmd, opts)
	registerTransformFlags(cmd, opts)
	registerResourceFilterFlags(cmd, opts)
	registerHardenFlags(cmd, opts)
}
