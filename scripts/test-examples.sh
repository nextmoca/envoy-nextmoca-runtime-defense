#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
envoy_log="$(mktemp)"
render="$(mktemp)"
trap 'rm -f "$render" "$envoy_log"' EXIT

ruby -ryaml - "$root/examples/istio/direct-provider-tls-origination.yaml" <<'RUBY'
documents = YAML.load_stream(File.read(ARGV.fetch(0)))
service_entry = documents.find { |document| document["kind"] == "ServiceEntry" }
destination_rule = documents.find { |document| document["kind"] == "DestinationRule" }

abort "Istio recipe must define a ServiceEntry" unless service_entry
abort "Istio recipe must define a DestinationRule" unless destination_rule

http_port = service_entry.dig("spec", "ports")&.find { |port| port["number"] == 80 }
abort "Istio ServiceEntry must expose plaintext HTTP on port 80" unless http_port&.fetch("protocol", nil) == "HTTP"
abort "Istio ServiceEntry must route HTTP to provider port 443" unless http_port["targetPort"] == 443

tls = destination_rule.dig("spec", "trafficPolicy", "portLevelSettings")&.find { |setting| setting.dig("port", "number") == 80 }&.dig("tls")
abort "Istio DestinationRule must originate TLS from port 80" unless tls&.fetch("mode", nil) == "SIMPLE"
RUBY

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
