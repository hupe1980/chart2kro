# Go Library API

The `pkg/chart2kro` package exposes the full chart2kro conversion pipeline as a Go library, allowing you to embed Helm-to-KRO conversion in your own tools without shelling out to the CLI.

## Install

```bash
go get github.com/hupe1980/chart2kro/pkg/chart2kro
```

## Quick Start

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

	fmt.Printf("Chart: %s (%s)\n", result.ChartName, result.ChartVersion)
	fmt.Printf("Resources: %d, Schema fields: %d\n", result.ResourceCount, result.SchemaFieldCount)
	fmt.Println(string(result.YAML))
}
```

## API Reference

### `Convert`

```go
func Convert(ctx context.Context, chartRef string, opts ...Option) (*Result, error)
```

Transforms a Helm chart reference into a KRO ResourceGraphDefinition.

**Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `ctx` | `context.Context` | Context for cancellation and timeouts |
| `chartRef` | `string` | Chart reference: local path, `.tgz`, OCI URL, or chart name |
| `opts` | `...Option` | Functional options (all optional) |

**Chart reference formats:**

| Format | Example |
|--------|---------|
| Local directory | `"./my-chart/"` |
| Packaged archive | `"my-chart-1.0.0.tgz"` |
| OCI registry | `"oci://ghcr.io/org/my-chart:1.0.0"` |
| Helm repository | `"my-chart"` (with `WithRepoURL`) |

### `Result`

```go
type Result struct {
	YAML             []byte                 // rendered RGD YAML
	RGDMap           map[string]interface{} // structured RGD as a map
	ChartName        string                 // source chart name
	ChartVersion     string                 // source chart version
	ResourceCount    int                    // number of K8s resources
	SchemaFieldCount int                    // number of schema parameters
	DependencyEdges  int                    // number of dependency edges
	HardenResult     *HardenSummary         // hardening details (nil if disabled)
}
```

### `SchemaOverride`

```go
type SchemaOverride struct {
	Type    string      // JSON Schema type: "string", "integer", "boolean", "number"
	Default interface{} // default value
}
```

### `HardenSummary`

```go
type HardenSummary struct {
	Changes  int      // number of hardening changes applied
	Warnings []string // hardening warnings
}
```

## Options

All configuration is done via functional options passed to `Convert`. Zero options gives sensible defaults.

### Chart Loading

| Option | Description |
|--------|-------------|
| `WithVersion(v string)` | Chart version constraint |
| `WithRepoURL(url string)` | Helm repository URL |
| `WithUsername(u string)` | Repository/registry username |
| `WithPassword(p string)` | Repository/registry password |
| `WithCaFile(f string)` | TLS CA certificate file |
| `WithCertFile(f string)` | TLS client certificate file |
| `WithKeyFile(f string)` | TLS client key file |

### Template Rendering

| Option | Description |
|--------|-------------|
| `WithReleaseName(name string)` | Helm release name (default: `"release"`) |
| `WithNamespace(ns string)` | Kubernetes namespace (default: `"default"`) |
| `WithStrict()` | Fail on missing template values |
| `WithTimeout(d time.Duration)` | Rendering timeout (default: `30s`) |

### Values Merging

| Option | Description |
|--------|-------------|
| `WithValueFiles(files []string)` | Paths to additional values files |
| `WithValues(vals []string)` | Value overrides (`key=value`) |
| `WithStringValues(vals []string)` | String value overrides (`key=value`) |
| `WithFileValues(vals []string)` | File value overrides (`key=filepath`) |

### Schema Generation

| Option | Description |
|--------|-------------|
| `WithKind(kind string)` | Override generated CRD kind |
| `WithAPIVersion(v string)` | Override schema apiVersion (default: `"v1alpha1"`) |
| `WithGroup(g string)` | Override schema group (default: `"kro.run"`) |
| `WithIncludeAllValues()` | Include all values, even unreferenced |
| `WithFlatSchema()` | Flatten nested values into camelCase fields |

### Filtering

| Option | Description |
|--------|-------------|
| `WithExcludeKinds(kinds []string)` | Exclude resources by Kind |
| `WithExcludeResources(names []string)` | Exclude resources by name pattern |
| `WithExcludeSubcharts(subs []string)` | Exclude subchart resources |
| `WithExcludeLabels(selector string)` | Exclude resources by label selector |
| `WithExternalizeSecret(names []string)` | Externalize Secrets by name |
| `WithExternalizeService(names []string)` | Externalize Services by name |
| `WithUseExternalPattern(patterns []string)` | Regex patterns for external refs |
| `WithProfile(p string)` | Predefined filter profile |
| `WithIncludeHooks()` | Include Helm hook resources |

### Security Hardening

| Option | Description |
|--------|-------------|
| `WithHarden()` | Enable security hardening |
| `WithSecurityLevel(level string)` | PSS level: `"restricted"`, `"baseline"`, `"none"` |
| `WithGenerateNetworkPolicies()` | Generate NetworkPolicy resources |
| `WithGenerateRBAC()` | Generate RBAC resources |
| `WithResolveDigests()` | Resolve image tags to sha256 digests |

### Advanced

| Option | Description |
|--------|-------------|
| `WithReadyConditions(path string)` | Path to custom ready conditions file |
| `WithResourceIDOverrides(map[string]string)` | Override auto-assigned resource IDs |
| `WithSchemaOverrides(map[string]SchemaOverride)` | Override inferred schema field types/defaults |
| `WithTransformConfigData(data []byte)` | Raw `.chart2kro.yaml` bytes for transformer overrides |
| `WithFast()` | Use template AST analysis (faster, less accurate) |

## Examples

### Convert from OCI Registry

```go
result, err := chart2kro.Convert(ctx, "oci://ghcr.io/org/my-chart:1.0.0",
	chart2kro.WithReleaseName("prod"),
	chart2kro.WithNamespace("production"),
)
```

### Convert from Helm Repository

```go
result, err := chart2kro.Convert(ctx, "nginx",
	chart2kro.WithRepoURL("https://charts.bitnami.com/bitnami"),
	chart2kro.WithVersion(">=18.0.0"),
)
```

### Custom Schema and Hardening

```go
result, err := chart2kro.Convert(ctx, "./my-chart/",
	chart2kro.WithKind("MyApp"),
	chart2kro.WithGroup("myapp.example.com"),
	chart2kro.WithIncludeAllValues(),
	chart2kro.WithHarden(),
	chart2kro.WithSecurityLevel("restricted"),
	chart2kro.WithGenerateNetworkPolicies(),
	chart2kro.WithSchemaOverrides(map[string]chart2kro.SchemaOverride{
		"replicaCount": {Type: "integer", Default: 3},
		"image.tag":    {Type: "string", Default: "latest"},
	}),
)
```

### Filtering Enterprise Charts

```go
result, err := chart2kro.Convert(ctx, "./enterprise-chart/",
	chart2kro.WithExcludeSubcharts([]string{"postgresql", "redis"}),
	chart2kro.WithExcludeKinds([]string{"PodDisruptionBudget"}),
	chart2kro.WithExternalizeSecret([]string{"db-creds=externalDb"}),
)
```

### Working with the Result

```go
result, err := chart2kro.Convert(ctx, "./my-chart/")
if err != nil {
	log.Fatal(err)
}

// Write YAML to file
os.WriteFile("rgd.yaml", result.YAML, 0o644)

// Inspect the RGD structure
spec := result.RGDMap["spec"].(map[string]interface{})
schema := spec["schema"].(map[string]interface{})
fmt.Printf("Schema kind: %s\n", schema["kind"])

// Check hardening results
if result.HardenResult != nil {
	fmt.Printf("Applied %d hardening changes\n", result.HardenResult.Changes)
	for _, w := range result.HardenResult.Warnings {
		fmt.Printf("Warning: %s\n", w)
	}
}
```

## Design Decisions

- **Functional options pattern** — zero config works, options compose cleanly, and the API is forward-compatible (new options don't break existing callers).
- **No logging** — the library discards all log output. Callers control their own logging.
- **Context-aware** — all operations respect context cancellation and timeouts.
- **Offline-first** — no cluster connection required. OCI/repo fetching uses standard HTTP.

See [ADR-001](adr/001-no-kro-pkg-dependency.md) for why chart2kro does not import `kubernetes-sigs/kro/pkg`.
