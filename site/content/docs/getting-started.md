+++
title = "Getting started"
description = "Install chart2kro, convert the podinfo Helm chart into a kro API, and run two instances of it on a local cluster in about ten minutes."
weight = 10

[extra]
group = "Start here"
+++

You will convert the [podinfo](https://github.com/stefanprodan/podinfo) chart into a new Kubernetes API, `Podinfo`, and create two instances of it. New to kro? Skim [Background](@/docs/background.md) first.

## 1. Install chart2kro

```bash
brew install hupe1980/tap/chart2kro
# or
go install github.com/hupe1980/chart2kro/cmd/chart2kro@latest
```

Or download a [signed release](https://github.com/hupe1980/chart2kro/releases) ([how to verify it](@/docs/verify-release.md)), or run the image: `docker run --rm ghcr.io/hupe1980/chart2kro --help`. Converting needs no cluster, Helm or kubectl.

## 2. Get a cluster with kro

Skip this if you already have kro v0.9.2 or newer.

```bash
kind create cluster
helm install kro oci://registry.k8s.io/kro/charts/kro --version 0.9.4 \
  --namespace kro-system --create-namespace --wait
```

## 3. Convert the chart

```bash
chart2kro convert oci://ghcr.io/stefanprodan/charts/podinfo --version 6.15.0 -o podinfo/
```

```text
podinfo 6.15.0 → Podinfo.kro.run/v1alpha1: 3 resources, 29 exposed, 33 frozen, 0 failed, 60 unused (256 witnesses, kro v0.9.4, Kubernetes 1.37)
  frozen: changes-structure             15
  frozen: toggles-fields                8
  frozen: transformed                   6
  frozen: unknown-kind                  4
  unused: affects-only-dropped-objects  11
  run 'chart2kro inspect' for details
  dropped: Pod <generated> (test hook)
```

The summary says: the chart has 3 resources; 29 values became inputs of the new API; the rest stay at their defaults, and each has a [reason](@/docs/troubleshooting.md#reason-codes). The 256 witnesses are the value combinations at which kro's output was compared with Helm's.

`podinfo/` now holds:

| File | Contents |
|---|---|
| `rgd.yaml` | The `ResourceGraphDefinition`: the API's schema and the resource templates |
| `instance.yaml` | An example instance with every input at its default |
| `report.json` | Every value and resource, with its outcome and reason |
| `prerequisites.yaml` | Only if the chart has CRDs or shared cluster objects: apply it once, first |

`report.json` follows a published [JSON schema]({{ get_url(path="schemas/report-v1.json") }}).

## 4. Install the API

```bash
kubectl apply -f podinfo/rgd.yaml
kubectl get rgd podinfo
```

```text
NAME      APIVERSION   KIND      STATE    READY   AGE
podinfo   v1alpha1     Podinfo   Active   True    2s
```

kro has created the `Podinfo` kind. Its schema is the chart's values, typed and defaulted.

## 5. Create instances

The example instance uses every default:

```bash
kubectl apply -f podinfo/instance.yaml
```

A second instance sets two inputs. Fields mirror `values.yaml`:

```yaml
apiVersion: kro.run/v1alpha1
kind: Podinfo
metadata:
  name: blue
spec:
  replicaCount: 2
  ui:
    color: "#34577c"
```

```bash
kubectl get podinfo
kubectl get deploy
```

```text
NAME      STATE    READY   AGE
blue      ACTIVE   True    36s
podinfo   ACTIVE   True    36s

NAME              READY   UP-TO-DATE   AVAILABLE   AGE
blue-podinfo      2/2     2            2           36s
podinfo-podinfo   1/1     1            1           36s
```

Each instance gets its own objects, named `<instance>-podinfo`, and its own pods: chart2kro added an `app.kubernetes.io/instance` selector label so the two never share a Service.

Delete an instance and kro deletes its objects: `kubectl delete podinfo blue`.

## 6. Find out why a value is not an input

```bash
chart2kro inspect oci://ghcr.io/stefanprodan/charts/podinfo --version 6.15.0 --value ui.message
```

```text
ui.message: frozen
  reason: changes-structure
  where:  Deployment (podinfo/templates/deployment.yaml) spec.template.spec.containers[0].env
  the value stays at its default in every instance
```

Setting `ui.message` adds an environment variable, so it changes the structure of the Deployment, which a kro expression cannot do. Set it as a platform default with `-f` instead.

## Next steps

- [Design a small API](@/docs/designing-apis.md) with `-f`, `--expose` and your own `--group`.
- [Move Helm releases to kro](@/docs/migration-from-helm.md).
- [How it works](@/docs/how-it-works.md): what "verified" means.
