# Istio Sidecar Integration Design

## Intent

Make the public Next Moca Envoy ExtProc bridge deployable with Istio sidecars
without moving any hosted selection, scanning, or enforcement logic into the
image. An opted-in agent workload must be able to send an OpenAI-compatible
request through its injected Envoy sidecar, have Next Moca govern the complete
request body, and forward only the governed request to a mesh-visible model
gateway.

## Supported Traffic Boundary

Istio's `EnvoyFilter` can add an HTTP filter only when the workload sidecar
sees HTTP. The first release therefore supports these two deliberate paths:

1. **Recommended: internal model gateway.** An agent calls a mesh service such
   as LiteLLM, Tyk AI Studio, or an organization model gateway over HTTP. Istio
   applies the ExtProc filter to that outbound HTTP listener. Mesh mTLS protects
   service-to-service transit as normal.
2. **Optional: direct external provider with Istio TLS origination.** The
   application calls an HTTP ServiceEntry port; Istio originates TLS on the
   provider hop. The application must intentionally use the plaintext HTTP
   entrypoint. This is not a workaround for an application-issued HTTPS call.

An application that opens TLS directly to an external provider presents opaque
TLS bytes to the sidecar. That traffic is intentionally unsupported: the
integration must not claim it can select or scan context it cannot inspect.

## Delivery Shape

- Extend the Helm chart with an opt-in `istio` section. It creates one
  workload-scoped `EnvoyFilter` only when `istio.enabled=true`.
- Match `SIDECAR_OUTBOUND` HTTP traffic on an explicit model-gateway port and
  insert Envoy's `envoy.filters.http.ext_proc` directly before the router.
- Use Istio's existing outbound cluster for the bridge service:
  `outbound|<grpcPort>||<bridgeService>.<bridgeNamespace>.svc.cluster.local`.
  The bridge Service's gRPC port is explicitly named `grpc`, which lets Istio
  configure HTTP/2 upstream handling.
- Buffer only request bodies. Skip response processing. A bounded `3s`
  message timeout and `failOpen` setting mirror the selected delivery mode.
- Scope every filter with a workload selector and one outbound port. Never
  create a mesh-global filter and never match the bridge workload itself.

## Configuration Contract

The Helm values are additive and disabled by default:

```yaml
istio:
  enabled: false
  workloadSelectorLabels: {}
  outboundPort: 8080
  bridgeNamespace: ""
  messageTimeout: 3s
  failOpen: false
```

`workloadSelectorLabels` is required when enabled. `bridgeNamespace` defaults
to the Helm release namespace. The optional direct-provider recipe is a
separate manifest, not a chart default, because its external host and
TLS-origination policy are customer-specific.

## Safety and Operations

- `failOpen: false` is appropriate for `quarantine`/`enforce`; `true` is only
  appropriate for deliberate shadow or active-selection availability policies.
- Buffered processing does not support streaming provider requests or protocol
  upgrades. Customers must keep such traffic on a bypass route.
- The release includes a Kind + Helm + Istio E2E test. It verifies the
  sidecar-injected client reaches a provider fixture only after the bridge
  removes unsafe tool text and propagates the receipt headers.
- Istio's EnvoyFilter API is powerful and tied to the generated Envoy config;
  the example is version-tested and should be retested when Istio is upgraded.

## Non-Goals

- TLS interception or decryption of arbitrary provider traffic.
- Ambient-mode waypoint support; this release is for injected Envoy sidecars.
- A new public Next Moca API, a second bridge binary, or core selection code in
  the distribution image.
