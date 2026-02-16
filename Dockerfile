# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Build stage
# ---------------------------------------------------------------------------
FROM golang:1-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w \
      -X github.com/hupe1980/chart2kro/internal/version.version=${VERSION} \
      -X github.com/hupe1980/chart2kro/internal/version.gitCommit=${COMMIT} \
      -X github.com/hupe1980/chart2kro/internal/version.buildDate=${BUILD_DATE}" \
    -o /chart2kro ./cmd/chart2kro/

# ---------------------------------------------------------------------------
# Final stage — distroless for minimal attack surface
# ---------------------------------------------------------------------------
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /chart2kro /chart2kro

USER nonroot:nonroot

ENTRYPOINT ["/chart2kro"]
