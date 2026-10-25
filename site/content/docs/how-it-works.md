+++
title = "How it works"
description = "How chart2kro renders, probes, classifies, assembles, validates and verifies a conversion with Helm's SDK and kro's own code."
weight = 100

[extra]
group = "Explanation"
nav = "How it works"
+++

chart2kro answers one question for every chart value: *if I let users of the new kind set this value, will kro produce exactly what Helm would?* It answers by experiment, and checks every answer with kro's own code.

## 1. Render

The chart is rendered in memory with the Helm v4 SDK, as `helm template` would render it: dependencies with their `condition`/`tags`, `values.schema.json` validation, and `.Capabilities` for `--kube-version`. Rendering uses a placeholder release name and namespace, so every place the chart uses `.Release.Name` or `.Release.Namespace` is known. Those places become `${schema.metadata.name}` and `${schema.metadata.namespace}`.

The chart is rendered twice with the same inputs. Fields that differ are values the chart *generates* (random passwords, certificates, hook name suffixes):

- generated Secret entries (passwords, keys, certificates) become **required inputs**: kro has no secure random source, and its `random.seededString` derives from the instance's public UID;
- other random alphanumerics (for example name suffixes) become `${random.seededString(n, schema.metadata.uid)}`, stable per instance;
- anything else generated becomes a required input;
- `checksum/*` annotations become a hash of the instance's spec, so pods still roll when the configuration changes.

## 2. Probe

Each value is rendered again with a **typed probe**:

| Value | Probes |
|---|---|
| Boolean | The opposite value. Two values, so both branches are seen |
| Number | Two distinct numbers |
| String | Two distinct mixed-case strings |
| List or empty map | Two distinct passthrough values |

Probes run one value at a time, in parallel. A probe that makes the chart fail affects only that value (reported as *failed*).

## 3. Classify

Each probe render is compared with the baseline:

| What changed | Becomes |
|---|---|
| Nothing, or only objects that are dropped (hooks, excluded) | **unused** (the value has no effect on instances with these defaults) |
| A field equals the probe | `${schema.spec.value}`, adapted to the field's type (`${string(...)}` for strings) |
| A string contains the probe | A string template: `${schema.spec.image.repository}:${schema.spec.image.tag}` |
| A boolean adds or removes whole objects | `includeWhen: ${schema.spec.enabled}`. Fields inside those objects are probed too |
| A boolean switches a field between two values | `${schema.spec.flag ? "Always" : "IfNotPresent"}` |
| A list or map passed through whole | `${schema.spec.nodeSelector}` |
| A list whose `range` loop emits objects | A collection: `forEach: [{tenant: ${schema.spec.tenants}}]`, entry fields as `${tenant.name}`, one resource per document of the loop body |
| Fields appear or disappear, or the value is transformed (`upper`, `trunc`, arithmetic) | **frozen**, with the reason |

Objects are matched by template and position, never by name, because names often depend on values.

**Loops.** For a list that adds or removes objects, the chart is rendered with 0, 1, 2 and 3 copies of its first entry. A template file that then emits exactly *k* objects per entry is a loop. Each field of the first entry is probed like a value, and differences between identical copies must be the loop index. The list's schema gets an element type listing exactly the default entry's fields (required, so unknown fields are rejected) and a unique key on the field that names the objects.

## 4. Assemble

The RGD follows kro's rules:

- lowerCamelCase resource ids;
- `readyWhen` on workloads by resource id;
- a schema typed and defaulted from the merged values, with descriptions from `values.yaml` comments (helm-docs `# --`, bitnami `## @param`, or a plain comment above the key), so `kubectl explain` documents every input;
- literal `${` in chart content escaped;
- `immutable=true` on inputs that land in fields Kubernetes does not allow to change (selectors, volume claim templates, …);
- CRDs and fixed-name cluster-scoped objects moved to `prerequisites.yaml`;
- test hooks dropped (other hooks only with `--include-hooks`).

No references between resources are invented. Resources find each other by the same templated names Helm uses, so kro never waits on readiness the chart did not ask for.

Two more rules:

- **Isolation.** If a workload's selector would match every instance's pods, the label `app.kubernetes.io/instance: ${schema.metadata.name}` is added to it, its pods, and the Services, PodDisruptionBudgets and NetworkPolicies that selected them.
- **Secrets.** A value you supplied with `-f`/`--set` that ends up in a Secret, and any value the chart generates for a Secret, becomes a required input without a default. Computed Secret entries are written as `stringData`; the API server merges them into `data`.

## 5. Validate

The RGD is checked offline with kro's own code:

1. kro's RGD CRD schema, exactly as the API server enforces it;
2. kro's graph builder, against an embedded OpenAPI catalog of Kubernetes plus the chart's CRDs and `--crd` files.

If kro would reject the RGD, chart2kro does not write it. If a rejection can be traced to one value (for example a type kro's checker refuses), that value is frozen and the conversion continues.

## 6. Verify

kro's own runtime instantiates the RGD offline at every witness:

- the defaults;
- each exposed value at both probes;
- the flags with the values inside their objects;
- all values at once.

Each instantiation is compared with Helm's render for the same values. Only these differences are expected and ignored:

1. the namespace field (kro places objects in the instance's namespace);
2. the isolation labels;
3. `checksum/*` annotations;
4. kro's `kro.run/collection-index` label on collection objects;
5. generated values;
6. objects moved to prerequisites or dropped.

A value whose instantiation differs from Helm is frozen, with the first difference as the reason, and the RGD is assembled again. This always ends: with nothing exposed, the RGD is the defaults, which match by construction.

The result is a claim of **validation at the listed witnesses** (the report counts them), not a proof of equivalence for every possible value.
