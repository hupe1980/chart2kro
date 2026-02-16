<div align="center">

# ⚓ chart2kro

**Transform Helm charts into KRO ResourceGraphDefinitions**

[![CI](https://github.com/hupe1980/chart2kro/actions/workflows/ci.yaml/badge.svg)](https://github.com/hupe1980/chart2kro/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/hupe1980/chart2kro)](https://github.com/hupe1980/chart2kro/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/hupe1980/chart2kro)](https://goreportcard.com/report/github.com/hupe1980/chart2kro)
[![Go Version](https://img.shields.io/github/go-mod/go-version/hupe1980/chart2kro)](https://github.com/hupe1980/chart2kro/blob/main/go.mod)
[![Go Reference](https://pkg.go.dev/badge/github.com/hupe1980/chart2kro/pkg/chart2kro.svg)](https://pkg.go.dev/github.com/hupe1980/chart2kro/pkg/chart2kro)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0)

`chart2kro` reads a Helm chart, renders its templates, and produces a fully functional [KRO](https://kro.run) `ResourceGraphDefinition` (RGD) — turning your chart into a reusable, composable platform abstraction.

[Quick Start](#-quick-start) · [Go Library](#-go-library) · [Examples](examples/) · [Documentation](#-documentation)

</div>

---

## ✨ Features

| | Feature | Description |
|---|---------|-------------|
| 🔄 | **Convert** | Helm charts (local, OCI, repository) → KRO ResourceGraphDefinitions |
| 🔍 | **Inspect** | Preview resources, exposed values, and transformations before conversion |
| ✅ | **Validate** | Check generated RGDs against KRO schemas and Kubernetes conventions |
| 📤 | **Export** | Output as YAML, JSON, or directly apply via kubectl |
| 📊 | **Diff** | Detect drift and breaking schema changes against prior versions |
| 🛡️ | **Harden** | Apply Pod Security Standards, NetworkPolicies, RBAC, and SLSA provenance |
| 🔒 | **Audit** | Scan for security issues and best-practice violations |
| 📝 | **Docs** | Auto-generate documentation for the custom resource API |
| 📋 | **Plan** | Terraform-like dry-run with schema fields, resources, and evolution analysis |
| 👀 | **Watch** | Auto-re-convert on file changes with debouncing, validation, and auto-apply |
| 🔌 | **Extensible** | Transformer plugin system with built-in and config-based overrides |
| 📦 | **Go Library** | Embed chart2kro in your own tools via the `pkg/chart2kro` API |

---

## 📦 Installation

<details>
<summary><b>Go Install</b></summary>

```bash
go install github.com/hupe1980/chart2kro/cmd/chart2kro@latest
```

</details>

<details>
<summary><b>From Source</b></summary>

```bash
git clone https://github.com/hupe1980/chart2kro.git
cd chart2kro
just build   # or: CGO_ENABLED=0 go build -o chart2kro ./cmd/chart2kro/
```

</details>

<details>
<summary><b>Docker</b></summary>

```bash
docker run --rm ghcr.io/hupe1980/chart2kro version
```

</details>

---

## 🚀 Quick Start

```bash
# Convert a local Helm chart
chart2kro convert ./my-chart/

# Convert from an OCI registry
chart2kro convert oci://ghcr.io/org/my-chart:1.0.0

# Convert from a Helm repository
chart2kro convert my-chart --repo-url https://charts.example.com --version "^1.0.0"

# Customise Kind and API group
chart2kro convert ./my-chart/ --kind MyApp --group myapp.kro.run

# Save to file
chart2kro convert ./my-chart/ -o rgd.yaml

# Preview without writing
chart2kro convert ./my-chart/ --dry-run

# Pipe directly to kubectl
chart2kro convert ./my-chart/ | kubectl apply -f -
```

> 💡 Check out the [examples/](examples/) directory for ready-to-run charts including NGINX, Redis, and a production microservice.

---

## 🎯 Chart Sources

| Source | Example |
|--------|---------|
| 📁 Local directory | `chart2kro convert ./my-chart/` |
| 📦 Packaged archive | `chart2kro convert my-chart-1.0.0.tgz` |
| 🐳 OCI registry | `chart2kro convert oci://ghcr.io/org/chart:1.0.0` |
| 🌐 Helm repository | `chart2kro convert my-chart --repo-url https://charts.example.com` |

### Values & Rendering

Values are merged in the same order as `helm install`:

```bash
chart2kro convert ./my-chart/ \
  -f base-values.yaml \
  -f env-values.yaml \
  --set image.tag=v2.0.0 \
  --set-string annotations.commit=abc123 \
  --set-file config=./app.conf \
  --release-name myapp \
  --namespace production \
  --strict
```

---

## 🔧 Commands

> See [docs/cli-reference.md](docs/cli-reference.md) for the full CLI reference.

### `convert`

Convert a Helm chart to a KRO ResourceGraphDefinition.

```bash
chart2kro convert <chart-reference> [flags]
```

<details>
<summary>📋 All convert flags</summary>

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--repo-url` | | | Helm repository URL |
| `--version` | | | Chart version constraint |
| `--username` | | | Repository/registry username |
| `--password` | | | Repository/registry password |
| `--ca-file` | | | TLS CA certificate file |
| `--cert-file` | | | TLS client certificate file |
| `--key-file` | | | TLS client key file |
| `--release-name` | | `release` | Helm release name |
| `--namespace` | | `default` | Kubernetes namespace |
| `--strict` | | `false` | Fail on missing template values |
| `--timeout` | | `30s` | Rendering timeout |
| `--values` | `-f` | | Values YAML file (repeatable) |
| `--set` | | | Set values on the command line |
| `--set-string` | | | Set string values |
| `--set-file` | | | Set values from file content |
| `--include-hooks` | | `false` | Include hooks as regular resources |
| `--kind` | | | Custom Kind for the generated RGD |
| `--api-version` | | `v1alpha1` | Custom API version for the generated RGD |
| `--group` | | `kro.run` | Custom API group for the generated RGD |
| `--include-all-values` | | `false` | Include all values in schema, even unreferenced |
| `--flat-schema` | | `false` | Flatten nested values into camelCase fields |
| `--output` | `-o` | | Write output to file instead of stdout |
| `--dry-run` | | `false` | Show what would be generated without writing |
| `--comments` | | `false` | Add inline comments on CEL expressions |
| `--split` | | `false` | Write one file per resource (requires `--output-dir`) |
| `--output-dir` | | | Output directory for `--split` |
| `--embed-timestamp` | | `false` | Add `chart2kro.io/generated-at` annotation |
| `--harden` | | `false` | Enable security hardening |
| `--security-level` | | `restricted` | PSS level: `none`, `baseline`, `restricted` |
| `--generate-network-policies` | | `false` | Generate deny-all NetworkPolicies per workload |
| `--generate-rbac` | | `false` | Generate ServiceAccount/Role/RoleBinding per workload |
| `--resolve-digests` | | `false` | Resolve image tags to sha256 digests from registries |

</details>

### `inspect`

Preview a chart before converting it:

```bash
chart2kro inspect ./my-chart/
chart2kro inspect ./my-chart/ --format json
```

### `plan`

Terraform-like preview showing schema fields, resources, and status projections:

```bash
chart2kro plan ./my-chart/
chart2kro plan ./my-chart/ --existing rgd.yaml   # with evolution analysis
```

### `diff`

Detect drift and breaking schema changes:

```bash
chart2kro diff ./my-chart/ --existing rgd.yaml
chart2kro diff ./my-chart/ --existing rgd.yaml --format json   # CI-friendly
```

> Exit code `8` signals breaking changes — safe for CI/CD gates.

### `validate`

```bash
chart2kro validate rgd.yaml
chart2kro validate --strict rgd.yaml   # also fail on warnings
```

### `export`

```bash
chart2kro export rgd.yaml                                        # canonical YAML
chart2kro export rgd.yaml --format json                          # JSON
chart2kro export rgd.yaml --format kustomize --output-dir ./out  # Kustomize
```

### `audit`

```bash
chart2kro audit ./my-chart/
chart2kro audit ./my-chart/ --fail-on high --format sarif > results.sarif
```

### `docs`

```bash
chart2kro docs my-rgd.yaml
chart2kro docs my-rgd.yaml --format html -o api-reference.html
```

### `watch`

Auto-re-convert on file changes:

```bash
chart2kro watch ./my-chart/ -o rgd.yaml
chart2kro watch ./my-chart/ -o rgd.yaml --apply            # auto-apply to cluster
chart2kro watch ./my-chart/ -o rgd.yaml --debounce 1s      # custom debounce
```

---

## 🛡️ Security Hardening

```bash
# Enable hardening (PSS restricted + resource defaults + SLSA provenance)
chart2kro convert ./my-chart/ --harden -o rgd.yaml

# Full hardening with NetworkPolicy and RBAC generation
chart2kro convert ./my-chart/ --harden --generate-network-policies --generate-rbac

# Resolve image tags to sha256 digests
chart2kro convert ./my-chart/ --harden --resolve-digests
```

<details>
<summary>🔐 What <code>--harden</code> does</summary>

| Policy | Description |
|--------|-------------|
| Pod Security Standards | Enforces `runAsNonRoot`, `readOnlyRootFilesystem`, drops all capabilities, sets seccomp profile |
| Resource Requirements | Injects default CPU/memory requests and limits |
| Image Policy | Warns on `:latest` tags, unapproved registries, missing digests |
| Digest Resolution | Resolves image tags to sha256 digests from container registries |
| NetworkPolicy | Generates deny-all + DNS egress policies per workload |
| RBAC | Generates least-privilege ServiceAccount/Role/RoleBinding |
| Provenance | Adds SLSA v1.0 attestation annotations |

Customize via `.chart2kro.yaml`:

```yaml
harden:
  images:
    deny-latest-tag: true
    allowed-registries: ["gcr.io/", "quay.io/"]
  resources:
    cpu-request: "200m"
    memory-request: "256Mi"
```

</details>

---

## 🏗️ Resource Filtering & Profiles

For enterprise charts with many subcharts:

```bash
--exclude-kinds Secret,ConfigMap           # exclude by kind
--exclude-subcharts postgresql,redis       # exclude by subchart
--exclude-labels "component=database"      # exclude by label
--externalize-secret db-creds=externalDb   # externalize resources
--use-external-pattern postgresql          # smart patterns
--profile enterprise                       # preset filter bundles
```

Custom profiles can be defined in `.chart2kro.yaml`. See [docs/cli-reference.md](docs/cli-reference.md) for details.

---

## ⚙️ Transformation Pipeline

`chart2kro convert` executes a multi-phase pipeline:

```
Load & Render → Parse Resources → Analyze Dependencies → Filter & Externalize
     → Assign Resource IDs → Detect Parameters → Apply Field Mappings
     → Extract Schema → Build Dependency Graph → Generate Readiness & Status
     → Security Hardening (optional) → Assemble RGD
```

See [docs/transformation-pipeline.md](docs/transformation-pipeline.md) for the full architecture reference.

---

## 🔧 Configuration

Configuration is loaded from three sources (highest precedence first):

1. **CLI flags** — e.g. `--log-level debug`
2. **Environment variables** — e.g. `CHART2KRO_LOG_LEVEL=debug`
3. **Config file** — `.chart2kro.yaml` (auto-discovered in `.` or `$HOME/.config/chart2kro/`)

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `--config` | — | `.chart2kro.yaml` | Path to config file |
| `--log-level` | `CHART2KRO_LOG_LEVEL` | `info` | `debug` · `info` · `warn` · `error` |
| `--log-format` | `CHART2KRO_LOG_FORMAT` | `text` | `text` · `json` |
| `--no-color` | `CHART2KRO_NO_COLOR` | `false` | Disable colored output |
| `--quiet`, `-q` | `CHART2KRO_QUIET` | `false` | Suppress non-essential output |

See [docs/configuration.md](docs/configuration.md) for the full reference.

---

## � Go Library

chart2kro can be used as a Go library in your own tools. The public API lives in `pkg/chart2kro` and uses the functional options pattern.

### Install

```bash
go get github.com/hupe1980/chart2kro/pkg/chart2kro
```

### Basic Usage

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/chart2kro/pkg/chart2kro"
)

func main() {
	result, err := chart2kro.Convert(context.Background(), "./my-chart/")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(result.YAML))
}
```

### With Options

```go
result, err := chart2kro.Convert(ctx, "oci://ghcr.io/org/my-chart:1.0.0",
	chart2kro.WithReleaseName("my-release"),
	chart2kro.WithNamespace("production"),
	chart2kro.WithIncludeAllValues(),
	chart2kro.WithTimeout(60 * time.Second),
	chart2kro.WithSchemaOverrides(map[string]chart2kro.SchemaOverride{
		"replicaCount": {Type: "integer", Default: 3},
	}),
)
```

### Result

The `Result` struct provides:

| Field | Type | Description |
|-------|------|-------------|
| `YAML` | `[]byte` | Rendered RGD YAML |
| `RGDMap` | `map[string]interface{}` | Structured RGD for further manipulation |
| `ChartName` | `string` | Source chart name |
| `ChartVersion` | `string` | Source chart version |
| `ResourceCount` | `int` | Number of Kubernetes resources |
| `SchemaFieldCount` | `int` | Number of extracted schema parameters |
| `DependencyEdges` | `int` | Number of dependency edges in the graph |
| `HardenResult` | `*HardenSummary` | Hardening details (when enabled) |

See [docs/library-api.md](docs/library-api.md) for the full API reference.

---

## �🐚 Shell Completion

```bash
# Bash
source <(chart2kro completion bash)

# Zsh
chart2kro completion zsh > "${fpath[1]}/_chart2kro"

# Fish
chart2kro completion fish > ~/.config/fish/completions/chart2kro.fish
```

---

## 📚 Documentation

| Document | Description |
|----------|-------------|
| 📖 [CLI Reference](docs/cli-reference.md) | Complete command and flag reference |
| ⚙️ [Configuration](docs/configuration.md) | `.chart2kro.yaml` configuration reference |
| 🏗️ [Transformation Pipeline](docs/transformation-pipeline.md) | Architecture and pipeline stage details |
| 💡 [Examples](examples/) | Working examples with real Helm charts |
| 📦 [Library API](docs/library-api.md) | Go library API reference and examples |

---

## 📄 License

Apache 2.0 — see [LICENSE](LICENSE) for details.
