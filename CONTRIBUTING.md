# Contributing

## Setup

You need Go (the version in `go.mod`), [just](https://github.com/casey/just) and golangci-lint v2. The E2E tests also need Docker, kind, helm and kubectl; the site needs [Zola](https://www.getzola.org).

```bash
just check     # lint, race tests, per-package coverage floor (80 %)
just golden    # rewrite golden files after an intended output change; review the diff
just corpus    # convert real-world charts (network, a few minutes)
just e2e       # kind + kro: RGDs become Active, instances are isolated
just vuln      # govulncheck
go test ./internal/rgd -run '^$' -fuzz FuzzEscapedLiteralRoundTrip   # when changing escaping
just site      # documentation site with live reload (needs zola)
```

## Ground rules

- **kro decides.** Never re-implement kro's rules; call its code (`internal/kroval`, `internal/oracle`). Output must pass kro's admission schema and graph builder.
- **Helm decides what is correct.** A conversion change must keep instances equal to `helm template` at the witnesses. The only exceptions are the documented ones in [How it works](https://hupe1980.github.io/chart2kro/docs/how-it-works/).
- **Nothing silently.** A value that cannot be exposed is reported with a stable reason code.
- **Test on real charts.** Fixtures in `testdata/charts` (with golden output in `testdata/golden`) plus `just corpus`. `testdata/charts/features` exercises every rule.
- **Small surface.** New commands or flags need a strong reason. Prefer the chart's own values and Helm's flags.
- **No subprocesses** in the product code (tests may call kubectl).

## Layout

```text
cmd/chart2kro       main
internal/cli        commands (thin client of pkg/chart2kro)
pkg/chart2kro       public API: Convert, Inspect, Validate, Diff, Instance
internal/chart      Helm v4 loading and rendering
internal/probe      candidates, typed probes, classification
internal/rgd        RGD assembly
internal/kroval     offline kro validation (embedded OpenAPI catalog)
internal/oracle     offline instantiation with kro's runtime
internal/manifest   tree operations
internal/yamlx      faithful YAML encoding
internal/rgddiff    API change classification
internal/version    build information
action/             GitHub Action scripts (action.yml at the root)
site/               website and docs (Zola)
```

The website and documentation live in `site/` ([Zola](https://www.getzola.org)): pages in `site/content/docs/`, the landing page in `site/content/_index.md` and `site/templates/index.html`. CI builds it on every PR and publishes it from `main`.

Commits follow [Conventional Commits](https://www.conventionalcommits.org/). Add a line to `CHANGELOG.md` for user-visible changes.

## Releasing

1. Run `just corpus` (real-world charts; it does not run in CI).
2. Rename `## [Unreleased]` in `CHANGELOG.md` to `## [X.Y.Z] - YYYY-MM-DD` and add a new empty `[Unreleased]` section above it.
3. Merge, then tag `vX.Y.Z` on `main` and push the tag.

The release workflow uses that CHANGELOG section as the release notes and fails if it is missing. It builds and signs the binaries and the image, and attests the archives (see [Verifying a release](https://hupe1980.github.io/chart2kro/docs/verify-release/)).
