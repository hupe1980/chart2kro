+++
title = "Limitations"
description = "Which Helm template constructs chart2kro cannot turn into kro inputs, what it does instead, and why."
weight = 80

[extra]
group = "Reference"
nav = "Limitations"
+++

What chart2kro cannot translate, and what it does instead. Values affected by these rows are listed by `chart2kro inspect`.

| Construct | What happens | Why |
|---|---|---|
| A value that adds or removes **fields inside** an object (`{% raw %}{{ if .Values.x }}foo: bar{{ end }}{% endraw %}`, `{% raw %}{{- with .Values.annotations }}{% endraw %}`) | Frozen (`changes-structure` / `toggles-fields`) | kro cannot omit a field conditionally without its alpha `omit()` feature |
| A value transformed by a template function (`upper`, `trunc`, `printf` with several values, arithmetic) | Frozen (`transformed`) | CEL has no faithful equivalent for every sprig function; guessing would deploy something different |
| A value that changes the number of list items (`range` over a value) | Frozen | kro's `forEach` is not used yet |
| A value that adds or removes objects but is not a boolean | Frozen (`adds-or-removes-objects`) | Only booleans are enumerated exhaustively |
| Values that matter only together with another non-default value | Classified relative to the defaults | Each value is probed with the others at their defaults |
| Release names that contain the chart name | Instances are always named `<instance>-<chart>` | The fullname helper's short form depends on the name; the result is deterministic and unique |
| Keys that start with a digit (`1st`) | Frozen (`key-not-addressable`) | CEL cannot address them |
| Custom resources whose CRDs are not available | The flag that enables them is frozen (`unknown-kind`); in the defaults they fail validation (exit 7) | kro needs the schema of every kind. Pass `--crd` |
| Helm hooks | Test hooks are dropped; other hooks are dropped unless `--include-hooks`, which warns for each kept hook | kro has no hooks; included hooks are created once per instance and never re-run |
| CRDs and cluster-scoped objects with fixed names | Moved to `prerequisites.yaml` (with a warning for cluster-scoped objects) | They cannot belong to one instance; apply them once before the RGD |
| Generated secrets (`randAlphaNum` and friends) | Become required inputs (`chart2kroGenerated.*`) | kro's `random.seededString` is derived from the instance UID, which is not secret |
| Changing a field to `immutable` on an existing API | Takes effect only when kro next updates the CRD | kro v0.9.4 skips CRD updates that change only validation rules; `diff` reports it |
| `lookup` | Renders empty, as with `helm template` | No cluster access |
| Jobs | Reconciled by kro, not run once. A warning is printed for Jobs with `ttlSecondsAfterFinished` | kro recreates a Job that Kubernetes deleted, so it runs again |
| Kubernetes versions | Rendering and validation use the embedded catalogs for 1.35, 1.36 and 1.37 (`--kube-version`) | Validation is offline; newer versions need a chart2kro release |
| Very large charts | Exit 5 above 1 MiB, naming the largest resources | The API server limits object size; use `--exclude`, `--exclude-subchart` or a smaller `--expose` |
| kro versions | Output needs kro ≥ v0.9.1 | It uses kro's `hash`, `json` and `random` CEL libraries |
