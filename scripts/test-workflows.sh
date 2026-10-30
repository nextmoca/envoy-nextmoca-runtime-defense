#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

test -f "$root/.github/workflows/release.yml"
test -f "$root/.github/workflows/chart.yml"

docker run --rm \
  -v "$root:/repo:ro" \
  -w /repo \
  rhysd/actionlint:1.7.10 \
  -color .github/workflows/ci.yml .github/workflows/chart.yml .github/workflows/release.yml

echo "GitHub Actions workflows validated."
