#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
resources="$root/e2e/istio/resources.yaml"
script="$root/scripts/e2e-istio.sh"

ruby -ryaml - "$resources" <<'RUBY'
documents = YAML.load_stream(File.read(ARGV.fetch(0)))
envoy_filter = documents.find { |document| document["kind"] == "EnvoyFilter" }
abort "Istio E2E resources must define an EnvoyFilter" unless envoy_filter
abort "Istio E2E filter must target outbound sidecar traffic" unless envoy_filter.to_s.include?("SIDECAR_OUTBOUND")
abort "Istio E2E filter must buffer complete request bodies" unless envoy_filter.to_s.include?("BUFFERED")
abort "Istio E2E filter must target only the agent fixture" unless envoy_filter.dig("spec", "workloadSelector", "labels", "app") == "agent-e2e"
RUBY

grep -Fq 'helm upgrade --install istio-base' "$script"
grep -Fq 'helm upgrade --install istiod' "$script"
grep -Fq 'kind create cluster' "$script"
grep -Fq 'X-Nextmoca-Tokens-Saved' "$script"

echo "Istio E2E fixture contract validated."
