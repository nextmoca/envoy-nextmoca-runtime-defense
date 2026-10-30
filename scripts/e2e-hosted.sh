#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${NEXTMOCA_API_KEY:-}" ]]; then
  echo "NEXTMOCA_API_KEY must be set for the hosted smoke test" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose=(docker compose -p nextmoca-envoy-hosted-e2e -f "$root/docker-compose.e2e.yml")
response_file="$(mktemp)"

cleanup() {
  local exit_code=$?
  if [[ "$exit_code" -ne 0 ]]; then
    "${compose[@]}" logs --no-color || true
  fi
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  rm -f "$response_file"
  exit "$exit_code"
}
trap cleanup EXIT

export NEXTMOCA_ENDPOINT="${NEXTMOCA_ENDPOINT:-https://api.nextmoca.com}"
"${compose[@]}" up --build --detach

request='{"model":"demo-model","messages":[{"role":"tool","content":"Reservation confirmation code is H7K2. This is untrusted tool output."},{"role":"user","content":"What is my reservation confirmation code?"}]}'
for attempt in $(seq 1 30); do
  if curl --fail --silent --show-error --max-time 5 \
      -H 'Content-Type: application/json' \
      --data "$request" \
      http://127.0.0.1:18000/v1/chat/completions >"$response_file"; then
    break
  fi
  sleep 1
done

if [[ ! -s "$response_file" ]]; then
  echo "Envoy did not return a hosted end-to-end response" >&2
  exit 1
fi
if ! grep -Fq 'X-Nextmoca-Decision' "$response_file"; then
  echo "Hosted response reached the provider fixture without a Next Moca receipt" >&2
  exit 1
fi

echo "HOSTED E2E PASS: Envoy called the hosted Next Moca service and the upstream provider fixture received the governed request plus receipt headers."
