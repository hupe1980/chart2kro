# chart2kro — https://github.com/casey/just

VERSION := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
COMMIT  := `git rev-parse --short HEAD 2>/dev/null || echo "unknown"`
DATE    := `date -u +"%Y-%m-%dT%H:%M:%SZ"`
LDFLAGS := "-s -w -X github.com/hupe1980/chart2kro/internal/version.version=" + VERSION + " -X github.com/hupe1980/chart2kro/internal/version.gitCommit=" + COMMIT + " -X github.com/hupe1980/chart2kro/internal/version.buildDate=" + DATE
KRO_VERSION := env_var_or_default("KRO_VERSION", "0.9.4")

default: check

# Build the static binary.
build:
    CGO_ENABLED=0 go build -trimpath -ldflags '{{LDFLAGS}}' -o chart2kro ./cmd/chart2kro

# Unit, golden and integration tests with the race detector.
test:
    go test -race -count=1 ./...

# Rewrite golden files after an intended output change (review the diff!).
golden:
    go test ./pkg/chart2kro -run TestGolden -update

lint:
    golangci-lint run ./...

# Per-package coverage floor (80 %).
cover:
    go test -coverpkg=./internal/...,./pkg/... -coverprofile=coverage.out ./internal/... ./pkg/... >/dev/null
    go run ./hack/coverfloor -profile coverage.out -min 80

vuln:
    go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

# Convert the pinned real-world charts (network).
corpus:
    go test -tags corpus -count=1 -timeout 60m -v ./test/corpus

# End-to-end on kind with kro {{KRO_VERSION}} (needs docker, kind, helm, kubectl).
e2e:
    kind create cluster --name chart2kro-e2e --wait 120s || true
    helm upgrade --install kro oci://registry.k8s.io/kro/charts/kro --version {{KRO_VERSION}} -n kro-system --create-namespace --wait --kube-context kind-chart2kro-e2e
    kind get kubeconfig --name chart2kro-e2e > .e2e-kubeconfig
    KUBECONFIG=$PWD/.e2e-kubeconfig go test -tags e2e -count=1 -timeout 30m -v ./test/e2e

e2e-down:
    kind delete cluster --name chart2kro-e2e

# Regenerate the embedded OpenAPI catalogs (one kind cluster per Kubernetes minor).
catalog *MINORS="v1.35.8 v1.36.4 v1.37.0":
    ./hack/catalog.sh {{MINORS}}

# Regenerate examples/ from examples/*/chart.
examples:
    for d in examples/*/; do d="${d%/}"; go run ./cmd/chart2kro convert "$d/chart" -o "$d" -q; done

# Documentation site with live reload (needs zola).
site:
    zola --root site serve

# Build the site into site/public.
site-build:
    zola --root site build

# Everything CI runs, except e2e and corpus.
check: lint test cover
