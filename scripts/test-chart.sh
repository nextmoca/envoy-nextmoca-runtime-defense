#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
chart="$root/charts/nextmoca-envoy-extproc"
default_render="$(mktemp)"
gateway_render="$(mktemp)"
trap 'rm -f "$default_render" "$gateway_render"' EXIT

helm template nextmoca "$chart" --namespace ai-gateway >"$default_render"

grep -Fq 'name: customer-nextmoca-key' "$default_render"
grep -Fq 'key: api-key' "$default_render"
grep -Fq 'name: grpc' "$default_render"
grep -Fq 'containerPort: 9002' "$default_render"
grep -Fq 'readOnlyRootFilesystem: true' "$default_render"
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

echo "Helm chart render tests passed."
