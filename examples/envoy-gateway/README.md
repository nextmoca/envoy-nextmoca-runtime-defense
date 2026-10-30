# Envoy Gateway Recipe

This values file adds a Next Moca `EnvoyExtensionPolicy` to one provider `HTTPRoute`.

```bash
helm upgrade --install nextmoca \
  oci://ghcr.io/nextmoca/charts/nextmoca-envoy-extproc \
  --version 0.1.0 \
  --namespace ai-gateway --create-namespace \
  -f examples/envoy-gateway/values.yaml
```

Replace `customer-nextmoca-key` with a pre-existing Secret and
`model-provider-route` with the name of the `HTTPRoute` that forwards model
traffic. The policy always requests a buffered body from Envoy Gateway, calls
the bridge on gRPC port `9002`, and uses `failOpen: false` in this security
example.

For a non-disruptive rollout, deploy the same route in `shadow` mode first.
Set `envoyGateway.failOpen=true` only for `shadow` or `active` routes where
pass-through is intentional. Use `quarantine` or `enforce` with `failOpen=false`
only after validating receipts and metrics on representative traffic.
