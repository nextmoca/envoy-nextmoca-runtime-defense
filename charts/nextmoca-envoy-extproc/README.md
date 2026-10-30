# Next Moca Envoy ExtProc

This OCI Helm chart deploys only the thin Next Moca Envoy External Processing bridge. It does not include the hosted Next Moca selection, scanning, or enforcement engine.

## Prerequisites

- Kubernetes 1.26 or later.
- A Next Moca API key stored in an existing Secret. The chart never creates or accepts the API key value.
- Envoy Gateway and its CRDs when `envoyGateway.enabled=true`.

```bash
kubectl create secret generic nextmoca-api-key \
  --from-literal=api-key="$NEXTMOCA_API_KEY" \
  --namespace ai-gateway

helm upgrade --install nextmoca \
  oci://ghcr.io/nextmoca/charts/nextmoca-envoy-extproc \
  --version 0.1.0 \
  --namespace ai-gateway --create-namespace \
  --set existingSecret.name=nextmoca-api-key
```

## Envoy Gateway attachment

Target a single provider `HTTPRoute`; do not attach the policy to unrelated application routes.

```bash
helm upgrade --install nextmoca \
  oci://ghcr.io/nextmoca/charts/nextmoca-envoy-extproc \
  --version 0.1.0 \
  --namespace ai-gateway \
  --set existingSecret.name=nextmoca-api-key \
  --set envoyGateway.enabled=true \
  --set envoyGateway.targetRef.name=model-provider-route
```

The policy always uses a buffered request body because the bridge must decide over the complete provider request. Use `shadow` first, then target `active`, `quarantine`, or `enforce` to the same route after observing receipts and metrics.

## Observability and security

- `GET /healthz` supplies liveness and readiness probes.
- `GET /metrics` supplies bounded Prometheus metrics and never includes prompts, record IDs, user data, API keys, or request IDs as labels.
- `networkPolicy.enabled=false` by default. When enabling it, define both ingress from Envoy and egress to DNS plus `api.nextmoca.com` through your cluster's approved egress mechanism.
- `serviceMonitor.enabled=true` requires the Prometheus Operator CRDs.
