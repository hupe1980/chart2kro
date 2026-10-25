# ⚓ chart2kro

[![CI](https://github.com/hupe1980/chart2kro/actions/workflows/ci.yaml/badge.svg)](https://github.com/hupe1980/chart2kro/actions/workflows/ci.yaml)
[![Go Reference](https://pkg.go.dev/badge/github.com/hupe1980/chart2kro.svg)](https://pkg.go.dev/github.com/hupe1980/chart2kro/pkg/chart2kro)
[![Go Report Card](https://goreportcard.com/badge/github.com/hupe1980/chart2kro)](https://goreportcard.com/report/github.com/hupe1980/chart2kro)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

**Turn a Helm chart into a [kro](https://kro.run) `ResourceGraphDefinition`: a Kubernetes API whose instances deploy what the chart deploys, verified with kro's own code, with every decision explained.**

📖 **Documentation: [hupe1980.github.io/chart2kro](https://hupe1980.github.io/chart2kro/)**

```bash
chart2kro convert oci://ghcr.io/stefanprodan/charts/podinfo -o podinfo/
kubectl apply -f podinfo/rgd.yaml          # a new API: kind Podinfo
kubectl apply -f podinfo/instance.yaml     # one instance, the chart's defaults
```

## ✨ Why chart2kro

| | |
|---|---|
| ✅ **Accepted by kro** | Every RGD is checked offline with kro's own admission schema and graph builder before it is written |
| 🎯 **Faithful to Helm** | An instance with the defaults deploys what `helm template` deploys |
| 🔬 **Verified inputs** | A value becomes an input only after kro's runtime, run offline, reproduced Helm's output for it |
| 📋 **Every decision explained** | Values that are not exposed are listed as *frozen*, *failed* or *unused*, with a reason code |
| 🧱 **Isolated instances** | Names and namespaces come from the instance; selectors never reach another instance's pods |
| 🔐 **No secret leaks** | Secrets you pass never become defaults; generated passwords become required inputs |

One static binary. No Helm, kubectl or cluster needed to convert. Output runs on **kro ≥ v0.9.1**.

## 📦 Install

```bash
brew install hupe1980/tap/chart2kro
# or
go install github.com/hupe1980/chart2kro/cmd/chart2kro@latest
```

Or grab a [signed release](https://github.com/hupe1980/chart2kro/releases), or run `docker run --rm ghcr.io/hupe1980/chart2kro --help`.

## 🚀 Usage

```bash
chart2kro convert ./mychart -o api/                     # rgd.yaml, instance.yaml, report.json
chart2kro convert ./mychart -f platform.yaml \
  --expose 'replicaCount,image.tag' --group apps.example.com -o api/
chart2kro inspect ./mychart --value ui.message          # why is a value not an input?
chart2kro validate rgd.yaml                             # validate any RGD offline
chart2kro convert ./mychart -o api/ --check             # CI: exit 9 if api/ is stale
chart2kro diff ./mychart --version 2.0.0 --against api/rgd.yaml   # exit 8 if breaking
chart2kro instance ./mychart --from release-values.yaml --name web # Helm release → instance
```

```text
podinfo 6.15.0 → Podinfo.kro.run/v1alpha1: 3 resources, 29 exposed, 33 frozen, 0 failed, 60 unused (256 witnesses, kro v0.9.4, Kubernetes 1.37)
  frozen: changes-structure             15
  frozen: toggles-fields                8
  frozen: transformed                   6
  frozen: unknown-kind                  4
  unused: affects-only-dropped-objects  11
  run 'chart2kro inspect' for details
```

## 📚 Learn more

- 🏁 [Getting started](https://hupe1980.github.io/chart2kro/docs/getting-started/): convert a chart and run two instances in ten minutes
- 🧭 [Background](https://hupe1980.github.io/chart2kro/docs/background/): kro, RGDs and how they differ from Helm
- 🎛️ [Designing a platform API](https://hupe1980.github.io/chart2kro/docs/designing-apis/) · 🚚 [Migrating from Helm](https://hupe1980.github.io/chart2kro/docs/migration-from-helm/)
- 🔧 [CLI reference](https://hupe1980.github.io/chart2kro/docs/cli-reference/) · 🐹 [Go library](https://hupe1980.github.io/chart2kro/docs/library-api/) · 🩺 [Troubleshooting](https://hupe1980.github.io/chart2kro/docs/troubleshooting/)
- ⚙️ [How it works](https://hupe1980.github.io/chart2kro/docs/how-it-works/) · 🚧 [Limitations](https://hupe1980.github.io/chart2kro/docs/limitations/)
- 🧪 [Examples](examples/) with generated output

Upgrading from v0.0.x? See the [CHANGELOG](CHANGELOG.md).

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## 📄 License

Apache 2.0, see [LICENSE](LICENSE).
