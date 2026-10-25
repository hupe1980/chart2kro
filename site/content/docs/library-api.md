+++
title = "Go library"
description = "Use chart2kro from Go: Convert, Inspect, Validate, Diff and Instance in the pkg/chart2kro package, with the same results as the CLI."
weight = 70

[extra]
group = "Reference"
nav = "Go library"
+++

```bash
go get github.com/hupe1980/chart2kro
```

The CLI is a thin client of `pkg/chart2kro`: the library produces exactly the CLI's output.

```go
import "github.com/hupe1980/chart2kro/pkg/chart2kro"

res, err := chart2kro.Convert(ctx, chart2kro.Options{
    Chart:      "oci://ghcr.io/stefanprodan/charts/podinfo",
    Version:    "6.15.0",
    ValueFiles: []string{"platform-defaults.yaml"},
    Expose:     []string{"replicaCount", "image.tag"},
    Group:      "apps.example.com",
})
if err != nil {
    os.Exit(chart2kro.ExitCode(err)) // same codes as the CLI
}

os.WriteFile("rgd.yaml", res.RGD, 0o644)
// res.Prerequisites: CRDs and shared cluster objects (may be empty)
// res.Instance:      an example instance
// res.Report:        every value with its outcome and reason
```

## API

| Function | |
|---|---|
| `Convert(ctx, Options) (*Result, error)` | Convert a chart |
| `Inspect(ctx, Options) (*Report, error)` | Convert and return only the report |
| `Validate(ctx, yamlStream, ValidateOptions) error` | Validate RGDs offline with kro's own checks |
| `Diff(existing, updated []byte) (*DiffResult, error)` | Classify API changes (breaking, destructive, compatible). Inputs may be YAML streams; each must contain exactly one RGD. JSON schema `chart2kro.io/diff/v1` |
| `Instance(ctx, Options, InstanceOptions) ([]byte, error)` | Helm values to an instance manifest |
| `ExitCode(err) int` | Map errors to the CLI's exit codes |

## Options

`Options` mirrors the CLI flags; the zero value means Helm's defaults. The fields you will use most:

| Field | Flag |
|---|---|
| `Chart`, `Version`, `RepoURL` | `CHART`, `--version`, `--repo` |
| `ValueFiles`, `Set`, `SetString`, `SetFile` | `-f`, `--set`, `--set-string`, `--set-file` |
| `Kind`, `Group`, `APIVersion` | `--kind`, `--group`, `--api-version` |
| `Expose`, `Freeze` | `--expose`, `--freeze` |
| `Exclude`, `ExcludeSubcharts`, `IncludeHooks` | `--exclude`, `--exclude-subchart`, `--include-hooks` |
| `KubeVersion`, `CRDFiles` | `--kube-version`, `--crd` |
| `Logger` | `--log-level`: a `*slog.Logger` for progress logs (default: discard) |

The full list is on [pkg.go.dev](https://pkg.go.dev/github.com/hupe1980/chart2kro/pkg/chart2kro).

## Errors

Errors wrap sentinel values, so test them with `errors.Is`: `ErrUsage`, `ErrLoad`, `ErrRender`, `ErrConstraint`, `ErrWrite`, `ErrInvalid`, `ErrBreaking`, `ErrStale`. `ExitCode` maps them to the CLI's [exit codes](@/docs/cli-reference.md#exit-codes).
