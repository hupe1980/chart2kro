# Transformation Pipeline

This document describes the core transformation pipeline that converts Helm charts into KRO ResourceGraphDefinitions (RGDs).

## Architecture

The pipeline is split into two layers:

| Layer | File | Description |
|-------|------|-------------|
| **Core pipeline** | `internal/cli/pipeline.go` | Shared chart→RGD pipeline (steps 1–10). Used by `convert`, `diff`, and `plan`. |
| **Command layer** | `internal/cli/convert.go` | Convert-specific post-processing: interactive summaries, timestamp/provenance annotations, serialization, and output handling. |

`runPipeline()` returns a `pipelineResult` containing the generated RGD map, transform result, chart metadata, hardening result, hook filter result, and resource filter result. Each command extracts what it needs:

- **convert** — serializes the RGD, prints summaries, writes output
- **diff** — serializes both old and new RGDs for unified diff and schema evolution analysis
- **plan** — formats the result as a table/compact/JSON preview

## Pipeline Overview

```
Helm Chart → Load → Render → Filter Hooks → Dependency Analysis → Parse YAML
    → Assign Resource IDs → Detect Parameters (Sentinel or Fast Mode)
    → Apply Field Mappings → Extract Schema (with optional JSON Schema enrichment)
    → Build Dependency Graph → Generate Status Projections
    → [Security Hardening (optional)]
    → Assemble RGD (with custom ready conditions)
    ──── convert-specific ────
    → Serialize (canonical YAML/JSON) → Output (stdout / file / split / kustomize)
```

## Pipeline Stages

### 1. Chart Loading & Rendering (BACKLOG 2)

Loads the Helm chart from any supported source (directory, archive, OCI, repository), merges values, and renders templates into raw Kubernetes YAML.

### 2. Hook Filtering (BACKLOG 2)

Identifies and removes Helm lifecycle hooks (pre-install, post-install, etc.) from the rendered output. Hooks are dropped by default but can be included with `--include-hooks`.

### 3. Subchart Dependency Analysis (BACKLOG 9)

When the chart declares dependencies (`Chart.yaml` `dependencies:`), the pipeline analyzes vendored subcharts for completeness before rendering.

**Package:** `internal/helm/deps`

- Checks that all declared dependencies are vendored in `charts/`
- Evaluates `condition` and `tags` fields to determine enabled/disabled subcharts
- Logs warnings for missing or unresolvable dependencies
- Does **not** block the pipeline — missing deps produce warnings, not errors

### 4. Resource Parsing

Splits the rendered multi-document YAML into individual Kubernetes resources. Each resource is decoded into an `unstructured.Unstructured` with GVK detection.

**Package:** `internal/k8s/parser`

- Splits on `---` document separators
- Decodes via `sigs.k8s.io/yaml`
- Extracts `apiVersion`, `kind`, and metadata
- Filters out empty documents and unknown types

### 5. Resource ID Assignment

Assigns stable, human-readable IDs to each resource.

**Package:** `internal/transform` (ids.go)

- Default ID: lowercase kind (e.g., `deployment`, `service`)
- Deduplication: suffix with name segment for multiple resources of the same kind
- Collision detection: returns an error if two resources produce the same ID
- Manual overrides via `--resource-id-overrides` config
- Invalid characters sanitized

### 6. Schema Extraction

Walks the Helm `values.yaml` tree and produces KRO SimpleSchema fields with inferred types and defaults.

**Package:** `internal/transform` (schema.go)

- Type inference: `bool` → `boolean`, `int/int64/float64` → `integer`/`number`, `string` → `string`
- SimpleSchema syntax: `integer | default=3`, `string | default="nginx"`
- Nested mode (default) preserves value hierarchy
- Flat mode (`--flat-schema`) produces camelCase field names
- `--include-all-values` includes unreferenced values
- **Schema overrides:** After extraction, `ApplySchemaOverrides` mutates fields in-place with user-specified types and defaults from `.chart2kro.yaml` `schemaOverrides:` (see [Configuration Reference](configuration.md#schemaoverrides))
- Flat mode (`--flat-schema`) produces camelCase field names
- `--include-all-values` includes unreferenced values

### 7. Parameter Detection (Sentinel Rendering)

Identifies which resource fields correspond to Helm values and generates CEL expressions for them.

**Package:** `internal/transform` (sentinel.go, params.go, fieldpath.go)

**How it works:**

1. **SentinelizeAll** — replaces ALL leaf values in the Helm values tree with string sentinel markers simultaneously (sentinel.go)
2. **Render** — re-renders the chart with the sentinel-injected values
3. **DiffAllResources** — diffs baseline vs sentinel-rendered resources using GVK+name matching to find all affected fields (sentinel.go)
4. **ExtractSentinelMappings** — extracts value paths from sentinel markers in changed fields (sentinel.go)
5. **ApplyFieldMappings** — replaces detected fields in resource templates with CEL expressions via `BuildCELExpression` (params.go)

**Module structure:**

| File | Purpose |
|------|---------|
| `sentinel.go` | Sentinel injection (`SentinelizeAll`), resource diffing (`DiffAllResources`), and mapping extraction |
| `params.go` | Field mapping types (`FieldMapping`, `MatchType`), CEL builders (`BuildCELExpression`, `BuildInterpolatedCELFromSentinel`), and `ApplyFieldMappings` |
| `fieldpath.go` | Dot-separated field path parsing (`parseFieldPath`) with bounds checking, nested map mutation (`setNestedField`), and path joining |

**Resource matching:** `DiffAllResources` matches baseline and sentinel-rendered resources by GVK+name (via `resourceMatchKey`) rather than positional index. This is robust against sentinel values changing conditional blocks and altering resource output order. Resources with zero-valued GVK (`Kind == ""`) are skipped to prevent false matches.

#### Parallel Diff

When the number of resources exceeds a small threshold, the pipeline automatically uses `ParallelDiffAllResources` to distribute resource diffing across a bounded worker pool (defaults to `GOMAXPROCS` goroutines). For small workloads (≤ 2 resources or 1 worker) it falls back to the sequential `DiffAllResources` to avoid goroutine overhead.

**Package:** `internal/transform` (parallel.go)

| Parameter | Default | Description |
|-----------|---------|-------------|
| `Workers` | `GOMAXPROCS` | Maximum concurrent diff goroutines |

`BatchSentinelizeIndependent` groups structurally independent leaf values (values under different top-level keys) so they can be sentinelized together in fewer rendering passes. Batches are capped at 8 top-level keys per pass.

**Value comparison:** `diffForSentinels` uses `reflect.DeepEqual` for type-aware comparison, correctly detecting changes when types differ (e.g., `int64(3)` vs `float64(3)`). Non-string sentinel values are stringified via `Sprintf("%v")` before extraction.

**Sentinel pattern:** All value types (string, integer, boolean) use the same string sentinel format:

| Pattern | Example |
|---------|---------|
| `__CHART2KRO_SENTINEL_<path>__` | `__CHART2KRO_SENTINEL_image.tag__` |

Using a single sentinel type for all values ensures consistent detection and supports multi-value string interpolation (e.g., `image: nginx:1.21` becomes `image: __CHART2KRO_SENTINEL_image.repository__:__CHART2KRO_SENTINEL_image.tag__`).

**Match types:**

| Type | Description | CEL Output |
|------|-------------|------------|
| Exact | Entire field value matches a single sentinel | `${schema.spec.replicaCount}` |
| Substring | Multiple sentinels or sentinel mixed with literals | `${schema.spec.image.repository}:${schema.spec.image.tag}` |

### 7b. Fast Mode (`--fast`)

An alternative to sentinel rendering that uses Go template AST analysis for O(1) renders.

**Package:** `internal/transform` (ast.go)

**How it works:**

1. **AnalyzeTemplates** — parses template files using `text/template/parse` and walks the AST to find all `.Values.*` field references
2. **MatchFieldsByValue** — walks rendered resources and matches field values against known Helm values to produce `FieldMapping` entries
3. **buildFastModeSentinel** — creates synthetic sentinel strings for substring matches, enabling reuse of `BuildInterpolatedCELFromSentinel`

**Trade-offs:**

| Aspect | Sentinel Mode | Fast Mode |
|--------|---------------|-----------|
| Renders | 2 (baseline + sentinel) | 1 (baseline only) |
| Accuracy | High (sentinel markers are unique) | Heuristic (value matching may miss transforms) |
| Complex expressions | Detected via sentinel substrings | May miss template-transformed values |
| Use case | Default, most accurate | Large charts where render time dominates |

Enable with `--fast` flag. For simple charts, fast mode produces identical output to sentinel mode.

### 7c. JSON Schema Enrichment

When a chart includes `values.schema.json`, schema extraction is enriched with explicit type information.

**Package:** `internal/transform` (jsonschema.go)

- `JSONSchemaResolver` parses the JSON Schema and resolves type info for any values path
- Schema extraction checks JSON Schema first, falls back to Go type inference
- Supports `type`, `format`, `description`, `enum`, `minimum`, `maximum` properties
- Type mapping: `integer` → `integer`, `number` → `number`, `boolean` → `boolean`, `array` → `array`, `object` → `object`, `string` → `string`

### 8. Dependency Graph

Analyzes cross-resource references and builds a DAG with topological ordering.

**Package:** `internal/transform` (deps.go)

**Detected dependency types:**
- Label selectors (Service → Deployment)
- Name references (Deployment → ConfigMap/Secret)
- ServiceAccount references
- Volume references (PVC, ConfigMap, Secret volumes)
- Environment variable references (configMapKeyRef, secretKeyRef) — scans both `containers` and `initContainers`

**Edge validation:** Self-references are ignored, and edges to non-existent nodes are silently skipped.

**Topological sort** uses Kahn's algorithm with deterministic alphabetical tie-breaking (O(n log n) binary-search insertion). Cycles are detected and reported as errors (exit code 5). `DetectCycles` deduplicates cycles by normalizing each cycle (rotating to the lexicographically smallest node) to avoid reporting the same cycle multiple times from different DFS starting points.

### 9. CEL Expression Generation

Generates KRO-compatible CEL expressions for schema references, cross-resource references, readiness conditions, and status projections.

**Package:** `internal/transform` (cel.go)

> **Design decision:** chart2kro uses plain string builders — not `cel-go` or `kro/pkg/cel`.
> KRO's `${…}` template syntax is a layer above standard CEL that `cel-go` cannot parse.
> `kro/pkg/cel` requires `rest.Config` for server-side evaluation. chart2kro is an offline
> code generator that produces simple field references and comparisons; adding `cel-go` would
> bloat the binary by ~8 MB for zero benefit. See [ADR-001](adr/001-no-kro-pkg-dependency.md)
> for the full rationale.
>
> `ValidateExpression()` provides lightweight syntax validation (balanced `${…}` delimiters)
> without a full CEL parser. Semantic validation is KRO's responsibility at apply time.

| Category | Pattern | Example |
|----------|---------|---------|
| Schema reference | `${schema.spec.<path>}` | `${schema.spec.replicas}` |
| Cross-resource | `${<resourceId>.<path>}` | `${deployment.spec.selector.matchLabels}` |
| Self reference | `${self.<path>}` | `${self.status.availableReplicas}` |
| Interpolation | `"${a}:${b}"` | `"${schema.spec.image.repo}:${schema.spec.image.tag}"` |
| IncludeWhen | `${schema.spec.<bool>}` | `${schema.spec.ingress.enabled}` |

**Default readyWhen conditions:**

| Kind | Condition |
|------|-----------|
| Deployment | `${self.status.availableReplicas == self.status.replicas}` |
| StatefulSet | `${self.status.readyReplicas == self.status.replicas}` |
| DaemonSet | `${self.status.numberReady == self.status.desiredNumberScheduled}` |
| Service | `${self.spec.clusterIP != ""}` |
| Job | `${self.status.succeeded > 0}` |
| PVC | `${self.status.phase == "Bound"}` |

Custom readiness conditions can be supplied via `--ready-conditions <file.yaml>`:

```yaml
Deployment:
  - "${self.status.readyReplicas == self.spec.replicas}"
Service:
  - "${self.status.loadBalancer.ingress[0].ip != \"\"}"
```

**Compound includeWhen** conditions join multiple checks with `&&`:

```
${schema.spec.serviceEnabled && schema.spec.servicePort != ""}
```

**Optional accessor `?`** is used for potentially absent fields in status projections:

```
${service.status.loadBalancer.?ingress[0].?ip}
```

This prevents CEL errors when intermediate path segments (like `ingress[0]`) don't exist.

### 9b. Transformer Registry

The transformation engine dispatches readiness conditions and status projections through a pluggable `TransformerRegistry`. Each resource is matched against registered transformers by GVK; the first matching transformer produces the `readyWhen` and `statusFields` for that resource. When no transformer matches, the engine falls back to `DefaultStatusProjections`.

**Packages:** `internal/transform` (transformer_iface.go), `internal/transform/transformer`

| File | Purpose |
|------|---------|
| `transformer_iface.go` | Engine-facing `TransformerRegistry` interface (avoids circular imports) |
| `transformer/transformer.go` | `Transformer` interface and `TransformInput`/`TransformOutput` types |
| `transformer/registry.go` | `Registry` with `Register`, `Prepend`, `TransformResource`, `DefaultRegistry` |
| `transformer/builtin.go` | Built-in transformers: Deployment, StatefulSet, DaemonSet, Service |
| `transformer/config_override.go` | Bridge from `.chart2kro.yaml` `transformers:` entries to the `Transformer` interface |

**Built-in transformers:**

| Transformer | Matches | readyWhen |
|-------------|---------|-----------|
| `DeploymentTransformer` | `Deployment` | `availableReplicas == replicas` |
| `StatefulSetTransformer` | `StatefulSet` | `readyReplicas == replicas` |
| `DaemonSetTransformer` | `DaemonSet` | `numberReady == desiredNumberScheduled` |
| `ServiceTransformer` | `Service` | `clusterIP != ""` |

Config-based overrides from `.chart2kro.yaml` are prepended to the registry so they take priority over built-ins. See the [Configuration Reference](configuration.md#transformers) for the config syntax.

### 10. RGD Assembly

Assembles all transformation artifacts into a complete KRO `ResourceGraphDefinition`.

**Package:** `internal/kro` (rgd.go)

- `apiVersion: kro.run/v1alpha1`
- `kind: ResourceGraphDefinition`
- Metadata with labels and annotations
- Schema with SimpleSchema spec and CEL status projections
- Resources ordered by topological sort with `readyWhen`, `includeWhen`, and `dependsOn`

### 10b. Security Hardening (optional)

When `--harden` is enabled, the hardening engine applies security policies to the transformed resources before RGD assembly.

**Package:** `internal/harden`

| File | Purpose |
|------|---------|
| `harden.go` | Orchestrator, types, `Policy` interface, `Hardener`, config file parsing |
| `pss.go` | Pod Security Standards (baseline/restricted) enforcement |
| `resources.go` | Default resource requirements injection |
| `image.go` | Image policy enforcement (latest tag, registry allowlist, digest requirement) |
| `digest.go` | Image digest resolution (resolves tags to sha256 digests from registries) |
| `netpol.go` | NetworkPolicy generation (deny-all + DNS egress + service ingress) |
| `rbac.go` | ServiceAccount + Role + RoleBinding generation |
| `provenance.go` | SLSA v1.0 provenance annotations |

**Policy execution order:**

1. **Pod Security Standards** — Non-destructive merge of security contexts (preserves existing settings)
2. **Resource Requirements** — Injects CPU/memory defaults where missing
3. **Image Policy** — Validates images and emits warnings
4. **Digest Resolution** — Resolves image tags to sha256 digests (`--resolve-digests`)
5. **NetworkPolicy Generation** — Creates deny-all + DNS egress policies per workload
6. **RBAC Generation** — Creates least-privilege ServiceAccount/Role/RoleBinding per workload

**PSS enforcement levels:**

| Level | Enforced Settings |
|-------|-------------------|
| `restricted` | `runAsNonRoot`, `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `seccompProfile: RuntimeDefault` |
| `baseline` | No `hostNetwork/hostPID/hostIPC`, no `privileged`, drops dangerous capabilities (e.g., `SYS_ADMIN`, `NET_RAW`) |
| `none` | No PSS enforcement |

**Non-destructive merge:** Existing security settings are never overwritten. Conflicts generate warnings (e.g., `privileged: true` with restricted level).

**Provenance:** When hardening is enabled, SLSA v1.0 provenance annotations are added to the RGD metadata, including build attestation, chart source, and hardening parameters.

## Output Format

The generated RGD follows the `kro.run/v1alpha1` API:

```yaml
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: <chart-name>
  labels:
    app.kubernetes.io/managed-by: chart2kro
    app.kubernetes.io/name: <chart-name>
    app.kubernetes.io/version: <chart-version>
  annotations:
    chart2kro.dev/generated: "true"
spec:
  schema:
    apiVersion: <name>.kro.run/v1alpha1
    kind: <PascalCaseName>
    spec:
      replicaCount: integer | default=1
      image:
        repository: string | default="nginx"
        tag: string | default="1.25"
    status:
      availableReplicas: ${deployment.status.availableReplicas}
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        spec:
          replicas: ${schema.spec.replicaCount}
          template:
            spec:
              containers:
              - image: ${schema.spec.image.repository}:${schema.spec.image.tag}
      readyWhen:
        - ${self.status.availableReplicas == self.status.replicas}
      dependsOn:
        - configmap
```

## GVK Classification

The `internal/k8s` package provides utilities to classify resources:

| Function | Matching Kinds |
|----------|---------------|
| `IsWorkload(gvk)` | Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, CronJob |
| `IsService(gvk)` | Service |
| `IsConfig(gvk)` | ConfigMap, Secret |
| `IsStorage(gvk)` | PVC, PV |
| `IsNetworking(gvk)` | Ingress, NetworkPolicy |
| `IsCRD(gvk)` | CustomResourceDefinition |
| `IsRBAC(gvk)` | Role, ClusterRole, RoleBinding, ClusterRoleBinding |
| `IsServiceAccount(gvk)` | ServiceAccount |

Individual kind classifiers (used by CEL expression builders):

| Function | Matching Kind |
|----------|--------------|
| `IsDeployment(gvk)` | Deployment (apps group) |
| `IsStatefulSet(gvk)` | StatefulSet (apps group) |
| `IsDaemonSet(gvk)` | DaemonSet (apps group) |
| `IsJob(gvk)` | Job (batch group) |
| `IsPVC(gvk)` | PersistentVolumeClaim |
| `APIVersion(gvk)` | Converts GVK to apiVersion string (e.g., `apps/v1`, `v1`) |

## Output Stage (BACKLOG 4)

After the RGD is assembled, the output stage serializes and writes the result.

**Package:** `internal/output`

### Canonical YAML Serializer

- Alphabetically sorted map keys (deterministic output)
- `apiVersion`, `kind`, `metadata`, `spec` appear in natural order (alphabetical sort matches)
- Null values and empty maps stripped
- Consistent 2-space indentation
- Optional inline CEL expression comments (`--comments`)
- Same input always produces byte-identical output (verified with 10-run determinism test)

### Output Writers & Registry

Writers abstract the output destination. A `WriterFactory` creates a `Writer` for a given output path. The `Registry` maps format names to `WriterFactory` functions, enabling pluggable output formats.

| Writer | Description |
|--------|-------------|
| `StdoutWriter` | Writes to stdout (default) |
| `FileWriter` | Writes to file with 0644 permissions, creates parent dirs |
| Split mode | `--split --output-dir` writes one file per resource + `kustomization.yaml` |

`DefaultRegistry()` pre-populates: `yaml`, `json`, `stdout`, `file`. The `export` command looks up writers via the registry for format-specific output. Custom formats can be registered at runtime via `Registry.Register(name, factory)`. `Registry.AvailableFormats()` returns a sorted, comma-separated string of registered format names for use in error messages and help text.

### Export Formats

| Format | Flag | Description |
|--------|------|-------------|
| `yaml` | `--format yaml` | Re-serialized canonical YAML |
| `json` | `--format json` | JSON with sorted keys and indentation |
| `kustomize` | `--format kustomize` | RGD YAML + `kustomization.yaml` in a directory |

### Validation (`chart2kro validate`)

Validates a generated RGD file for correctness:
- YAML syntax
- Required fields (`apiVersion`, `kind`, `metadata.name`, `spec.schema`, `spec.resources`)
- Schema field types (valid SimpleSchema syntax)
- Schema `kind` PascalCase
- CEL references (`${schema.spec.*}`, `${<resourceId>.*}`, `${self.*}`)
- Unique resource IDs
- Dependency graph cycles
- `--strict` fails on warnings
