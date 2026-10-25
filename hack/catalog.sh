#!/usr/bin/env bash
# Regenerate the embedded OpenAPI catalogs: for each kind node image tag (e.g. v1.37.0),
# start a cluster, dump /openapi/v3 and discovery, minify, and write
# internal/kroval/data/{catalog,discovery}/v<major>.<minor>/.
set -euo pipefail
cd "$(dirname "$0")/.."
for tag in "$@"; do
  minor=$(echo "$tag" | sed -E 's/^v([0-9]+\.[0-9]+).*/\1/')
  name="c2k-catalog-${minor//./-}"
  tmp=$(mktemp -d)
  kind create cluster --name "$name" --image "kindest/node:$tag" --wait 120s
  ctx="kind-$name"
  kubectl --context "$ctx" get --raw /openapi/v3 > "$tmp/index.json"
  for p in $(go run ./hack/jsonkeys "$tmp/index.json" paths); do
    kubectl --context "$ctx" get --raw "/openapi/v3/$p" > "$tmp/$(echo "$p" | sed 's#/#__#g')_openapi.json"
  done
  out=internal/kroval/data/catalog/v$minor disc=internal/kroval/data/discovery/v$minor
  rm -rf "$out" "$disc"; mkdir -p "$disc"
  go run ./hack/minify "$tmp" "$out"
  kubectl --context "$ctx" get --raw /api/v1 > "$disc/api_v1.json"
  for gv in $(kubectl --context "$ctx" api-versions | grep /); do
    kubectl --context "$ctx" get --raw "/apis/$gv" > "$disc/apis_$(echo "$gv" | sed 's#/#_#g').json"
  done
  kind delete cluster --name "$name"
  rm -rf "$tmp"
done
