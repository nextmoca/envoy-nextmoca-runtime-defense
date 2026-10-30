#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
envoy_log="$(mktemp)"
render="$(mktemp)"
trap 'rm -f "$render" "$envoy_log"' EXIT

docker run --rm \
  -v "$root/examples/envoy/envoy.yaml:/etc/envoy/envoy.yaml:ro" \
  envoyproxy/envoy:v1.36.0 \
  --mode validate -c /etc/envoy/envoy.yaml >"$envoy_log" 2>&1
cat "$envoy_log"
if grep -Fq 'Deprecated field' "$envoy_log"; then
  echo "Raw Envoy example uses a deprecated configuration field" >&2
  exit 1
fi
if ! grep -Fq 'address: 127.0.0.1' "$root/examples/envoy/envoy.yaml"; then
  echo "Raw Envoy example must bind the admin interface to loopback" >&2
  exit 1
fi

helm template nextmoca "$root/charts/nextmoca-envoy-extproc" \
  --namespace ai-gateway \
  -f "$root/examples/envoy-gateway/values.yaml" >"$render"
grep -Fq 'kind: EnvoyExtensionPolicy' "$render"
grep -Fq 'name: model-provider-route' "$render"
grep -Fq 'body: Buffered' "$render"

echo "Envoy and Envoy Gateway examples validated."
