# Microservice Example

A production-ready microservice with Ingress, HorizontalPodAutoscaler, and environment configuration — demonstrating a realistic multi-resource conversion.

## Chart Resources

| Resource | Description |
|----------|-------------|
| Deployment | Application container with health checks |
| Service | ClusterIP service |
| Ingress | External HTTP routing |
| HorizontalPodAutoscaler | CPU-based autoscaling |
| ConfigMap | Application environment variables |

## Convert

Run all commands from the **project root**:

```bash
# Generate the RGD
go run ./cmd/chart2kro/ convert examples/microservice/chart/ \
  --kind Microservice \
  --group apps.kro.run \
  -o examples/microservice/rgd.yaml

# With security hardening
go run ./cmd/chart2kro/ convert examples/microservice/chart/ \
  --kind Microservice \
  --group apps.kro.run \
  --harden \
  -o examples/microservice/rgd-hardened.yaml

# Inspect the chart
go run ./cmd/chart2kro/ inspect examples/microservice/chart/
```

## Apply

```bash
# Install the RGD into your cluster (requires KRO)
kubectl apply -f examples/microservice/rgd.yaml

# Create an instance
kubectl apply -f - <<EOF
apiVersion: apps.kro.run/v1alpha1
kind: Microservice
metadata:
  name: my-api
spec:
  replicaCount: 3
  image:
    repository: myorg/api-server
    tag: "1.0.0"
  service:
    port: 8080
  ingress:
    host: api.example.com
  autoscaling:
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilization: 80
EOF
```

## Schema Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `replicaCount` | integer | `2` | Base replica count (when HPA disabled) |
| `image.repository` | string | `myorg/api-server` | Container image |
| `image.tag` | string | `1.0.0` | Image tag |
| `service.port` | integer | `8080` | Service port |
| `ingress.host` | string | `api.example.com` | Ingress hostname |
| `autoscaling.minReplicas` | integer | `2` | HPA minimum replicas |
| `autoscaling.maxReplicas` | integer | `10` | HPA maximum replicas |
| `autoscaling.targetCPUUtilization` | integer | `75` | CPU target percentage |
