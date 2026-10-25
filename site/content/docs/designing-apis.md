+++
title = "Designing a platform API"
description = "Turn a chart with hundreds of values into a small, stable kro API: platform defaults with -f, strict --expose, your own API group, trimmed resources, and CI checks."
weight = 30

[extra]
group = "Guides"
nav = "Designing APIs"
+++

A converted chart exposes every value that can be verified, often dozens. A good platform API exposes a handful. This guide shows how to get there with Helm's own flags plus a few of chart2kro's.

## 1. See what the chart offers

```bash
chart2kro inspect ./mychart
chart2kro inspect ./mychart --format json | jq -r '.values[] | select(.outcome=="exposed") | .path'
```

## 2. Set your platform's opinions with `-f`

Values from `-f` and `--set` become the API's **defaults**, exactly as in `helm install`:

```yaml
# platform.yaml
image:
  registry: registry.example.com
resources:
  requests: { cpu: 100m, memory: 128Mi }
podSecurityContext:
  runAsNonRoot: true
```

Opinions that should never change belong here. Values that are frozen (for example because they add fields) can still be set this way; they just cannot be per-instance inputs.

## 3. Choose the inputs with `--expose`

```bash
chart2kro convert ./mychart -f platform.yaml \
  --expose 'replicaCount,image.tag,ingress.enabled,ingress.hosts,resources' \
  -o api/
```

| Pattern | Matches |
|---|---|
| `image.tag` | That value |
| `resources` | The value and everything below it (`resources.limits.cpu`, …) |
| `ingress.*` | One level: `ingress.enabled`, `ingress.className`, … |
| `**.tag` | `tag` at any depth |
| `image.[rt]*` | Shell-style classes per segment (`repository`, `tag`) |

`--expose` is **strict**: if a listed value cannot be a verified input, the conversion fails with exit 5 and says why. Use `--freeze` for the opposite: everything except some values.

## 4. Name the API

```bash
--group apps.example.com --kind WebApp --api-version v1alpha1
```

The defaults are `kro.run`, the chart name in PascalCase, and `v1alpha1`. Use your own group: two teams converting the same chart into `kro.run` would collide on the same CRD. kro does not allow changing kind, group or version of an existing RGD.

## 5. Trim what instances do not need

| Need | Do |
|---|---|
| Use an external database instead of the bundled one | The chart's own switches: `--set postgresql.enabled=false --set externalDatabase.host=db.prod` |
| Drop a whole subchart | `--exclude-subchart postgresql` |
| Drop one resource | `--exclude <id>` with an id from `inspect` |
| Keep Helm hooks | `--include-hooks` (they are created once per instance and never re-run) |

chart2kro warns when an excluded resource is still referenced by name; it must then exist when instances are created.

## 6. Custom resources and Kubernetes versions

If the chart creates custom resources whose CRDs are not in the chart, pass them so kro's schema checks can run offline:

```bash
chart2kro convert ./mychart --crd servicemonitor-crd.yaml -o api/
```

`--kube-version` selects the Kubernetes minor for `.Capabilities` and validation (1.35, 1.36 or 1.37; default the newest).

## 7. Commit the API and guard it in CI

Commit `api/` and add two checks:

```yaml
# .github/workflows/api.yaml
- run: chart2kro convert ./mychart -f platform.yaml --expose '…' -o api/ --check
- run: chart2kro diff ./mychart -f platform.yaml --expose '…' --against api/rgd.yaml
```

`--check` exits 9 when `api/` is stale. `diff` exits 8 when a change would break existing instances:

| Class | Examples |
|---|---|
| Breaking | A field removed or retyped; a required field added; a default removed; kind, group, version or scope changed |
| Destructive | A resource removed: kro deletes it from every instance |
| Compatible | A field or resource added; a template, `includeWhen` or default changed |

Before upgrading the chart, run `diff` with `--version NEW`. If it is breaking, publish the new chart as a new kind or version and migrate instances.

## Chart sources and credentials

| Source | Example |
|---|---|
| Directory or archive | `./mychart`, `mychart-1.2.3.tgz` |
| OCI registry | `oci://ghcr.io/org/charts/app --version 1.2.3` |
| Repository from `helm repo add` | `bitnami/nginx` |
| Repository URL | `nginx --repo https://charts.example.com` |

`--username`, `--password`, `--ca-file`, `--cert-file` and `--key-file` work for registries and repositories. Credentials are never written or logged. Charts with dependencies need them vendored first: `helm dependency build`.
