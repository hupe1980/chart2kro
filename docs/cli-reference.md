# CLI Reference

`chart2kro` — Transform Helm charts into KRO ResourceGraphDefinitions.

## Usage

```
chart2kro [command] [flags]
```

## Global Flags

These flags are available on all commands:

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--config <path>` | | `.chart2kro.yaml` | Path to configuration file |
| `--log-level <level>` | | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `--log-format <format>` | | `text` | Log output format: `text`, `json` |
| `--no-color` | | `false` | Disable ANSI colored output |
| `--quiet` | `-q` | `false` | Suppress non-essential output (sets log level to `error`) |
| `--help` | `-h` | | Show help for any command |

## Commands

### `chart2kro convert`

Convert a Helm chart to a KRO ResourceGraphDefinition.

```
chart2kro convert <chart-reference> [flags]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `chart-reference` | Path to a local chart directory, `.tgz` archive, OCI reference (`oci://...`), or `repo/chart` |

**Chart Loading Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--repo-url <url>` | | Helm repository URL (required for `repo/chart` references) |
| `--version <constraint>` | | Chart version constraint |
| `--username <user>` | | Repository/registry username |
| `--password <pass>` | | Repository/registry password |
| `--ca-file <path>` | | TLS CA certificate file |
| `--cert-file <path>` | | TLS client certificate file |
| `--key-file <path>` | | TLS client key file |

**Rendering Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--release-name <name>` | `release` | Helm release name for template rendering |
| `--namespace <ns>` | `default` | Kubernetes namespace for template rendering |
| `--strict` | `false` | Fail on missing template values |
| `--timeout <duration>` | `30s` | Template rendering timeout |

**Values Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--values <file>` | `-f` | Values YAML file (can specify multiple, last wins) |
| `--set <key=value>` | | Set values (dotted paths for nested values) |
| `--set-string <key=value>` | | Set string values |
| `--set-file <key=filepath>` | | Set values from file content |

**Hook Handling Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--include-hooks` | `false` | Include hook resources as regular resources (strips `helm.sh/hook` annotations) |

**Resource Filtering Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--exclude-kinds <kinds>` | | Exclude resources by kind, comma-separated (e.g., `Secret,ConfigMap`) |
| `--exclude-resources <ids>` | | Exclude resources by assigned ID, comma-separated (e.g., `secretDb,configMapRedis`) |
| `--exclude-subcharts <names>` | | Exclude all resources from named subcharts, comma-separated (e.g., `postgresql,redis`) |
| `--exclude-labels <selector>` | | Exclude resources matching a label selector (e.g., `component=database,tier!=frontend`) |
| `--externalize-secret <spec>` | | Externalize a Secret as a schema reference (`name=schemaField`). Can be repeated |
| `--externalize-service <spec>` | | Externalize a Service as a schema reference (`name=schemaField`). Can be repeated |
| `--use-external-pattern <name>` | | Auto-detect and apply external pattern for a subchart (e.g., `postgresql`). Comma-separated |
| `--profile <name>` | | Apply a conversion profile: `enterprise`, `minimal`, `app-only`, or a custom profile name |

Label selectors support Kubernetes-style operators: `key=value` (equality), `key!=value` (inequality), `key in (v1,v2)` (set membership). Multiple selectors are combined with AND semantics.

**Profiles:**

| Profile | Description |
|---------|-------------|
| `enterprise` | Excludes common infrastructure subcharts (postgresql, redis, elasticsearch, etc.) |
| `minimal` | Excludes **all** subchart resources, keeping only the root chart |
| `app-only` | Excludes StatefulSet and PersistentVolumeClaim kinds (stateless app only) |

Custom profiles can be defined in `.chart2kro.yaml` under the `profiles:` key and referenced by name. Custom profiles can extend built-in profiles using the `extends` field.

**Transformation Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--kind <name>` | | Override the generated schema kind (default: PascalCase chart name). Does not affect RGD metadata name. |
| `--api-version <ver>` | `v1alpha1` | KRO schema apiVersion |
| `--group <group>` | `kro.run` | KRO schema group |
| `--include-all-values` | `false` | Include all values in schema, not just referenced ones |
| `--flat-schema` | `false` | Use flat camelCase schema field names instead of nested objects |
| `--ready-conditions <file>` | | Path to a YAML file with custom readiness conditions per Kind |
| `--fast` | `false` | Use template AST analysis instead of sentinel rendering (faster, may miss complex expressions) |

**Security Hardening Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--harden` | `false` | Enable security hardening |
| `--security-level <level>` | `restricted` | Pod Security Standards level: `none`, `baseline`, `restricted` |
| `--generate-network-policies` | `false` | Generate deny-all NetworkPolicies with DNS egress for each workload |
| `--generate-rbac` | `false` | Generate ServiceAccount, Role, and RoleBinding per workload |
| `--resolve-digests` | `false` | Resolve image tags to sha256 digests from container registries |

When `--harden` is enabled, the following security policies are applied:

- **Pod Security Standards** — Enforce Kubernetes PSS at the chosen level (restricted or baseline)
- **Resource Requirements** — Inject default CPU/memory requests and limits into containers
- **Image Policy** — Warn on `:latest` tags, unapproved registries, and missing digests (configured via `.chart2kro.yaml`)
- **Digest Resolution** — Resolve image tags to sha256 digests by querying container registries (`--resolve-digests`)
- **NetworkPolicy Generation** — Create deny-all + DNS egress policies per workload, with ingress rules from matching Services
- **RBAC Generation** — Create ServiceAccount, Role (least-privilege), and RoleBinding per workload
- **SLSA Provenance** — Add `chart2kro.io/provenance` annotation with SLSA v1.0 attestation

Image policy and resource defaults can be configured in `.chart2kro.yaml` under the `harden:` section. See [configuration.md](configuration.md) for details.

**Output Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output <path>` | `-o` | stdout | Output file path |
| `--dry-run` | | `false` | Preview output without writing files |
| `--comments` | | `false` | Add inline comments on CEL expressions |
| `--split` | | `false` | Write one YAML file per resource (requires `--output-dir`) |
| `--output-dir <dir>` | | | Output directory for `--split` |
| `--embed-timestamp` | | `false` | Add `chart2kro.io/generated-at` annotation |

**Examples:**

```bash
# Convert a local chart directory (outputs RGD to stdout)
chart2kro convert ./my-chart/

# Convert and write to a file
chart2kro convert ./my-chart/ -o rgd.yaml

# Convert a packaged chart archive
chart2kro convert ./my-chart-1.0.0.tgz

# Convert from an OCI registry
chart2kro convert oci://ghcr.io/org/my-chart:1.0.0

# Convert from a Helm repository
chart2kro convert my-chart --repo-url https://charts.example.com

# Convert with custom values
chart2kro convert ./my-chart/ -f custom-values.yaml --set replicas=3

# Convert with custom release name and namespace
chart2kro convert ./my-chart/ --release-name myapp --namespace production

# Convert including hook resources
chart2kro convert ./my-chart/ --include-hooks

# Convert with custom CRD kind name
chart2kro convert ./my-chart/ --kind MyApplication -o rgd.yaml

# Convert with flat schema mode
chart2kro convert ./my-chart/ --flat-schema -o rgd.yaml

# Include all Helm values in the schema
chart2kro convert ./my-chart/ --include-all-values

# Preview output without writing (dry-run)
chart2kro convert ./my-chart/ --dry-run

# Use fast mode (template AST analysis, no sentinel rendering)
chart2kro convert ./my-chart/ --fast

# Custom readiness conditions from a YAML file
chart2kro convert ./my-chart/ --ready-conditions custom-ready.yaml

# Add inline comments on CEL expressions
chart2kro convert ./my-chart/ --comments -o rgd.yaml

# Split output into per-resource files with kustomization.yaml
chart2kro convert ./my-chart/ --split --output-dir ./kro-output/

# Add a generated-at timestamp annotation
chart2kro convert ./my-chart/ --embed-timestamp -o rgd.yaml

# Exclude all Secret and ConfigMap resources
chart2kro convert ./my-chart/ --exclude-kinds Secret,ConfigMap

# Exclude PostgreSQL subchart resources
chart2kro convert ./my-chart/ --exclude-subcharts postgresql

# Exclude resources by label selector
chart2kro convert ./my-chart/ --exclude-labels "component=database"

# Externalize a database Secret as a schema field
chart2kro convert ./my-chart/ --externalize-secret db-credentials=externalDatabaseSecret

# Auto-detect external pattern for a PostgreSQL subchart
chart2kro convert ./my-chart/ --use-external-pattern postgresql

# Use the enterprise profile (excludes common infra subcharts)
chart2kro convert ./my-chart/ --profile enterprise

# Use the minimal profile (root chart only, no subcharts)
chart2kro convert ./my-chart/ --profile minimal

# Combine filters: exclude subcharts + externalize a secret
chart2kro convert ./my-chart/ --exclude-subcharts redis \
  --externalize-secret db-secret=externalDatabase

# Enable security hardening (PSS restricted + resource defaults)
chart2kro convert ./my-chart/ --harden -o rgd.yaml

# Harden with baseline security level
chart2kro convert ./my-chart/ --harden --security-level baseline

# Harden with NetworkPolicy and RBAC generation
chart2kro convert ./my-chart/ --harden --generate-network-policies --generate-rbac

# Full hardening: restricted PSS + netpol + RBAC + provenance
chart2kro convert ./my-chart/ --harden --generate-network-policies --generate-rbac \
  --embed-timestamp -o rgd.yaml

# Resolve image tags to sha256 digests during hardening
chart2kro convert ./my-chart/ --harden --resolve-digests -o rgd.yaml
```

---

### `chart2kro inspect`

Inspect a Helm chart without converting it.

```
chart2kro inspect <chart-reference> [flags]
```

Preview what resources would be generated, which values would be exposed as API fields, and what transformations would be applied — without performing an actual conversion.

Displays chart metadata, resource table, schema preview, dependency graph, subchart details, and detected external patterns with suggested flags.

**Arguments:**

| Argument | Description |
|----------|-------------|
| `chart-reference` | Path to a local chart directory, `.tgz` archive, OCI reference (`oci://...`), or `repo/chart` |

**Chart Loading Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--repo-url <url>` | | Helm repository URL (required for `repo/chart` references) |
| `--version <constraint>` | | Chart version constraint |
| `--username <user>` | | Repository/registry username |
| `--password <pass>` | | Repository/registry password |

**Rendering Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--release-name <name>` | `release` | Helm release name for template rendering |
| `--namespace <ns>` | `default` | Kubernetes namespace for template rendering |
| `--timeout <duration>` | `30s` | Template rendering timeout |

**Values Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--values <file>` | `-f` | Values YAML file (can specify multiple) |
| `--set <key=value>` | | Set values (dotted paths for nested values) |
| `--set-string <key=value>` | | Set string values |

**Output Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--format <fmt>` | `table` | Output format: `table`, `json`, `yaml` |
| `--show-resources` | `false` | Show only the resource table |
| `--show-values` | `false` | Show only values / schema fields |
| `--show-deps` | `false` | Show only the dependency graph |
| `--show-schema` | `false` | Show only the generated schema preview |

**Examples:**

```bash
# Inspect a local chart
chart2kro inspect ./my-chart/

# Inspect with JSON output
chart2kro inspect ./my-chart/ --format json

# Inspect with YAML output
chart2kro inspect ./my-chart/ --format yaml

# Show only the resource table
chart2kro inspect ./my-chart/ --show-resources

# Show only the generated schema preview
chart2kro inspect ./my-chart/ --show-schema

# Inspect with custom values
chart2kro inspect ./my-chart/ -f production-values.yaml

# Inspect an OCI chart
chart2kro inspect oci://ghcr.io/org/my-chart:1.0.0
```

---

### `chart2kro validate`

Validate a generated ResourceGraphDefinition.

```
chart2kro validate <file> [flags]
```

Validates against the KRO schema, Kubernetes API conventions, and CEL expression syntax.
Reports all errors and warnings found in the RGD file.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--strict` | `false` | Fail on warnings in addition to errors |

**Checks performed:**

- YAML syntax validity
- Required fields: `apiVersion`, `kind`, `metadata.name`, `spec.schema`, `spec.resources`
- Schema field types (valid SimpleSchema syntax)
- Schema `kind` is PascalCase
- CEL expression references (`${schema.spec.*}`, `${<resourceId>.*}`)
- Unique resource IDs
- Dependency graph cycles
- GVK well-formedness

**Examples:**

```bash
# Validate an RGD file
chart2kro validate rgd.yaml

# Strict mode — fail on warnings too
chart2kro validate --strict rgd.yaml
```

---

### `chart2kro export`

Export a ResourceGraphDefinition in various formats.

```
chart2kro export <file> [flags]
```

Export as YAML, JSON, or Kustomize format.

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--format <fmt>` | | `yaml` | Output format: `yaml`, `json`, `kustomize` |
| `--output <path>` | `-o` | stdout | Output file path |
| `--output-dir <dir>` | | | Output directory (required for `kustomize` format) |
| `--comments` | | `false` | Add inline comments on CEL expressions |

**Examples:**

```bash
# Re-serialize to canonical YAML (normalize formatting)
chart2kro export rgd.yaml

# Export as JSON
chart2kro export rgd.yaml --format json

# Export as JSON to a file
chart2kro export rgd.yaml --format json -o rgd.json

# Export as Kustomize directory
chart2kro export rgd.yaml --format kustomize --output-dir ./kro-kustomize/

# Export with inline comments
chart2kro export rgd.yaml --comments
```

---

### `chart2kro diff`

Compare a newly generated RGD against a previous version.

```
chart2kro diff <chart-reference> --existing <rgd-file> [flags]
```

Diff loads the existing RGD file from disk, re-runs the full conversion pipeline on the chart, and
produces a unified diff between the two. Schema evolution analysis is automatically included,
highlighting breaking and non-breaking changes.

Use this for **upgrade detection** (has the upstream chart changed?), **drift detection** in CI
(is the committed RGD up-to-date?), and **safe regeneration** (review before overwriting).

**Arguments:**

| Argument | Description |
|----------|-------------|
| `chart-reference` | Path to a local chart directory, `.tgz` archive, OCI reference (`oci://...`), or `repo/chart` |

**Diff-Specific Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--existing <path>` | *(required)* | Path to the existing RGD YAML file to diff against |
| `--format <fmt>` | `unified` | Output format: `unified`, `json` |
| `--no-color` | `false` | Disable ANSI color output in unified diff |

**Shared Flags:**

All chart loading, rendering, values, transformation, hook handling, and resource filtering flags from `convert` are supported (e.g., `--release-name`, `--values`, `--kind`, `--profile`, `--fast`, etc.).

**Exit Codes:**

| Code | Meaning |
|------|---------|
| `0` | No differences found |
| `1` | General error |
| `2` | Invalid arguments (e.g., missing `--existing`) |
| `8` | Breaking schema changes detected |

**Examples:**

```bash
# Compare against an existing RGD file (unified diff + evolution summary)
chart2kro diff ./my-chart/ --existing rgd.yaml

# Disable colored output (for piping or CI logs)
chart2kro diff ./my-chart/ --existing rgd.yaml --no-color

# JSON output for CI automation (pipe to jq)
chart2kro diff ./my-chart/ --existing rgd.yaml --format json

# Diff with custom values
chart2kro diff ./my-chart/ --existing rgd.yaml -f production.yaml --set replicas=5

# Diff with a conversion profile
chart2kro diff ./my-chart/ --existing rgd.yaml --profile enterprise

# CI drift detection — exit code 8 means drift or breaking changes
chart2kro diff ./my-chart/ --existing kro/rgds/chart.yaml || echo "RGD is stale"
```

---

### `chart2kro audit`

Run security analysis on a Helm chart and report findings.

```
chart2kro audit <chart-reference> [flags]
```

Audits rendered Kubernetes resources for security issues, categorised by
severity (`critical`, `high`, `medium`, `low`, `info`). Supports table, JSON,
and SARIF v2.1.0 output. Custom policy files can extend the built-in checks.

**Chart Loading & Rendering Flags:**

All chart loading flags from `convert` are supported (`--repo-url`, `--version`,
`--username`, `--password`) as well as rendering flags (`--release-name`,
`--namespace`, `--timeout`, `--values`, `--set`, `--set-string`, `--include-hooks`).

**Audit-Specific Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--format <fmt>` | | `table` | Output format: `table`, `json`, `sarif` |
| `--fail-on <severity>` | | | Minimum severity that causes a non-zero exit (exit code 9) |
| `--security-level <level>` | | `restricted` | Target PSS level: `none`, `baseline`, `restricted` |
| `--policy <path>` | | | Custom policy YAML file (can be repeated) |

**Security Level Filtering:**

The `--security-level` flag controls which built-in checks are active:

| Level | Checks |
|-------|--------|
| `restricted` | All 12 checks (SEC-001 through SEC-012) |
| `baseline` | Best-practice checks + baseline PSS checks (excludes SEC-006, SEC-012) |
| `none` | Best-practice checks only: SEC-003, SEC-004, SEC-007, SEC-009, SEC-010, SEC-011 |

**Built-In Checks:**

| Rule | Severity | Description |
|------|----------|-------------|
| SEC-001 | Critical | Container runs as root (no `runAsNonRoot: true`) |
| SEC-002 | Critical | Container is privileged |
| SEC-003 | High | No resource limits defined |
| SEC-004 | High | Image uses `:latest` tag |
| SEC-005 | High | Host networking/PID/IPC enabled |
| SEC-006 | Medium | `readOnlyRootFilesystem` not set |
| SEC-007 | Medium | No NetworkPolicy covers the namespace |
| SEC-008 | Medium | Dangerous capabilities (SYS_ADMIN, NET_RAW, ALL) |
| SEC-009 | Low | Broad label selector (≤1 match label) |
| SEC-010 | Low | Missing liveness/readiness probes |
| SEC-011 | Info | Ingress without TLS configuration |
| SEC-012 | Info | No Seccomp profile set |

**Custom Policy File Format:**

```yaml
rules:
  - id: CUSTOM-001
    severity: high
    match:
      kind: Deployment
    condition: "no liveness probe"
    message: "All Deployments must have liveness probes"
    remediation: "Add spec.template.spec.containers[*].livenessProbe"
```

Supported conditions: `no liveness probe`, `no readiness probe`,
`no resource limits`, `uses latest tag`, `privileged`,
`host networking`, `no seccomp profile`.

**Exit Codes:**

| Code | Meaning |
|------|---------|
| 0 | No findings at or above the `--fail-on` threshold |
| 1 | Unexpected error |
| 2 | Invalid arguments |
| 9 | Findings at or above the `--fail-on` threshold |

**Examples:**

```bash
# Audit a chart with default settings
chart2kro audit ./my-chart/

# Fail CI on high or above, output SARIF
chart2kro audit ./my-chart/ --fail-on high --format sarif > results.sarif

# Use a custom policy file
chart2kro audit ./my-chart/ --policy ./policies/org-policy.yaml
```

---

### `chart2kro docs`

Generate API reference documentation from a ResourceGraphDefinition.

```
chart2kro docs <rgd-file> [flags]
```

Parses an RGD YAML file and generates human-readable documentation covering
spec fields (with types and defaults), status fields, managed resources,
and optionally an example YAML instance.

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--format <fmt>` | `-f` | `markdown` | Output format: `markdown`, `html`, `asciidoc` |
| `--title <text>` | | | Override document title |
| `--include-examples` | | `true` | Include example YAML in output |
| `--output <path>` | `-o` | | Write to file instead of stdout |

**Exit Codes:**

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Unexpected error |
| 2 | Invalid arguments |
| 7 | YAML syntax error |

**Examples:**

```bash
# Generate markdown docs from an RGD
chart2kro docs my-rgd.yaml

# Generate HTML, write to file
chart2kro docs my-rgd.yaml --format html -o api-reference.html

# AsciiDoc without examples
chart2kro docs my-rgd.yaml --format asciidoc --include-examples=false

# Custom title
chart2kro docs my-rgd.yaml --title "MyApp Custom Resource"
```

---

### `chart2kro plan`

Preview what a conversion would produce (Terraform-like plan).

```
chart2kro plan <chart-reference> [flags]
```

Plan runs the full conversion pipeline in memory and displays the result as a structured
preview: schema fields, resources, status projections, and (optionally) schema evolution
analysis against an existing RGD.

No files are written — this is a read-only preview command.

**Arguments:**

| Argument | Description |
|----------|-------------|
| `chart-reference` | Path to a local chart directory, `.tgz` archive, OCI reference (`oci://...`), or `repo/chart` |

**Plan-Specific Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--existing <path>` | | Path to an existing RGD YAML file for evolution analysis |
| `--format <fmt>` | `table` | Output format: `table`, `json`, `compact` |

**Shared Flags:**

All chart loading, rendering, values, transformation, hook handling, and resource filtering flags from `convert` are supported (e.g., `--release-name`, `--values`, `--kind`, `--profile`, `--fast`, etc.).

**Exit Codes:**

| Code | Meaning |
|------|---------|
| `0` | Success (no breaking changes) |
| `1` | General error |
| `2` | Invalid arguments |
| `8` | Breaking schema changes detected (when `--existing` is used) |

**Examples:**

```bash
# Preview the conversion result (table format)
chart2kro plan ./my-chart/

# Compact summary (one-liner)
chart2kro plan ./my-chart/ --format compact

# JSON output for CI automation
chart2kro plan ./my-chart/ --format json

# Plan with schema evolution analysis against an existing RGD
chart2kro plan ./my-chart/ --existing rgd.yaml

# Plan with existing + JSON output (pipe to jq)
chart2kro plan ./my-chart/ --existing rgd.yaml --format json | jq '.breakingChanges'

# Plan with custom values and profile
chart2kro plan ./my-chart/ -f production.yaml --profile enterprise

# CI gate — fail if breaking changes detected
chart2kro plan ./my-chart/ --existing rgd.yaml --format json \
  | jq -e '.breakingChangeCount == 0'
```

---

### `chart2kro watch`

Watch a chart for changes and auto-convert.

```
chart2kro watch <chart-reference> [flags]
```

Monitor a Helm chart directory for file changes and automatically re-run the conversion pipeline. Uses `fsnotify` for efficient OS-level file system notifications and recursively watches all chart files (templates, values, `Chart.yaml`, helpers, etc.).

**Watch-Specific Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--debounce <duration>` | `500ms` | Quiet period before triggering a rebuild after file changes |
| `--validate` | `true` | Auto-validate the generated RGD after each regeneration |
| `--apply` | `false` | Auto-apply the output to the cluster via `kubectl apply -f` |

The watch command inherits all **Chart Loading**, **Rendering**, **Values**, **Hook Handling**, **Resource Filtering**, **Transformation**, **Security Hardening**, and **Output** flags from the `convert` command.

**Behaviour:**

- File changes within the debounce window are coalesced into a single regeneration
- Each regeneration shows: timestamp, changed file, result (success/failure), resource and schema field counts
- Schema changes between generations are detected and summarised (fields added/removed, defaults changed)
- Hidden directories (`.git`) and editor temp files (`.swp`, `~`, `#`) are automatically ignored
- Graceful shutdown on `SIGINT` (Ctrl+C) or `SIGTERM`
- When `--apply` is used, `kubectl` must be on `PATH`; failed applies are reported with kubectl stderr
- Auto-apply is skipped when validation fails

**Examples:**

```bash
# Watch a chart directory and write output on each change
chart2kro watch ./my-chart/ -o rgd.yaml

# Watch with custom debounce interval
chart2kro watch ./my-chart/ -o rgd.yaml --debounce 1s

# Watch with auto-apply to the cluster
chart2kro watch ./my-chart/ -o rgd.yaml --apply

# Watch with custom values file
chart2kro watch ./my-chart/ -o rgd.yaml -f custom-values.yaml

# Watch with hardening enabled
chart2kro watch ./my-chart/ -o rgd.yaml --harden

# Disable auto-validation
chart2kro watch ./my-chart/ -o rgd.yaml --validate=false
```

---

### `chart2kro version`

Print version information.

```
chart2kro version [flags]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output version info as JSON |

**Examples:**

```bash
$ chart2kro version
chart2kro v1.0.0 (commit: abc1234, built: 2026-01-15, go1.24.0 darwin/arm64)

$ chart2kro version --json
{
  "version": "v1.0.0",
  "gitCommit": "abc1234",
  "buildDate": "2026-01-15",
  "goVersion": "go1.24.0",
  "platform": "darwin/arm64"
}
```

---

### `chart2kro completion`

Generate shell completion scripts.

```
chart2kro completion <shell>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `shell` | One of: `bash`, `zsh`, `fish`, `powershell` |

**Examples:**

```bash
# Bash
source <(chart2kro completion bash)

# Zsh
chart2kro completion zsh > "${fpath[1]}/_chart2kro"

# Fish
chart2kro completion fish > ~/.config/fish/completions/chart2kro.fish

# PowerShell
chart2kro completion powershell | Out-String | Invoke-Expression
```

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General error |
| `2` | Invalid arguments or configuration |
| `5` | Dependency cycle detected |
| `6` | Output write failure |
| `7` | Validation failure |
| `8` | Breaking schema changes detected (diff/plan) |
| `9` | Audit findings at or above threshold |

## See Also

- [Configuration Reference](configuration.md) — `.chart2kro.yaml` options
- [Transformation Pipeline](transformation-pipeline.md) — architecture and pipeline stages