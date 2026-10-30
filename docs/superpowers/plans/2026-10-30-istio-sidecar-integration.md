# Istio Sidecar Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a workload-scoped Istio sidecar deployment path for the thin Next Moca ExtProc bridge.

**Architecture:** The optional Helm `istio` block emits a namespaced, workload-scoped `EnvoyFilter` that inserts the standard Envoy External Processing filter before the outbound router for one explicit HTTP port. A separate direct-provider TLS-origination recipe documents the only supported way to inspect an external provider call without decrypting application-originated TLS.

**Tech Stack:** Go bridge, Helm 3, Kubernetes, Istio `EnvoyFilter`, Envoy ExtProc v3, Kind E2E.

**Spec:** `docs/superpowers/specs/2026-10-30-istio-sidecar-integration-design.md`

## Global Constraints

- The bridge image remains a thin hosted-API client; do not add selection, scan, or enforcement core code.
- Disabled-by-default Istio integration; require workload selector labels when enabled.
- Govern only mesh-visible HTTP on one configured outbound port; never claim direct external TLS inspection.
- Preserve existing raw Envoy and Envoy Gateway behavior and their verification gates.
- Keep the new E2E disposable and run it against a supported Istio Helm release.

## Review Focus

- Empty workload selectors: Helm must reject them when Istio is enabled.
- Unscoped filters: rendered filter must match `SIDECAR_OUTBOUND` plus the configured port.
- HTTP/2 to the bridge: the ext_proc gRPC service must use Istio's existing outbound cluster and the named gRPC port.
- Streaming: documentation must state that request buffering excludes streaming and protocol upgrades.
- Direct HTTPS: documentation must distinguish it from the explicit TLS-origination recipe.

---

### Task 1: Add an optional, workload-scoped Helm EnvoyFilter

**Files:**
- Modify: `charts/nextmoca-envoy-extproc/values.yaml`
- Modify: `charts/nextmoca-envoy-extproc/values.schema.json`
- Create: `charts/nextmoca-envoy-extproc/templates/envoyfilter.yaml`
- Create: `charts/nextmoca-envoy-extproc/tests/istio-values.yaml`
- Modify: `scripts/test-chart.sh`

**Interfaces:**
- Consumes: the chart bridge Service name and gRPC port.
- Produces: an `EnvoyFilter` only when `istio.enabled=true`.

- [ ] Write a render assertion for an Istio-enabled values file that expects the workload selector, `SIDECAR_OUTBOUND`, configured port, `envoy.filters.http.ext_proc`, buffered request mode, timeout, fail-open policy, and the correct `outbound|9002||...` cluster.
- [ ] Run `make chart-test` and verify the assertion fails because the Istio chart template does not exist.
- [ ] Add the typed Istio values and schema guard that requires at least one workload selector label when enabled.
- [ ] Implement the conditional EnvoyFilter template with `INSERT_BEFORE` the HTTP router.
- [ ] Run `make chart-lint chart-test` and verify it passes.
- [ ] Commit the chart integration.

### Task 2: Add deployment recipes and precise traffic-boundary documentation

**Files:**
- Create: `examples/istio/README.md`
- Create: `examples/istio/direct-provider-tls-origination.yaml`
- Modify: `README.md`
- Modify: `scripts/test-examples.sh`

**Interfaces:**
- Consumes: the Helm Istio values contract from Task 1.
- Produces: a model-gateway quick start and a customer-editable ServiceEntry/DestinationRule TLS-origination recipe.

- [ ] Write a validation assertion that the example has a `ServiceEntry` HTTP port mapped to target port `443` and a `DestinationRule` with `tls.mode: SIMPLE`.
- [ ] Run `bash scripts/test-examples.sh` and verify it fails because the recipe is absent.
- [ ] Add the recipe and guide, including the direct-HTTPS unsupported boundary and streaming bypass requirement.
- [ ] Run `bash scripts/test-examples.sh` and verify it passes.
- [ ] Commit the recipes and docs.

### Task 3: Add a disposable Istio sidecar end-to-end verification

**Files:**
- Create: `e2e/istio/resources.yaml`
- Create: `scripts/e2e-istio.sh`
- Modify: `Makefile`
- Modify: `README.md`

**Interfaces:**
- Consumes: `nextmoca/envoy-extproc:local`, `nextmoca/e2e-tools:local`, Helm-installed Istio, and the workload-scoped EnvoyFilter.
- Produces: `make e2e-istio`, which exits successfully only when a sidecar-injected client sends governed provider context and receipt headers through the mesh.

- [ ] Write the failure assertions in the script: the response must retain the safe reservation code, omit the malicious instruction, and include `X-Nextmoca-Tokens-Saved`.
- [ ] Run the script before the manifests exist and verify it fails at the missing resources.
- [ ] Add fixture Services/Deployments, a sidecar-injected client, and the workload-scoped EnvoyFilter. Install Istio via its official Helm base and istiod charts into a disposable Kind cluster.
- [ ] Run `make e2e-istio` and verify the real sidecar request is mutated by the bridge and the cluster is deleted afterward.
- [ ] Run the existing `make e2e` and `make e2e-envoy-gateway` to guard the other deployment paths.
- [ ] Commit the E2E path.

### Task 4: Final verification and release readiness

**Files:**
- Modify: `README.md` only if verification commands or user-facing installation copy needs correction.

- [ ] Run `go test ./...`, `go vet ./...`, `make chart-lint chart-test`, `bash scripts/test-examples.sh`, `bash scripts/test-workflows.sh`, `make e2e`, and `make e2e-istio`.
- [ ] Review `git diff --check` and confirm the runtime image remains limited to the bridge binary.
- [ ] Commit any verification-driven documentation correction.
