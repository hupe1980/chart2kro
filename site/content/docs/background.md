+++
title = "Background: Helm, kro and ResourceGraphDefinitions"
description = "What kro and ResourceGraphDefinitions are, how they differ from Helm charts, why you would convert a chart, and the terms chart2kro uses."
weight = 20

[extra]
group = "Start here"
nav = "Background"
+++

## Helm in one paragraph

A Helm chart is a set of Go templates plus a `values.yaml`. `helm install` (or `helm template`) merges your values into the defaults, renders the templates into Kubernetes manifests on the client, and applies them as a *release*. The cluster never sees the chart, only the rendered objects.

## kro in one paragraph

[kro](https://kro.run) (Kube Resource Orchestrator, developed in [kubernetes-sigs/kro](https://github.com/kubernetes-sigs/kro)) is a controller that runs **in** the cluster. You give it a `ResourceGraphDefinition` (RGD), and it creates a new custom resource kind from it. Each object of that kind, an *instance*, makes kro create and continuously reconcile the resources the RGD describes.

## Anatomy of an RGD

```yaml
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: webapp
spec:
  schema:                       # the new API: group, version, kind and fields
    apiVersion: v1alpha1
    kind: WebApp
    spec:
      replicas: integer | default=1
      image: string | required=true
  resources:                    # what each instance creates
    - id: deployment
      readyWhen:
        - ${deployment.status.?availableReplicas.orValue(0) == deployment.spec.?replicas.orValue(1)}
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: ${schema.metadata.name}
        spec:
          replicas: ${schema.spec.replicas}
          # …
    - id: ingress
      includeWhen:
        - ${schema.spec.ingress.enabled}
      template: { … }
```

| Part | Meaning |
|---|---|
| `schema.spec` | The instance's fields in [SimpleSchema](https://kro.run/docs/concepts/rgd/schema): `type \| marker=value …`, e.g. `string \| default="nginx" immutable=true` |
| `${…}` | A [CEL](https://kro.run/docs/concepts/rgd/cel-expressions) expression, evaluated per instance. `schema.spec.x` reads an instance field; `deployment.status…` reads another resource |
| `includeWhen` | Create the resource only when the expressions are true |
| `readyWhen` | When kro considers the resource ready |

## Helm and kro compared

| | Helm | kro |
|---|---|---|
| Runs | On the client, at install time | In the cluster, continuously |
| Input | `values.yaml`, untyped | The instance's `spec`, typed and validated by the API server |
| Unit of deployment | Release | Instance of a custom kind |
| Drift | Fixed on the next `helm upgrade` | Reconciled by the controller |
| Logic | Go templates and sprig functions | CEL expressions |
| Lifecycle hooks | `pre-install`, `post-upgrade`, … | None: everything is reconciled |

## Why convert a chart

- **Platform APIs.** Offer `kind: WebApp` with five fields instead of a chart with two hundred values. RBAC, admission policies and GitOps tools work on it like on any other kind.
- **No client-side tooling.** Delivery needs only `kubectl apply`; the cluster does the rendering.
- **Reconciliation.** kro corrects drift and rolls out spec changes on its own.

What you give up: template logic that CEL cannot express, and Helm hooks. chart2kro tells you exactly which values are affected; see [Limitations](@/docs/limitations.md).

## Why not convert by hand

Templates hide which values end up where. A value can land in several fields, inside a string, behind a condition, or pass through a function like `upper`. A hand-written RGD that gets one of these wrong deploys something different from the chart, and nothing tells you. chart2kro finds each mapping by experiment and checks it with kro's own runtime; see [How it works](@/docs/how-it-works.md).

## Glossary

| Term | Meaning |
|---|---|
| **Exposed** | The value is an input of the new API (`spec.<path>`), verified against Helm. |
| **Frozen** | The value cannot be a verified input. It keeps its default in every instance, and the report gives a [reason code](@/docs/troubleshooting.md#reason-codes). |
| **Failed** | The chart does not render with a probe value for it, so its effect is unknown. It keeps its default. |
| **Unused** | Changing the value changes nothing in the RGD's resources, with the current defaults. |
| **Probe** | A synthetic, type-preserving value (a distinct number, a distinct string, a flipped boolean) used to see which fields a value controls. |
| **Witness** | One combination of values at which kro's output is compared with Helm's: the defaults, each value at its probes, and combinations. The report counts them. |
| **Collection** | A resource with `forEach`: kro creates one object per entry of a list input. chart2kro emits one for a `range` loop that creates objects |
| **Prerequisites** | Objects that cannot belong to one instance, such as the chart's CRDs. They go to `prerequisites.yaml` and are applied once. |
| **Instance label** | `app.kubernetes.io/instance: ${schema.metadata.name}`, added to selectors that would otherwise match the pods of every instance. |

## Further reading

- [kro documentation](https://kro.run/docs/overview) and [RGD concepts](https://kro.run/docs/concepts/rgd/overview)
- [Helm documentation](https://helm.sh/docs/)
- [CEL language definition](https://cel.dev)
