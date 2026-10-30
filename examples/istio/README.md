# Istio Sidecar Integration

The bridge is designed for Istio **sidecar mode** when the agent's Envoy sees
the model request as HTTP. The chart injects a workload-scoped `EnvoyFilter`
that adds Envoy's `ext_proc` filter before the sidecar's outbound router.

## Recommended: govern an internal model gateway

Have agents call an internal, mesh-visible model gateway such as LiteLLM, Tyk
AI Studio, or a provider proxy over HTTP. Istio protects the service-to-service
hop with mesh mTLS while the sidecar can still inspect the HTTP request.

```bash
helm upgrade --install nextmoca \
  oci://ghcr.io/nextmoca/charts/nextmoca-envoy-extproc \
  --version 0.1.0 \
  --namespace ai-gateway --create-namespace \
  --set existingSecret.name=nextmoca-api-key \
  --set istio.enabled=true \
  --set istio.workloadSelectorLabels.app=agent \
  --set istio.outboundPort=8080
```

`istio.workloadSelectorLabels` must select only the agent workloads that call
the gateway. `outboundPort` must be the gateway's mesh-visible HTTP port. The
chart does not apply an unscoped, mesh-wide filter.

## Direct external provider with TLS origination

[`direct-provider-tls-origination.yaml`](direct-provider-tls-origination.yaml)
is an optional Istio `ServiceEntry` and `DestinationRule` for a provider host.
Apply it only after changing the namespace and provider host as appropriate:

```bash
kubectl apply -f examples/istio/direct-provider-tls-origination.yaml
```

The agent must call `http://api.openai.com` through the declared HTTP port;
Istio maps that port to `443` and originates TLS upstream. This preserves
provider-hop encryption without requiring Next Moca to decrypt TLS.

Do **not** use this recipe when the agent opens `https://api.openai.com`
directly. Its sidecar receives opaque TLS bytes, so it cannot select, scan, or
quarantine the request. Use an internal model gateway instead when the client
cannot intentionally use a mesh-visible HTTP entrypoint.

## Limits and rollout

The filter buffers complete request bodies. Keep streaming responses, WebSocket
upgrades, and other non-bufferable traffic on a bypass route. Start with
`shadow`, then use `active`, and move to `quarantine` or `enforce` only after
reviewing receipt headers and `/metrics`.
