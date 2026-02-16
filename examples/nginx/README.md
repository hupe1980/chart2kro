# Nginx Example

A simple NGINX web server with custom configuration, demonstrating the basic chart2kro conversion workflow.

## Chart Resources

| Resource | Description |
|----------|-------------|
| Deployment | NGINX container with health checks and resource limits |
| Service | ClusterIP service exposing port 80 |
| ConfigMap | Custom NGINX server configuration |

## Convert

Run all commands from the **project root**:

```bash
# Generate the RGD
go run ./cmd/chart2kro/ convert examples/nginx/chart/ \
  --kind NginxApp \
  --group nginx.kro.run \
  -o examples/nginx/rgd.yaml

# Preview without writing
go run ./cmd/chart2kro/ convert examples/nginx/chart/ --dry-run

# Inspect the chart
go run ./cmd/chart2kro/ inspect examples/nginx/chart/
```

## Apply

```bash
# Install the RGD into your cluster (requires KRO)
kubectl apply -f examples/nginx/rgd.yaml

# Create an instance
kubectl apply -f - <<EOF
apiVersion: nginx.kro.run/v1alpha1
kind: NginxApp
metadata:
  name: my-nginx
spec:
  replicaCount: 2
  image:
    repository: nginx
    tag: "1.27"
  service:
    type: ClusterIP
    port: 80
EOF
```

## Schema Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `replicaCount` | integer | `1` | Number of replicas |
| `image.repository` | string | `nginx` | Container image |
| `image.tag` | string | `1.27` | Image tag |
| `service.type` | string | `ClusterIP` | Service type |
| `service.port` | integer | `80` | Service port |
