# Envoy Distribution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the thin Envoy ExtProc bridge installable and observable through a signed OCI release, OCI Helm chart, and production deployment recipes.

**Architecture:** The existing Go bridge remains the only runtime binary and continues to call the hosted Next Moca API. Helm, Kubernetes manifests, raw-Envoy examples, CI release automation, and Prometheus-style bridge metrics live outside the hosted decision algorithm. A tag-triggered workflow builds the same distroless image already exercised in end-to-end tests and publishes it with provenance artifacts.

**Tech Stack:** Go 1.26, Prometheus Go client, Docker Buildx, GitHub Actions, Helm OCI charts, Kubernetes Gateway API, Envoy ExtProc.

**Spec:** `README.md`

## Global Constraints

- Publish only the thin bridge, adapters, operational assets, and fixtures; never vendor Next Moca selection, scanning, or enforcement code.
- Keep the hosted API key runtime-only, sourced from an existing Kubernetes Secret.
- Preserve current request/response behavior and provider adapters.
- Keep CI reproducible without a hosted Next Moca credential.
- Security modes remain route configuration, never embedded customer policy in the Helm chart.

## Review Focus

- Chart installs must fail clearly when no existing API-key Secret is named.
- Generated `EnvoyExtensionPolicy` must use a buffered request body and a configurable route target.
- Metrics must never label by prompt, record ID, API key, or arbitrary request ID.
- OCI workflow must publish only this repository's Docker build context and must be tag-gated.
- Raw Envoy examples must preserve the request content-length correction required by body rewrites.

### Task 1: Add bounded bridge metrics

**Files:**
- Create: `internal/metrics/metrics.go`
- Create: `internal/metrics/metrics_test.go`
- Modify: `internal/extproc/service.go`
- Modify: `cmd/nextmoca-envoy-extproc/main.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Produces `metrics.New() *Metrics`, `ObserveDecision(mode string, decision policy.Decision)`, `ObserveBlock(reason string)`, and `Handler() http.Handler`.
- `extproc.NewServer` accepts the metrics instance and records only bounded mode/decision/outcome/reason labels.

- [ ] Write tests that assert a transform, pass-through, and block increment the expected bounded metric families.
- [ ] Add a registry-backed metrics package and wire it into ExtProc body processing.
- [ ] Serve `/metrics` alongside `/healthz` and document it.
- [ ] Run `go test ./... -count=1` and `go vet ./...`.

### Task 2: Package a production OCI Helm chart

**Files:**
- Create: `charts/nextmoca-envoy-extproc/Chart.yaml`
- Create: `charts/nextmoca-envoy-extproc/values.yaml`
- Create: `charts/nextmoca-envoy-extproc/values.schema.json`
- Create: `charts/nextmoca-envoy-extproc/templates/*.yaml`
- Create: `charts/nextmoca-envoy-extproc/README.md`
- Modify: `Makefile`, `README.md`

**Interfaces:**
- The chart accepts `existingSecret`, image repository/tag/digest, bridge configuration, resources, autoscaling, network policy, service monitor, and optional Envoy Gateway policy settings.
- The chart renders `Deployment`, `Service`, `ServiceAccount`, optional `HPA`, `NetworkPolicy`, `ServiceMonitor`, and `EnvoyExtensionPolicy`.

- [ ] Render the chart without optional resources and assert Secret name, gRPC service, health endpoint, and security context.
- [ ] Render with the Gateway policy and assert `request.body: Buffered`, route target, message timeout, and fail-open choice.
- [ ] Implement chart templates and schema validation without embedding an API key.
- [ ] Run `helm lint` and `helm template` for both values sets.

### Task 3: Add raw Envoy and Gateway deployment recipes

**Files:**
- Create: `examples/envoy/envoy.yaml`
- Create: `examples/envoy/README.md`
- Create: `examples/envoy-gateway/values.yaml`
- Create: `examples/envoy-gateway/README.md`
- Modify: `README.md`

**Interfaces:**
- Raw Envoy example points the standard `envoy.filters.http.ext_proc` filter to the bridge over h2c.
- Gateway example configures the Helm chart to attach to a named HTTPRoute.

- [ ] Verify raw Envoy config parses with the official Envoy image.
- [ ] Verify Gateway values render through Helm.
- [ ] Document route-scoped rollout: shadow, active, quarantine, then enforce.

### Task 4: Automate signed OCI and Helm releases

**Files:**
- Create: `.github/workflows/release.yml`
- Create: `.github/workflows/chart.yml`
- Modify: `.github/workflows/ci.yml`, `README.md`

**Interfaces:**
- A `v*` tag builds and pushes the distroless bridge image to `ghcr.io/nextmoca/envoy-nextmoca-runtime-defense` with SBOM/provenance attestation and keyless Cosign signing.
- A tag also packages and pushes the Helm chart to `oci://ghcr.io/nextmoca/charts`.

- [ ] Add a release workflow with minimum `packages: write`, `attestations: write`, and `id-token: write` permissions.
- [ ] Add CI chart lint/template validation and image build verification.
- [ ] Validate workflow YAML and render release metadata locally where possible.

### Task 5: Verify the full distribution path

**Files:**
- Modify: `README.md`

- [ ] Run Go tests, static checks, Helm lint/template, raw Envoy config validation, and image build.
- [ ] Run local Envoy, hosted-service fixture, and Envoy Gateway E2E suites.
- [ ] Record only verified commands and outcomes in the README release section.
- [ ] Commit the distribution changes on the feature branch for review.
