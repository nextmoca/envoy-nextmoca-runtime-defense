# Next Moca Runtime Defense for Envoy

This repository is a thin [Envoy External Processing](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_proc_filter) bridge for the hosted Next Moca Runtime Defense API.

It gives an Envoy route a last-hop control point before an AI-provider request leaves the gateway:

```text
Agent or application request
  -> Envoy / Envoy Gateway
  -> Next Moca ExtProc bridge
  -> hosted Next Moca Select, Scan, Enforce API
  -> model provider
```

The bridge contains **no Next Moca selection, scanning, or enforcement engine**. It only:

- translates supported provider request shapes into the hosted API contract;
- validates the hosted response;
- rewrites selected original text into the provider request;
- carries a compact receipt as upstream request headers; and
- returns a deterministic pass-through or block when the configured delivery policy requires it.

## Supported Provider Shapes

| Adapter | Request shape | Auto detection |
|---|---|---|
| `openai_chat` | Chat Completions `messages` | Set explicitly, because a bare `messages` array is ambiguous |
| `openai_responses` | Responses API `input` | Yes |
| `anthropic_messages` | Messages API `messages` plus `anthropic-version` | Yes |
| `gemini_generate_content` | Gemini `contents[].parts[].text` | Yes |

Only scalar text context is rewritten. Multimodal blocks and unsupported request shapes are left untouched by default, or rejected in screening modes when `NEXTMOCA_UNSUPPORTED_SHAPE=block`.

## Quick Start

Build the production image locally:

```bash
docker build --target runtime -t nextmoca/envoy-extproc:local .
```

The bridge requires a hosted API key at runtime. Never bake a key into the image or a ConfigMap.

```bash
docker run --rm \
  -e NEXTMOCA_API_KEY="$NEXTMOCA_API_KEY" \
  -e NEXTMOCA_MODE=quarantine \
  -e NEXTMOCA_REQUEST_ADAPTER=openai_chat \
  -p 9002:9002 -p 8080:8080 \
  nextmoca/envoy-extproc:local
```

`GET /healthz` is served on port `8080`; the Envoy ExtProc gRPC service listens on port `9002`.

## Helm Installation

The production distribution is an OCI Helm chart. It deploys only this thin
bridge and requires the API key to already exist in a Kubernetes Secret.

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

The release workflow publishes the image and chart when a `v*` Git tag is
created. Until that first release tag exists, build the image locally and use
the checked-out chart with `helm upgrade --install nextmoca charts/nextmoca-envoy-extproc ...`.

Release tags publish a multi-architecture (`linux/amd64`, `linux/arm64`)
distroless bridge image to
`ghcr.io/nextmoca/envoy-nextmoca-runtime-defense` and the matching chart to
`oci://ghcr.io/nextmoca/charts/nextmoca-envoy-extproc`. The image includes an
SBOM and build provenance attestation and is signed by digest using GitHub
Actions keyless Cosign identity.

The bridge serves bounded Prometheus metrics on `/metrics`. Its metric labels
never include prompts, selected records, user data, API keys, or request IDs.

## Envoy Gateway

The Kubernetes example in [`deploy/envoy-gateway`](deploy/envoy-gateway) deploys the bridge and attaches it to an `HTTPRoute` through an `EnvoyExtensionPolicy`.

Build and publish the thin bridge image from this repository, then replace
`REPLACE_WITH_PUBLISHED_BRIDGE_IMAGE` in `deployment.yaml` with that immutable
image reference before applying the manifests.

```bash
kubectl apply -f deploy/envoy-gateway/secret.example.yaml  # replace the placeholder first
kubectl apply -f deploy/envoy-gateway/deployment.yaml
kubectl apply -f deploy/envoy-gateway/extension-policy.yaml
```

Set `REPLACE_WITH_MODEL_ROUTE` to the name of the target provider `HTTPRoute` before applying the policy. The policy deliberately uses `request.body: Buffered`. The bridge rejects partial bodies rather than selecting from incomplete input, and buffered request processing is incompatible with streaming requests and protocol upgrades such as WebSockets.

Use `failOpen: false` with `quarantine` or `enforce` for a fail-closed security boundary. Use `failOpen: true` only for shadow or active-selection routes where pass-through is the explicit availability policy.

## Runtime Configuration

| Variable | Default | Purpose |
|---|---|---|
| `NEXTMOCA_API_KEY` | required | Hosted Next Moca API key |
| `NEXTMOCA_ENDPOINT` | `https://api.nextmoca.com` | Hosted service origin |
| `NEXTMOCA_MODE` | `shadow` | `shadow`, `active`, `quarantine`, or `enforce` |
| `NEXTMOCA_REQUEST_ADAPTER` | `auto` | Provider adapter listed above |
| `NEXTMOCA_UNSUPPORTED_SHAPE` | `pass` | `pass` or `block` malformed/unsupported request shapes |
| `NEXTMOCA_ON_SELECTION_ERROR` | `pass` | `pass` or `block` when the hosted service is unavailable |
| `NEXTMOCA_MAX_CONTEXT_TOKENS` | `8192` | Target selected-context allowance |
| `NEXTMOCA_CAPACITY_TOKENS` | `128000` | Downstream available input capacity |
| `NEXTMOCA_OPERATING_POINT` | `np-2026-10-r8` | Hosted operating point |
| `NEXTMOCA_TIMEOUT` | `3s` | One hosted request deadline; no bridge retries |

The bridge sends `x-nextmoca-decision`, `x-nextmoca-outcome`, token counts, and, when supplied, `x-nextmoca-selection-id` to the upstream provider as receipt headers. These headers are never returned to the original caller by the bridge.

## Deployment Recipes

- [`examples/envoy`](examples/envoy) provides a validated raw Envoy ExtProc
  filter configuration for a narrow `/v1/` provider route.
- [`examples/envoy-gateway`](examples/envoy-gateway) provides values for a
  route-scoped `EnvoyExtensionPolicy` through Envoy Gateway.
- [`examples/istio`](examples/istio) provides a workload-scoped Istio sidecar
  configuration for mesh-visible HTTP model-gateway traffic and an optional
  external-provider TLS-origination recipe.

For either deployment surface, roll out one provider route at a time:
`shadow` first, then `active`, followed by `quarantine` or `enforce` only after
the gateway receipts and `/metrics` output are understood. Buffered request
bodies are required; streaming requests and WebSocket upgrades should use a
separate route that bypasses this bridge.

## Istio sidecars

Istio sidecars can govern an agent's model request when the sidecar observes
HTTP, typically because the agent calls an internal model gateway. Enable the
chart's opt-in `istio` section with a workload selector and the gateway's
outbound HTTP port. Direct application-issued HTTPS to an external provider is
encrypted before it reaches the sidecar and is intentionally unsupported. The
optional TLS-origination recipe in [`examples/istio`](examples/istio) preserves
HTTPS on the provider hop while giving Istio an HTTP request to govern.

## Verification

```bash
make test
make chart-lint
make chart-test
make e2e
```

`make e2e` launches four local containers: Envoy, the thin bridge, a deterministic hosted-service fixture, and a provider fixture. It proves that Envoy buffers a real request, the ExtProc stream rewrites it, unsafe fixture text is absent from the forwarded request, and the provider receives the Next Moca receipt.

To verify the bridge against the real hosted API while retaining a local provider fixture:

```bash
export NEXTMOCA_API_KEY='...'
make e2e-hosted
```

This command does not send traffic to a third-party model. It proves the gateway-to-hosted-Next-Moca-to-upstream request path and receipt propagation.

To run the Gateway API policy against a disposable local Kind cluster, install [Kind](https://kind.sigs.k8s.io/) and Helm, then run:

```bash
make e2e-envoy-gateway
```

It installs Envoy Gateway `v1.9.0`, deploys the bridge plus deterministic fixtures, attaches the included `EnvoyExtensionPolicy`, sends an OpenAI-compatible request through the Gateway data plane, and then deletes the cluster.

## Verified Release Gate

The following gate passed on October 30, 2026 for this repository's bridge,
chart, and examples:

```bash
go test ./... -count=1
go vet ./...
make build chart-lint chart-test
./scripts/test-examples.sh
./scripts/test-workflows.sh
make e2e
NEXTMOCA_API_KEY="$NEXTMOCA_API_KEY" make e2e-hosted
E2E_RESULT_FILE=/tmp/nextmoca-envoy-gateway-result make e2e-envoy-gateway
```

The local Envoy test verifies buffered-body mutation and receipt propagation.
The hosted test calls the hosted Next Moca API while retaining a local provider
fixture, so it does not invoke a third-party model. The Gateway test provisions
a disposable Kind cluster, installs Envoy Gateway and the route-scoped policy,
then verifies the same governed request path before deleting the cluster.

## Development

```bash
go test ./... -count=1
docker build --target runtime -t nextmoca/envoy-extproc:local .
```

The Apache-2.0-licensed source in this repository is limited to the bridge and its tests, fixtures, deployment manifests, and documentation.
