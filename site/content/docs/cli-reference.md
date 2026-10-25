+++
title = "CLI reference"
description = "Every chart2kro command, flag, output format and exit code: convert, inspect, validate, diff, instance and version."
weight = 60

[extra]
group = "Reference"
nav = "CLI"
+++

```text
chart2kro convert   CHART [flags]                 convert a chart into a ResourceGraphDefinition
chart2kro inspect   CHART [flags]                 show what a conversion exposes, freezes and drops, and why
chart2kro validate  FILE... [flags]               validate any RGD offline with kro's own checks
chart2kro diff      CHART --against RGD [flags]   classify API changes against an existing RGD
chart2kro instance  CHART --from VALUES --name N  turn Helm values into an instance of the generated kind
chart2kro version   [--format json]
chart2kro completion bash|zsh|fish|powershell
```

`CHART` is a directory, a `.tgz`, an `oci://` reference, or `repo/name` (resolved through your Helm `repositories.yaml`, or a `--repo` URL).

## Global flags

| Flag | Default | |
|---|---|---|
| `--log-level` | `warn` | `debug`, `info`, `warn`, `error` (logs go to stderr) |
| `--log-format` | `text` | `text` or `json` |
| `-q, --quiet` | off | Print only results and errors (also raises `--log-level` to `error`) |

## Chart and conversion flags

`convert`, `inspect`, `diff` and `instance` accept these:

| Flag | |
|---|---|
| `--version` | Chart version (OCI and repository charts) |
| `--repo URL` | Repository URL for a plain chart name |
| `--username`, `--password`, `--ca-file`, `--cert-file`, `--key-file` | Registry or repository credentials and TLS. Credentials are never written or logged |
| `-f, --values FILE` | Values file (repeatable). Merged values become the API's **defaults** |
| `--set`, `--set-string`, `--set-file` | As in Helm (repeatable) |
| `--kind`, `--group`, `--api-version` | Identity of the generated API. Defaults: PascalCase chart name, `kro.run`, `v1alpha1`. Use your own group to avoid collisions |
| `--expose GLOBS` | Expose only these value paths (`image.*`, `**.tag`). Strict: a path that cannot be exposed fails with exit 5. Malformed globs fail with exit 2 |
| `--freeze GLOBS` | Never expose these value paths |
| `--exclude IDS` | Drop resources by id (the ids `inspect` shows). Warns if an id matches nothing, or if a kept resource still refers to the excluded one by name |
| `--exclude-subchart NAMES` | Drop every resource of a subchart |
| `--include-hooks` | Keep non-test Helm hooks as ordinary resources, with a warning for each. kro reconciles them; they do not run once |
| `--kube-version` | Kubernetes version for `.Capabilities` and for validation. Default: newest supported (`chart2kro version`) |
| `--crd FILE` | CRDs for validating custom resources the chart uses (repeatable). The chart's own CRDs are always used |
| `--timeout` | Timeout per render (default 30s) |
| `--parallel` | Concurrent renders (default: number of CPUs) |

## `convert`

| Flag | |
|---|---|
| `-o, --output-dir DIR` | Write `rgd.yaml`, `prerequisites.yaml` (if any), `instance.yaml` and `report.json` |
| `--check` | With `-o`: compare with the files in DIR instead of writing (exit 9 if stale) |
| `--report FILE` | Also write the report JSON to FILE |

Without `-o`, the prerequisites (if any) and then the RGD are written to stdout as one YAML stream. Files are replaced atomically.

`prerequisites.yaml` holds objects that cannot belong to an instance: the chart's CRDs, and cluster-scoped objects with fixed names. Apply them once, before the RGD.

## `inspect`

| Flag | |
|---|---|
| `--value PATH` | Explain one value: its description, where it lands, or why it is not exposed |
| `--format` | `text` (default) or `json` (the report) |

The JSON report follows the [`chart2kro.io/report/v1` schema]({{ get_url(path="schemas/report-v1.json") }}).

## `validate`

Validates every `ResourceGraphDefinition` in the given files (`-` for stdin) with kro's admission schema and graph builder, offline. CRDs found in the files and in `--crd` files are used to resolve custom resources. `--kube-version` selects the validation catalog.

`--format json` prints `{"schemaVersion": "chart2kro.io/validate/v1", "valid", "kroVersion", "kubeVersion", "problems": [...]}` to stdout; the exit code is still 7 if anything is invalid.

## `diff`

Converts the chart and classifies the changes from `--against RGD`. The file may be a YAML stream (such as a saved `convert` stdout); it must contain exactly one `ResourceGraphDefinition`.

| Severity | Changes |
|---|---|
| **breaking** | `kind`, `apiVersion`, `group` or `scope` changed (kro makes them immutable); a field removed or retyped; a required field added; a default removed |
| **destructive** | A resource removed: kro deletes it from every instance |
| **compatible** | A field or resource added; a resource's template or `includeWhen` changed; a default changed; `immutable` added or removed |

`--format json` prints machine-readable output ([`chart2kro.io/diff/v1`]({{ get_url(path="schemas/diff-v1.json") }})). Exit 8 if anything is breaking or destructive.

## `instance`

| Flag | |
|---|---|
| `--from FILE` | Values the instance should reproduce, e.g. from `helm get values` (repeatable) |
| `--from-set k=v` | A single value (repeatable) |
| `--name`, `--namespace` | Instance metadata (`--name` is required and must be a DNS-1123 subdomain) |
| `--allow-unexposed` | Drop values the API does not expose instead of failing. Without it, the error lists each value with the reason it is not exposed |

Values must match the API's types (exit 5 otherwise, naming the expected type). Values that land in a Secret are written into the instance with a warning: treat the file as a secret.

`-f`/`--set` keep their `convert` meaning: they are the API's defaults.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Internal error (please report it) |
| 2 | Usage error (unknown command or flag, invalid value) |
| 3 | The chart could not be loaded |
| 4 | The chart could not be rendered |
| 5 | A conversion constraint was not met: an `--expose` path cannot be exposed, the RGD is too large, or `instance` values are not exposed |
| 6 | Writing output failed |
| 7 | kro rejects the RGD (`validate`, or `convert` when the chart needs CRDs that were not supplied) |
| 8 | `diff`: breaking or destructive change |
| 9 | `convert --check`: output is stale |
| 130 | Interrupted (SIGINT) |
