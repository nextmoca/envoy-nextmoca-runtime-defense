#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
resources="$root/e2e/istio/resources.yaml"
script="$root/scripts/e2e-istio.sh"

ruby -ryaml - "$resources" <<'RUBY'
documents = YAML.load_stream(File.read(ARGV.fetch(0)))
abort "Istio E2E must install the chart rather than duplicate an EnvoyFilter fixture" if documents.any? { |document| document["kind"] == "EnvoyFilter" }
agent = documents.find { |document| document.dig("kind") == "Deployment" && document.dig("metadata", "name") == "agent-e2e" }
abort "Istio E2E resources must define the injected agent fixture" unless agent
RUBY

grep -Fq 'helm upgrade --install istio-base' "$script"
grep -Fq 'helm upgrade --install istiod' "$script"
grep -Fq 'helm upgrade --install nextmoca "$root/charts/nextmoca-envoy-extproc"' "$script"
grep -Fq -- '--set istio.enabled=true' "$script"
grep -Fq 'context="kind-$cluster"' "$script"
grep -Fq 'kubectl --context "$context"' "$script"
grep -Fq -- '--kube-context "$context"' "$script"
grep -Fq 'kind create cluster' "$script"
grep -Fq 'X-Nextmoca-Tokens-Saved' "$script"

echo "Istio E2E fixture contract validated."
