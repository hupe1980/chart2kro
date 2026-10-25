+++
title = "GitHub Action"
description = "Check in every pull request that the committed RGD matches its Helm chart and that changes do not break existing instances, with one workflow step."
weight = 35

[extra]
group = "Guides"
nav = "GitHub Action"
+++

The chart2kro action runs the two checks from [Designing a platform API](@/docs/designing-apis.md#7-commit-the-api-and-guard-it-in-ci) in one step:

1. **Stale output:** `convert --check` fails if the committed directory no longer matches the chart.
2. **API changes:** `diff` compares the pull request's chart with the RGD on the base branch and fails on breaking or destructive changes.

Results appear in the job summary, with a table of API changes.

## Usage

```yaml
# .github/workflows/api.yaml
name: API
on:
  pull_request:
    paths: ["charts/webapp/**", "api/**", "platform.yaml"]

permissions:
  contents: read

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: hupe1980/chart2kro@v0.2.0
        with:
          chart: charts/webapp
          output-dir: api
          args: -f platform.yaml --expose 'replicaCount,image.tag' --group apps.example.com
```

Pin the action to a release tag (or its commit SHA). The action installs the chart2kro release with the same version and verifies it against the release's `checksums.txt`.

## Inputs

| Input | Default | |
|---|---|---|
| `chart` | (required) | Chart reference: directory, `.tgz`, `oci://…` or `repo/chart` |
| `output-dir` | (required) | Directory with the committed `convert -o` output |
| `args` | | Flags for `convert` and `diff`, as on the command line (quotes are respected) |
| `check` | `true` | Fail if `output-dir` is stale |
| `diff` | `true` | Compare with `output-dir/rgd.yaml` on the pull request's base branch |
| `fail-on-breaking` | `true` | Fail on breaking or destructive changes; `false` only reports them |
| `version` | the action's tag | chart2kro version to install |

## Outputs

| Output | |
|---|---|
| `stale` | `true` if `output-dir` is stale |
| `breaking` | `true` if the API has breaking or destructive changes |

## Notes

- The diff only runs for pull requests. On a push, or when the base branch has no `output-dir/rgd.yaml` yet, it is skipped with a note.
- Breaking changes are not always mistakes. Set `fail-on-breaking: false`, or use a new `--kind` or `--group` for the new API (see [Troubleshooting](@/docs/troubleshooting.md)).
- Private registries: pass `--username`/`--password` in `args` from secrets, or log in with `helm registry login` before the step.
