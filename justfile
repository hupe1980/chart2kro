# chart2kro justfile
# https://github.com/casey/just

VERSION := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
COMMIT  := `git rev-parse --short HEAD 2>/dev/null || echo "none"`
DATE    := `date -u +"%Y-%m-%dT%H:%M:%SZ"`
LDFLAGS := "-s -w -X github.com/hupe1980/chart2kro/internal/version.version=" + VERSION + " -X github.com/hupe1980/chart2kro/internal/version.gitCommit=" + COMMIT + " -X github.com/hupe1980/chart2kro/internal/version.buildDate=" + DATE

# Default target
default: build

# Build the chart2kro binary with version ldflags
build:
    CGO_ENABLED=0 go build -ldflags '{{LDFLAGS}}' -o chart2kro ./cmd/chart2kro/

# Run all tests with race detector
test:
    go test -race -count=1 ./...

# Run golangci-lint
lint:
    golangci-lint run

# Format Go source files
fmt:
    gofmt -w .
    goimports -w .

# Run go vet
vet:
    go vet ./...

# Remove build artifacts
clean:
    rm -f chart2kro
    rm -rf dist/
    rm -f coverage.out coverage.html

# Generate HTML coverage report
coverage:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report: coverage.html"

# Run all checks (lint + test + vet)
check: lint vet test
