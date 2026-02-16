# Configuration Reference

`chart2kro` loads configuration from three sources with the following precedence (highest to lowest):

1. **CLI flags** — always take the highest priority
2. **Environment variables** — `CHART2KRO_` prefix, underscores replace hyphens
3. **Config file** — `.chart2kro.yaml` auto-discovered or specified via `--config`

## Config File Locations

When `--config` is not specified, `chart2kro` searches for `.chart2kro.yaml` in:

1. Current working directory (`.`)
2. `$HOME/.config/chart2kro/`

If no config file is found, all settings use their default values.

## Settings

### `log-level`

Controls the verbosity of log output.

| | |
|-|-|
| **Type** | `string` |
| **Default** | `info` |
| **Valid values** | `debug`, `info`, `warn`, `error` |
| **Flag** | `--log-level <level>` |
| **Env** | `CHART2KRO_LOG_LEVEL` |

**Example:**

```yaml
# .chart2kro.yaml
log-level: debug
```

---

### `log-format`

Controls the format of log output.

| | |
|-|-|
| **Type** | `string` |
| **Default** | `text` |
| **Valid values** | `text`, `json` |
| **Flag** | `--log-format <format>` |
| **Env** | `CHART2KRO_LOG_FORMAT` |

Use `json` for CI environments and log aggregation systems.

**Example:**

```yaml
# .chart2kro.yaml
log-format: json
```

---

### `no-color`

Disables ANSI colored output.

| | |
|-|-|
| **Type** | `bool` |
| **Default** | `false` |
| **Flag** | `--no-color` |
| **Env** | `CHART2KRO_NO_COLOR` |

**Example:**

```yaml
# .chart2kro.yaml
no-color: true
```

---

### `quiet`

Suppresses all log output below `error` level, regardless of the `log-level` setting.

| | |
|-|-|
| **Type** | `bool` |
| **Default** | `false` |
| **Flag** | `--quiet` / `-q` |
| **Env** | `CHART2KRO_QUIET` |

**Example:**

```yaml
# .chart2kro.yaml
quiet: true
```

## Full Example

```yaml
# .chart2kro.yaml
log-level: info
log-format: text
no-color: false
quiet: false

# Security hardening (applied when --harden is used)
harden:
  security-level: restricted
  generate-network-policies: true
  generate-rbac: false
  images:
    deny-latest-tag: true
    allowed-registries:
      - gcr.io/
      - docker.io/library/
    require-digests: false
  resources:
    cpu-request: "100m"
    memory-request: "128Mi"
    cpu-limit: "500m"
    memory-limit: "512Mi"
```

---

## Security Hardening Configuration

When `--harden` is enabled on the CLI, image and resource policies can be configured in the config file under the `harden:` section.

### `harden.images`

Controls image policy warnings.

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `deny-latest-tag` | `bool` | `false` | Warn on `:latest` or untagged images |
| `allowed-registries` | `[]string` | `[]` | Allowed image registry prefixes |
| `require-digests` | `bool` | `false` | Warn when images don't use `@sha256:` digests |

### `harden.resources`

Default resource requirements injected into containers that don't have them.

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `cpu-request` | `string` | `"100m"` | Default CPU request |
| `memory-request` | `string` | `"128Mi"` | Default memory request |
| `cpu-limit` | `string` | `"500m"` | Default CPU limit |
| `memory-limit` | `string` | `"512Mi"` | Default memory limit |
| `require-limits` | `bool` | `false` | Error if a container has no resource limits and no defaults configured |

**Example:**

```yaml
# .chart2kro.yaml
harden:
  images:
    deny-latest-tag: true
    allowed-registries:
      - gcr.io/
      - quay.io/
    require-digests: true
  resources:
    cpu-request: "200m"
    memory-request: "256Mi"
    cpu-limit: "1"
    memory-limit: "1Gi"
    require-limits: false
```

## Precedence Examples

```bash
# Flag wins over env and config file:
CHART2KRO_LOG_LEVEL=debug chart2kro convert ./chart/ --log-level error
# → effective log-level: error

# Env wins over config file:
CHART2KRO_LOG_LEVEL=debug chart2kro convert ./chart/
# → effective log-level: debug (even if .chart2kro.yaml says info)

# Config file applies when no flag or env is set:
# .chart2kro.yaml contains log-level: warn
chart2kro convert ./chart/
# → effective log-level: warn
```

## Custom Conversion Profiles

Custom conversion profiles can be defined in `.chart2kro.yaml` under the `profiles:` key. Profiles bundle filter/exclusion settings that can be activated with `--profile <name>`.

### Profile Structure

```yaml
# .chart2kro.yaml
profiles:
  my-profile:
    excludeSubcharts:
      - postgresql
      - redis
    excludeKinds:
      - PodDisruptionBudget
      - NetworkPolicy
    externalResources:
      - "Secret:db-credentials=externalDatabaseSecret"
      - "Service:redis-master=externalRedisService"
```

### Extending Built-in Profiles

Custom profiles can inherit from built-in profiles using the `extends` field:

```yaml
profiles:
  production:
    extends: enterprise
    excludeKinds:
      - NetworkPolicy
```

The `extends` field merges the parent profile's settings with the custom profile's settings. The custom profile's settings take precedence for overlapping fields, while lists are concatenated.

### Built-in Profiles

Three built-in profiles are available without configuration:

| Profile | Description |
|---------|-------------|
| `enterprise` | Excludes common infrastructure subcharts: `postgresql`, `redis`, `memcached`, `elasticsearch`, `kafka`, `rabbitmq`, `mongodb`, `mysql` |
| `minimal` | Excludes **all** subchart resources, keeping only the root chart |
| `app-only` | Excludes `StatefulSet` and `PersistentVolumeClaim` kinds (stateless applications only) |

### External Resources Format

External resources use the format `Kind:name=schemaField`:

- `Kind` — Kubernetes resource kind (e.g., `Secret`, `Service`)
- `name` — resource name to externalize
- `schemaField` — schema field name for the external reference

Example: `Secret:db-creds=externalDatabaseSecret` removes the `db-creds` Secret and adds `externalDatabaseSecret` as a schema field. Other resources referencing `db-creds` are rewired to use `${schema.spec.externalDatabaseSecret}`.

---

## Transformation Extensibility

The `transformers:`, `schemaOverrides:`, and `resourceIdOverrides:` sections allow you to customise the conversion pipeline without writing Go code. Config-based overrides take priority over built-in transformer defaults.

### `transformers`

Override readiness conditions and status projections for specific resource kinds. Each entry matches resources by `kind` (required) and optionally `apiVersion`.

```yaml
# .chart2kro.yaml
transformers:
  - match:
      kind: Deployment
      apiVersion: apps/v1
    readyWhen:
      - "${deployment.status.readyReplicas == deployment.spec.replicas}"
    statusFields:
      - name: ready
        celExpression: "${deployment.status.readyReplicas}"
      - name: available
        celExpression: "${deployment.status.availableReplicas}"

  - match:
      kind: StatefulSet
    readyWhen:
      - "${statefulSet.status.readyReplicas == statefulSet.spec.replicas}"
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `match.kind` | `string` | Yes | Kubernetes resource Kind to match |
| `match.apiVersion` | `string` | No | API version to match (e.g., `apps/v1`) |
| `readyWhen` | `[]string` | No | Custom readiness condition CEL expressions |
| `statusFields` | `[]object` | No | Custom status field projections |
| `statusFields[].name` | `string` | Yes | Status field name |
| `statusFields[].celExpression` | `string` | Yes | CEL expression for the status value |

### `schemaOverrides`

Override inferred schema field types and defaults. Keys are dotted paths corresponding to Helm value paths.

```yaml
# .chart2kro.yaml
schemaOverrides:
  replicaCount:
    type: integer
    default: "3"
  image.tag:
    type: string
    default: "latest"
  service.port:
    type: integer
    default: "8080"
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | `string` | No | Schema type: `string`, `integer`, `boolean`, `number`, `object`, `array` |
| `default` | `string` | No | Default value for the schema field |

### `resourceIdOverrides`

Override the automatically assigned resource IDs. Keys are the original generated ID; values are the desired replacement ID.

```yaml
# .chart2kro.yaml
resourceIdOverrides:
  deploymentMyApp: appDeployment
  serviceMyApp: appService
  configMapEnv: envConfig
```

Resource IDs are used in dependency references and CEL expressions. Renaming an ID via overrides automatically applies throughout the generated RGD.

### Full Extensibility Example

```yaml
# .chart2kro.yaml
log-level: info

transformers:
  - match:
      kind: Deployment
    readyWhen:
      - "${deployment.status.readyReplicas == deployment.spec.replicas}"
    statusFields:
      - name: ready
        celExpression: "${deployment.status.readyReplicas}"

schemaOverrides:
  replicaCount:
    type: integer
    default: "3"

resourceIdOverrides:
  deploymentMyApp: appServer

profiles:
  production:
    extends: enterprise
    excludeKinds:
      - PodDisruptionBudget
```
