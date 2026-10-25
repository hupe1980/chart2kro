+++
title = "Migrating from Helm"
description = "Move a Helm release to a kro instance: design the API, install it, turn `helm get values` into an instance, and switch over without downtime."
weight = 40

[extra]
group = "Guides"
nav = "Migrating from Helm"
+++

## 1. Design the API

```bash
chart2kro inspect ./mychart                       # what can be exposed, and what cannot
chart2kro convert ./mychart \
  -f platform-defaults.yaml \
  --expose 'replicaCount,image.tag,resources,ingress.*' \
  --group apps.example.com --kind WebApp \
  -o api/
```

Commit `api/` and run `chart2kro convert … -o api/ --check` in CI (exit 9 if stale). Before upgrading the chart, run `chart2kro diff ./mychart --version NEW --against api/rgd.yaml`: it exits 8 if existing instances would break.

## 2. Install the API

```bash
kubectl apply -f api/prerequisites.yaml   # only if the file exists: CRDs and shared cluster objects
kubectl apply -f api/rgd.yaml
kubectl get rgd                           # STATE Active
```

## 3. Turn releases into instances

```bash
helm get values my-release -n prod -o yaml > release.yaml
chart2kro instance ./mychart -f platform-defaults.yaml \
  --expose 'replicaCount,image.tag,resources,ingress.*' \
  --from release.yaml --name my-release --namespace prod > my-release.yaml
```

Pass the same conversion flags as in step 1. If the release sets values the API does not expose, the command fails and lists them: expose them, or accept their defaults with `--allow-unexposed`.

## 4. Switch over

kro creates objects named after the instance: `my-release-mychart`, like Helm's fullname for a release called `my-release`.

- **New installs:** apply the instance.
- **Existing releases:** objects created by Helm carry Helm's labels and ownership metadata. The safe path is to create the instance under a new name (or in a new namespace), move traffic, and then `helm uninstall` the release.

Adopting running Helm objects in place is not supported.
