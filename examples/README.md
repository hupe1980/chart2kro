# Examples

Each directory holds a chart (`chart/`) and the files `chart2kro convert` generates from it: `rgd.yaml`, `instance.yaml` and `report.json`.

```bash
just examples        # regenerate all
```

| Example | Shows | Resources | Inputs |
|---|---|---|---|
| [nginx](nginx/) | A web server with a ConfigMap-mounted configuration: the smallest useful conversion. | 3 | 10 |
| [redis](redis/) | A StatefulSet with a headless Service and persistence: shows readiness for StatefulSets and isolated instances of a stateful workload. | 4 | 11 |
| [microservice](microservice/) | A Deployment with Ingress and an HPA behind feature flags: shows booleans becoming `includeWhen` and CEL conditionals. | 5 | 16 |

Try one on a cluster with kro ≥ v0.9.2:

```bash
kubectl apply -f examples/redis/rgd.yaml
kubectl apply -f examples/redis/instance.yaml
kubectl get redis
```
