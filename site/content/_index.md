+++
title = "chart2kro: Helm charts to kro ResourceGraphDefinitions"
description = "Convert Helm charts into kro ResourceGraphDefinitions that kro accepts, that behave like the chart, and that explain every decision. Verified offline with kro's own code."
template = "index.html"

[extra]
before = """
```yaml
# values.yaml
replicaCount: 1
image:
  repository: nginx
  tag: "1.27"

# templates/deployment.yaml
metadata:
  name: {{ .Release.Name }}-nginx
spec:
  replicas: {{ .Values.replicaCount }}
  template:
    spec:
      containers:
        - image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
```
"""
after = """
```yaml
spec:
  schema:
    kind: Nginx
    spec:
      replicaCount: integer | default=1
      image:
        repository: string | default="nginx"
        tag: string | default="1.27"
  resources:
    - id: deployment
      template:
        metadata:
          name: ${schema.metadata.name}-nginx
        spec:
          replicas: ${schema.spec.replicaCount}
          template:
            spec:
              containers:
                - image: ${schema.spec.image.repository}:${schema.spec.image.tag}
```
"""
lead = "chart2kro converts a Helm chart into a kro ResourceGraphDefinition: a Kubernetes API whose instances deploy what the chart deploys. Every input is verified against Helm with kro's own runtime, and every value it does not expose comes with a reason."

[[extra.features]]
icon = "✅"
title = "Accepted by kro"
text = "Every RGD is checked offline with kro's own admission schema and graph builder before it is written. chart2kro never writes an RGD kro would reject."

[[extra.features]]
icon = "🎯"
title = "Faithful to Helm"
text = "An instance with the defaults deploys what `helm template` deploys for that release name and namespace."

[[extra.features]]
icon = "🔬"
title = "Verified inputs"
text = "A value becomes an input only after kro's runtime, run offline, reproduced Helm's output for it at several probe values."

[[extra.features]]
icon = "📋"
title = "Every decision explained"
text = "Values that are not exposed are listed as *frozen*, *failed* or *unused*, with a stable reason code and what to do about it."

[[extra.features]]
icon = "🧱"
title = "Isolated instances"
text = "Names come from the instance and namespaces from where it is created. Selectors never reach another instance's pods."

[[extra.features]]
icon = "🔐"
title = "No secret leaks"
text = "Secrets you pass with `-f` never become defaults. Passwords the chart would generate become required inputs, never derived from public data."

[[extra.steps]]
title = "Render"
text = "The chart is rendered in memory with the Helm v4 SDK, exactly like `helm template`."

[[extra.steps]]
title = "Probe"
text = "Each value is rendered again with typed probe values to see which fields it controls, and how."

[[extra.steps]]
title = "Assemble and validate"
text = "Mappings become CEL expressions, `includeWhen` conditions and schema fields. kro's own code validates the result."

[[extra.steps]]
title = "Verify"
text = "kro's runtime instantiates the RGD offline and compares it with Helm's render. Any mismatch freezes the value."

[[extra.usecases]]
title = "🎛️ Design a small API"
text = "Set platform opinions with Helm's own `-f`, choose the knobs with `--expose`. Exposure is strict: CI fails if a knob cannot be verified."
code = "chart2kro convert ./app \\\n  -f platform.yaml \\\n  --expose 'image.tag,replicaCount' \\\n  --group apps.example.com -o api/"

[[extra.usecases]]
title = "🚦 Keep it current in CI"
text = "Fail the build when the committed RGD is stale, or when a chart upgrade would break existing instances."
code = "chart2kro convert ./app -o api/ --check\nchart2kro diff ./app --version 2.0.0 \\\n  --against api/rgd.yaml"

[[extra.usecases]]
title = "🚚 Move releases to kro"
text = "Turn the values of a running Helm release into an instance of the new API. Unexposed values fail loudly instead of disappearing."
code = "helm get values web -o yaml > web.yaml\nchart2kro instance ./app \\\n  --from web.yaml --name web"

[[extra.faq]]
q = "Does chart2kro need a cluster, Helm or kubectl?"
a = "No. Loading, rendering, validation and verification all run in-process. You only need a cluster with kro to apply the result."

[[extra.faq]]
q = "Which kro versions are supported?"
a = "The generated RGDs need kro v0.9.1 or newer. chart2kro validates with the kro v0.9.4 libraries, and its end-to-end tests run on both v0.9.1 and v0.9.4."

[[extra.faq]]
q = "What happens to values chart2kro cannot translate?"
a = "They stay at their default in every instance and are listed with a reason, for example `transformed` when the chart passes the value through `upper`. See [Limitations](@/docs/limitations.md) and [Troubleshooting](@/docs/troubleshooting.md#reason-codes)."

[[extra.faq]]
q = "Is the conversion proven equivalent to the chart?"
a = "No tool can prove that for arbitrary templates. chart2kro validates each exposed value at concrete witnesses (the report counts them) and freezes anything that did not match Helm. It never guesses."

[[extra.faq]]
q = "Can I use private registries and repositories?"
a = "Yes: `oci://` references, `repo/chart` from your Helm `repositories.yaml`, `--repo` URLs, basic auth and TLS client certificates. Credentials are never written or logged."

[[extra.faq]]
q = "Can I edit the generated RGD?"
a = "Yes, it is plain YAML. Most teams keep it generated instead: commit the output, and let `convert --check` and `diff` guard it in CI."

+++
