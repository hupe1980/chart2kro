# Redis Example

Redis in-memory data store with persistence via StatefulSet, demonstrating stateful workload conversion.

## Chart Resources

| Resource | Description |
|----------|-------------|
| StatefulSet | Redis server with persistent storage |
| Service | ClusterIP service on port 6379 |
| Service (headless) | Headless service for StatefulSet DNS |
| ConfigMap | Redis server configuration |

## Convert

Run all commands from the **project root**:

```bash
# Generate the RGD
go run ./cmd/chart2kro/ convert examples/redis/chart/ \
  --kind RedisCache \
  --group redis.kro.run \
  -o examples/redis/rgd.yaml

# Inspect the chart
go run ./cmd/chart2kro/ inspect examples/redis/chart/
```

## Apply

```bash
# Install the RGD into your cluster (requires KRO)
kubectl apply -f examples/redis/rgd.yaml

# Create an instance
kubectl apply -f - <<EOF
apiVersion: redis.kro.run/v1alpha1
kind: RedisCache
metadata:
  name: my-redis
spec:
  image:
    repository: redis
    tag: "7.4-alpine"
  persistence:
    size: 5Gi
  service:
    port: 6379
EOF
```

## Schema Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `image.repository` | string | `redis` | Container image |
| `image.tag` | string | `7.4-alpine` | Image tag |
| `persistence.size` | string | `1Gi` | PVC storage size |
| `service.port` | integer | `6379` | Service port |
| `maxmemoryPolicy` | string | `allkeys-lru` | Redis eviction policy |
