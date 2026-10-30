# Raw Envoy ExtProc Recipe

Use this recipe when you manage Envoy through static configuration or xDS rather than Envoy Gateway.

1. Deploy the Next Moca bridge with the Helm chart or an equivalent workload. It must be resolvable by Envoy over gRPC/h2c.
2. Copy `envoy.yaml` into your Envoy configuration and replace the bridge and model-provider DNS names.
3. Restrict the ExtProc filter to model-provider routes only. The example uses `/v1/` as a narrow starting point.
4. Start with `failure_mode_allow: true` and `NEXTMOCA_MODE=shadow`. Once receipts and `/metrics` show expected behavior, change the route and bridge together through `active`, `quarantine`, and finally `enforce`.

`request_body_mode: BUFFERED` is deliberate. The bridge will not make a context decision over a partial provider request. Buffered request bodies are not compatible with request streaming or protocol upgrades.

For a fail-closed security route, use `failure_mode_allow: false` with `NEXTMOCA_MODE=quarantine` or `enforce` and `NEXTMOCA_ON_SELECTION_ERROR=block`.
