# Examples

Working examples demonstrating chart2kro converting Helm charts into KRO ResourceGraphDefinitions.

## Quick Start

Run all commands from the **project root**:

```bash
go run ./cmd/chart2kro/ convert examples/nginx/chart/ --kind NginxApp --group nginx.kro.run
```

## Examples

| Example | Description | Resources |
|---------|-------------|-----------|
| [nginx](nginx/) | Simple web server with ConfigMap | Deployment, Service, ConfigMap |
| [redis](redis/) | Stateful cache with persistence | StatefulSet, Service, Headless Service, ConfigMap |
| [microservice](microservice/) | Production app with Ingress & HPA | Deployment, Service, Ingress, HPA, ConfigMap |

## Regenerating RGDs

Each example includes a pre-generated `rgd.yaml`. To regenerate:

```bash
# Nginx
go run ./cmd/chart2kro/ convert examples/nginx/chart/ --kind NginxApp --group nginx.kro.run -o examples/nginx/rgd.yaml

# Redis
go run ./cmd/chart2kro/ convert examples/redis/chart/ --kind RedisCache --group redis.kro.run -o examples/redis/rgd.yaml

# Microservice
go run ./cmd/chart2kro/ convert examples/microservice/chart/ --kind Microservice --group apps.kro.run -o examples/microservice/rgd.yaml
```

## Trying Other Features

```bash
# Inspect chart before converting
go run ./cmd/chart2kro/ inspect examples/nginx/chart/

# Convert with security hardening
go run ./cmd/chart2kro/ convert examples/microservice/chart/ --kind Microservice --group apps.kro.run --harden

# Diff against a previous RGD
go run ./cmd/chart2kro/ diff examples/nginx/chart/ --existing examples/nginx/rgd.yaml

# Audit for security issues
go run ./cmd/chart2kro/ audit examples/microservice/chart/

# Generate docs for the custom resource
go run ./cmd/chart2kro/ docs examples/nginx/chart/ --kind NginxApp
```
