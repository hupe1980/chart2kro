+++
title = "Troubleshooting"
description = "Every chart2kro exit code and reason code with its fix, plus what to do when an RGD stays Inactive or an instance stays IN_PROGRESS."
weight = 50

[extra]
group = "Guides"
nav = "Troubleshooting"
+++

Find your exit code, then the fix.

| Exit | Message | Fix |
|---|---|---|
| 2 | `--kind "my-app" must match …` | Kinds are PascalCase (`MyApp`). Groups are DNS subdomains; API versions look like `v1alpha1` |
| 2 | `derived kind … is not a valid kro kind` | The chart name does not make a valid kind (e.g. it starts with a digit): pass `--kind` |
| 2 | `invalid glob …` | `--expose`/`--freeze` patterns use `*`, `?`, `[…]` per segment and `**` across segments; close every `[` |
| 2 | `instance name … must be a DNS-1123 subdomain` | Lowercase letters, digits, `-` and `.` only |
| 3 | `missing dependencies …: run 'helm dependency build'` | The chart declares subcharts that are not vendored. Run `helm dependency build`, or convert the packaged `.tgz` |
| 3 | `repository "x" not found` | Run `helm repo add x URL`, or pass `--repo URL` |
| 4 | Helm's template error | The chart does not render with these values. `helm template` shows the same error |
| 5 | `cannot expose: <value> (<reason>: …)` | That value cannot be a verified input. `chart2kro inspect --value <path>` explains why. Remove it from `--expose`, or set it with `-f` as a fixed platform default |
| 5 | `the RGD is N bytes (limit …)` | Exclude what the API does not need: `--exclude <id>` or `--exclude-subchart <name>`. The message names the largest resources |
| 5 | `these values are set but not exposed` (`instance`) | Expose them in the conversion, or accept the defaults with `--allow-unexposed` |
| 5 | `these values do not match the API's types` (`instance`) | Fix the value's type in the release values (e.g. `replicas: 2`, not `"2"`) |
| 7 | `cannot resolve group version "…": schema not found` | The chart uses custom resources: pass their CRDs with `--crd crds.yaml` |
| 7 | other kro messages | Report it, with `chart2kro version` and the chart. chart2kro should never produce an RGD kro rejects |
| 8 | `diff` lists breaking changes | Existing instances would break. Keep the old API (a new `--kind` or `--group` for the new one), or migrate instances first |
| 9 | `stale` | Run the same `convert` without `--check` and commit the result |
| 130 | `interrupted` | Ctrl-C (or the context was cancelled). Nothing is half-written: output files are replaced atomically |

When kro rejects a resource, the message also names the chart template it came from.

## Reason codes

Every value that is not exposed carries one of these codes (`inspect`, `report.json`):

| Code | Outcome | Meaning | What to do |
|---|---|---|---|
| `render-failed` | failed | The chart does not render with the probe value (e.g. it validates the value). Probe values are type-preserving but synthetic | Set the value with `-f` as a platform default |
| `changes-structure` | frozen | The value adds or removes fields, or changes a field's type | Set it with `-f`, or offer the variants as separate APIs (`--kind`) |
| `toggles-fields` | frozen | A boolean switches fields inside an object on or off | Set it with `-f` |
| `transformed` | frozen | The value reaches the output only through a template function (`upper`, `trunc`, arithmetic, `printf` with several values) | Set it with `-f`; or change the chart to use the value directly |
| `adds-or-removes-objects` | frozen | A non-boolean value changes which objects exist | Set it with `-f`; or gate the objects with a boolean in the chart |
| `branch-types-differ` | frozen | A boolean selects values of different types; kro's ternary needs one type | Set it with `-f` |
| `null-default` | frozen | The default is `null`, so its type is unknown | Give it a typed default with `-f` (e.g. `""` or `{}`) |
| `key-not-addressable` | frozen | The key cannot be written as a CEL path (e.g. it starts with a digit) | Set it with `-f` |
| `shares-field` | frozen | Two values write the same field in a way one expression cannot express | Expose only one of them (`--expose`/`--freeze`) |
| `verification-mismatch` | frozen | kro's offline instantiation differed from Helm at a witness; the detail names the first difference | Set it with `-f`; report it if the difference looks wrong |
| `kro-type-check` | frozen | kro's type checker rejected the expression for this value | Set it with `-f` |
| `affects-only-dropped-objects` | unused | The value only changes hooks, excluded resources or prerequisites | Nothing; it has no effect on instances |
| `user-secret` | frozen | The value is Secret data you supplied with `-f`/`--set`; it is never written into the RGD | `--expose` it to make it an instance input |
| `unknown-kind` | frozen | The value enables a custom resource whose CRD is not available | Pass the CRD with `--crd` |

## The RGD applied, but stays `Inactive`

```bash
kubectl get rgd NAME -o jsonpath='{.status.conditions}'
```

`cannot update CRD …: breaking changes detected` means a previous RGD with the same `group` and `kind` left its CRD behind, and the new schema is incompatible. Use another `--kind`/`--group`, or delete the old CRD once no instances remain.

## An instance stays `IN_PROGRESS`

The workloads are not ready yet (`kubectl get deploy,sts -n NS`). chart2kro adds readiness only for Deployments, StatefulSets and DaemonSets.

## An `immutable` field can still be changed

kro v0.9.4 does not update an existing CRD when only validation rules change. A field that became `immutable=true` is enforced only for CRDs created after the change, or after another schema change updates the CRD. `diff` reports such changes.

## A value I expected is "unused"

The value has no effect with the current defaults. For example, it sits inside a block that another value switches off. Enable that block with `-f`/`--set`, then convert again.
