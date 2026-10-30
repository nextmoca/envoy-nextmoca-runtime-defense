#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
chart="$root/charts/nextmoca-envoy-extproc"
default_render="$(mktemp)"
gateway_render="$(mktemp)"
istio_render="$(mktemp)"
istio_override_render="$(mktemp)"
trap 'rm -f "$default_render" "$gateway_render" "$istio_render" "$istio_override_render"' EXIT

helm template nextmoca "$chart" --namespace ai-gateway >"$default_render"

grep -Fq 'name: customer-nextmoca-key' "$default_render"
grep -Fq 'key: api-key' "$default_render"
grep -Fq 'name: grpc' "$default_render"
grep -Fq 'appProtocol: grpc' "$default_render"
grep -Fq 'containerPort: 9002' "$default_render"
grep -Fq 'readOnlyRootFilesystem: true' "$default_render"
grep -Fq 'runAsUser: 65532' "$default_render"
grep -Fq 'drop:' "$default_render"
grep -Fq 'automountServiceAccountToken: false' "$default_render"

if helm template nextmoca "$chart" --set-string existingSecret.name='' >/dev/null 2>&1; then
  echo "Helm chart must reject an empty existingSecret.name" >&2
  exit 1
fi

helm template nextmoca "$chart" --namespace ai-gateway -f "$chart/tests/gateway-values.yaml" >"$gateway_render"
grep -Fq 'kind: EnvoyExtensionPolicy' "$gateway_render"
grep -Fq 'name: model-provider-route' "$gateway_render"
grep -Fq 'body: Buffered' "$gateway_render"
grep -Fq 'messageTimeout: 3s' "$gateway_render"
grep -Fq 'failOpen: false' "$gateway_render"

helm template nextmoca "$chart" --namespace ai-gateway -f "$chart/tests/istio-values.yaml" >"$istio_render"
grep -Fq 'kind: EnvoyFilter' "$istio_render"
grep -Fq 'namespace: agent-workloads' "$istio_render"
grep -Fq 'app: model-agent' "$istio_render"
grep -Fq 'context: SIDECAR_OUTBOUND' "$istio_render"
grep -Fq 'portNumber: 8080' "$istio_render"
grep -Fq 'name: envoy.filters.http.ext_proc' "$istio_render"
grep -Fq 'request_body_mode: BUFFERED' "$istio_render"
grep -Fq 'message_timeout: 3s' "$istio_render"
grep -Fq 'failure_mode_allow: false' "$istio_render"
grep -Fq 'outbound|9002||nextmoca-nextmoca-envoy-extproc.ai-gateway.svc.cluster.local' "$istio_render"
grep -Fq 'sidecar.istio.io/inject: "false"' "$istio_render"

helm template nextmoca "$chart" --namespace ai-gateway \
  -f "$chart/tests/istio-values.yaml" \
  -f "$chart/tests/istio-bridge-annotation-override-values.yaml" >"$istio_override_render"
if [[ "$(grep -Fc 'sidecar.istio.io/inject:' "$istio_override_render")" -ne 1 ]] || ! grep -Fq 'sidecar.istio.io/inject: "false"' "$istio_override_render"; then
  echo "Istio bridge injection must remain disabled even when podAnnotations request injection" >&2
  exit 1
fi

if helm template nextmoca "$chart" --set istio.enabled=true >/dev/null 2>&1; then
  echo "Helm chart must reject Istio configuration without workload selector labels" >&2
  exit 1
fi

echo "Helm chart render tests passed."
