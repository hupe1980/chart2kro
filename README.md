<div align="center">

# ⚓ chart2kro

**Turn Helm charts into kro APIs you can trust**

[![CI](https://github.com/hupe1980/chart2kro/actions/workflows/ci.yaml/badge.svg)](https://github.com/hupe1980/chart2kro/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/hupe1980/chart2kro)](https://github.com/hupe1980/chart2kro/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/hupe1980/chart2kro/pkg/chart2kro.svg)](https://pkg.go.dev/github.com/hupe1980/chart2kro/pkg/chart2kro)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

`chart2kro` converts a Helm chart into a [kro](https://kro.run) `ResourceGraphDefinition`: a Kubernetes API whose instances deploy what the chart deploys. Every input is verified against Helm with kro's own code, and every value it does not expose comes with a reason.

[Website](https://hupe1980.github.io/chart2kro/) · [Quick Start](#-quick-start) · [Commands](#-commands) · [Go Library](#-go-library) · [Examples](examples/) · [Docs](https://hupe1980.github.io/chart2kro/docs/)

</div>

---

## ✨ Features

| | Feature | Description |
|---|---|---|
| ✅ | **Accepted by kro** | Every RGD is checked offline with kro's own admission schema and graph builder before it is written |
| 🎯 | **Faithful to Helm** | An instance with the defaults deploys what `helm template` deploys |
| 🔬 | **Verified inputs** | A value becomes an input only after kro's runtime, run offline, reproduced Helm's output for it; loops over lists become kro collections |
| 📋 | **Every decision explained** | Inputs carry the chart's `values.yaml` docs; values that are not exposed get a reason code |
| 🧱 | **Isolated instances** | Names and namespaces come from the instance; selectors never reach another instance's pods |
| 🔐 | **No secret leaks** | Secrets you pass never become defaults; generated passwords become required inputs |
| 🚦 | **CI-ready** | `convert --check` catches stale output, `diff` catches breaking API changes, and a GitHub Action runs both |
| 📦 | **Go library** | The CLI is a thin client of `pkg/chart2kro` |

One static binary. No Helm, kubectl or cluster needed to convert. Output runs on **kro ≥ v0.9.1**.

---

## 📦 Installation

<details open>
<summary><b>Homebrew</b></summary>

```bash
brew install hupe1980/tap/chart2kro
```

</details>

<details>
<summary><b>Go install</b></summary>

```bash
go install github.com/hupe1980/chart2kro/cmd/chart2kro@latest
```

</details>

<details>
<summary><b>Release binaries</b></summary>

Download an archive for Linux, macOS or Windows from the [releases](https://github.com/hupe1980/chart2kro/releases). Releases are signed; see [verifying a release](https://hupe1980.github.io/chart2kro/docs/verify-release/).

</details>

<details>
<summary><b>Docker</b></summary>

```bash
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/work" -w /work \
  ghcr.io/hupe1980/chart2kro convert ./mychart -o api/
```

</details>

<details>
<summary><b>From source</b></summary>

```bash
git clone https://github.com/hupe1980/chart2kro.git && cd chart2kro
just build   # or: CGO_ENABLED=0 go build -o chart2kro ./cmd/chart2kro
```

</details>

---

## 🚀 Quick Start

**1. Convert a chart** (a directory, `.tgz`, `oci://` reference or `repo/chart`):

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
```

**2. Install the API** on a cluster with kro:

```bash
kubectl apply -f podinfo/rgd.yaml
```

**3. Create instances.** Fields mirror the chart's `values.yaml`:

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

The [Getting started](https://hupe1980.github.io/chart2kro/docs/getting-started/) guide walks through this on a local cluster.

---

## 🔧 Commands

| Command | Purpose | Example |
|---|---|---|
| `convert` | Chart → RGD, example instance and report | `chart2kro convert ./mychart -f platform.yaml --expose 'image.tag,replicaCount' -o api/` |
| `inspect` | Why a value is exposed, frozen or unused | `chart2kro inspect ./mychart --value ui.message` |
| `validate` | Validate any RGD offline with kro's checks | `chart2kro validate api/rgd.yaml` |
| `diff` | Classify API changes against an existing RGD | `chart2kro diff ./mychart --version 2.0.0 --against api/rgd.yaml` |
| `instance` | Helm release values → an instance | `chart2kro instance ./mychart --from release.yaml --name web` |
| `version` | Version, kro and Kubernetes catalogs | `chart2kro version --format json` |

In CI, `convert -o api/ --check` exits 9 when `api/` is stale, and `diff` exits 8 on breaking changes. On GitHub, one step does both:

```yaml
- uses: hupe1980/chart2kro@v0.2.0
  with:
    chart: charts/webapp
    output-dir: api
```

See the [GitHub Action guide](https://hupe1980.github.io/chart2kro/docs/github-action/). All flags and exit codes are in the [CLI reference](https://hupe1980.github.io/chart2kro/docs/cli-reference/).

---

## 🐹 Go Library

```go
import "github.com/hupe1980/chart2kro/pkg/chart2kro"

res, err := chart2kro.Convert(ctx, chart2kro.Options{
    Chart:  "oci://ghcr.io/stefanprodan/charts/podinfo",
    Expose: []string{"replicaCount", "image.tag"},
    Group:  "apps.example.com",
})
if err != nil {
    os.Exit(chart2kro.ExitCode(err)) // same exit codes as the CLI
}
// res.RGD, res.Instance, res.Prerequisites, res.Report
```

See the [Go library guide](https://hupe1980.github.io/chart2kro/docs/library-api/) and [pkg.go.dev](https://pkg.go.dev/github.com/hupe1980/chart2kro/pkg/chart2kro).

---

## 🐚 Shell Completion

```bash
source <(chart2kro completion bash)    # also: zsh, fish, powershell
```

---

## 📚 Documentation

| | |
|---|---|
| 🏁 [Getting started](https://hupe1980.github.io/chart2kro/docs/getting-started/) | Convert a chart and run two instances |
| 🧭 [Background](https://hupe1980.github.io/chart2kro/docs/background/) | kro, RGDs and how they differ from Helm |
| 🎛️ [Designing a platform API](https://hupe1980.github.io/chart2kro/docs/designing-apis/) | `-f`, `--expose`, API groups and CI checks |
| 🚚 [Migrating from Helm](https://hupe1980.github.io/chart2kro/docs/migration-from-helm/) | Turn running releases into instances |
| 🚦 [GitHub Action](https://hupe1980.github.io/chart2kro/docs/github-action/) | Check every pull request |
| ⚙️ [How it works](https://hupe1980.github.io/chart2kro/docs/how-it-works/) | Render, probe, assemble, validate, verify |
| 🚧 [Limitations](https://hupe1980.github.io/chart2kro/docs/limitations/) · 🩺 [Troubleshooting](https://hupe1980.github.io/chart2kro/docs/troubleshooting/) | What cannot be translated, and every exit and reason code |

Upgrading from v0.0.x? See the [CHANGELOG](CHANGELOG.md).

---

## 🤝 Contributing

Contributions are welcome; see [CONTRIBUTING.md](CONTRIBUTING.md).

## 📄 License

Apache 2.0, see [LICENSE](LICENSE).
