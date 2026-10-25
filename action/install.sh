#!/usr/bin/env bash
# Installs chart2kro for the GitHub Action: the release matching VERSION (or the
# action's own tag), verified against the release's checksums.txt, or a build from
# the action's source when it runs from a branch or SHA.
#
# Env: VERSION (optional), ACTION_REF, ACTION_PATH, RUNNER_TEMP, GITHUB_PATH
set -euo pipefail

repo="hupe1980/chart2kro"
bin_dir="${RUNNER_TEMP:-/tmp}/chart2kro/bin"
mkdir -p "$bin_dir"

version="${VERSION:-}"
if [[ -z "$version" && "${ACTION_REF:-}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]]; then
  version="$ACTION_REF"
fi
version="${version#v}"

if [[ -z "$version" ]]; then
  echo "Building chart2kro from source (${ACTION_REF:-local})"
  (cd "${ACTION_PATH:-.}" && CGO_ENABLED=0 go build -trimpath -o "$bin_dir/chart2kro" ./cmd/chart2kro)
else
  case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) echo "::error::unsupported OS $(uname -s)"; exit 1 ;;
  esac

  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) echo "::error::unsupported architecture $(uname -m)"; exit 1 ;;
  esac

  archive="chart2kro_${version}_${os}_${arch}.tar.gz"
  base="https://github.com/${repo}/releases/download/v${version}"
  work="$(mktemp -d)"

  curl -fsSL -o "$work/$archive" "$base/$archive"
  curl -fsSL -o "$work/checksums.txt" "$base/checksums.txt"

  want="$(awk -v f="$archive" '$2 == f { print $1 }' "$work/checksums.txt")"
  if command -v sha256sum >/dev/null; then
    got="$(sha256sum "$work/$archive" | cut -d' ' -f1)"
  else
    got="$(shasum -a 256 "$work/$archive" | cut -d' ' -f1)"
  fi

  if [[ -z "$want" || "$want" != "$got" ]]; then
    echo "::error::checksum mismatch for $archive (want ${want:-none}, got $got)"
    exit 1
  fi

  tar -xzf "$work/$archive" -C "$bin_dir" chart2kro
  echo "Installed chart2kro v${version} (sha256 verified)"
fi

if [[ -n "${GITHUB_PATH:-}" ]]; then
  echo "$bin_dir" >>"$GITHUB_PATH"
fi

"$bin_dir/chart2kro" version
