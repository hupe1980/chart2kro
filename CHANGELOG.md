# Changelog

All notable changes to this project are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-10-25

A rewrite: v0.0.x output was rejected by kro. Regenerate every RGD.

### Breaking

- **Removed commands:** `audit`, `docs`, `export`, `watch`, `plan`, with all their flags.
- **Removed and renamed flags:**

  | v0.0.x | Now |
  |---|---|
  | `-o rgd.yaml`, `--dry-run` | `-o DIR` (`rgd.yaml`, `prerequisites.yaml`, `instance.yaml`, `report.json`); stdout without `-o` |
  | `--repo-url` | `--repo` |
  | `diff --existing`, `plan` | `diff --against`, `inspect` |
  | `--exclude-resources`, `--exclude-subcharts` | `--exclude` (ids from `inspect`), `--exclude-subchart` |
  | `--exclude-kinds`, `--exclude-labels`, `--profile` | `--exclude` |
  | `--json`, `--fail-on`, `--strict` | `--format json`, exit codes, strict `--expose` |
  | `--harden`, `--security-level`, `--generate-network-policies`, `--generate-rbac` | removed; use a policy engine on the output |
  | `--release-name` | the instance's `metadata.name` |
  | `inspect --show-*` | `inspect` shows everything; `--value PATH` for one value |
  | `--externalize-*`, `--use-external-pattern`, `--flat-schema`, `--include-all-values`, `--include-examples`, `--comments`, `--split`, `--resolve-digests`, `--fast`, `--embed-timestamp`, `--ready-conditions`, `--validate`, `--no-color`, `--config` | removed |

- **Output:** `schema.group` is separate from `apiVersion`; lowerCamelCase ids; no `dependsOn`; names and namespaces come from the instance; computed Secret entries are `stringData`.
- **Exit codes:** see the [CLI reference](https://hupe1980.github.io/chart2kro/docs/cli-reference/#exit-codes).
- **Library:** one `Options` struct; `Convert` returns `*Result`.
- **Requirements:** Go ≥ 1.26 to build; kro ≥ v0.9.1 to run the output.

### Added

- Verified conversion: a value is exposed only when kro's runtime, run offline, reproduces Helm's output for it.
- Offline validation with kro v0.9.4's admission schema and graph builder (`validate`, and in `convert`).
- `instance`, `convert --check`, `--expose` and `--freeze`. `inspect` explains every value's outcome; `diff` classifies breaking, destructive and compatible changes. JSON output for `inspect`, `validate`, `diff` and `version`.
- Booleans become `includeWhen` or CEL conditionals; immutable Kubernetes fields become `immutable=true`.
- Isolated instances; generated secrets become required inputs; CRDs and shared cluster objects go to `prerequisites.yaml`.
- Signed releases with build provenance, a container image, and a Homebrew cask (`brew install hupe1980/tap/chart2kro`).

### Changed

- Helm SDK v4, including dependency `condition`/`tags` and `--kube-version` capabilities.
- YAML output keeps empty maps, empty list items and nulls.

[Unreleased]: https://github.com/hupe1980/chart2kro/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/hupe1980/chart2kro/compare/v0.0.2...v0.1.0
