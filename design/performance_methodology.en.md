# Performance and Validation Methodology (External-Publishing Version)

This document defines how `connaxis` performance tests, protocol validation, and external comparison reports should be run and presented. The goals are to:

- improve reproducibility and explainability
- avoid unfair comparisons or misleading conclusions
- report performance results together with runtime conditions, invariants, and fallback behavior

Scope:

- throughput/latency performance testing (TCP / HTTP / WS / TLS)
- TLS/kTLS functional and compatibility validation
- engineering-oriented external comparisons with similar frameworks

## 1. Publishing Principles

### 1.1 Reproducible
- Record the full test environment (CPU, kernel, Go version, NIC, NUMA, container vs bare metal, etc.).
- Record key server settings (worker count, TLS engine mode, per-loop listener model assumptions, flow-control knobs).
- Record the client load tool and parameters (concurrency, connections, duration, warmup).

### 1.2 Explainable
- Do not publish a single throughput number only; report throughput, latency percentiles, and error rate together at minimum.
- For TLS/kTLS paths, explicitly report whether fallback occurred and why.
- State the test objective clearly: peak throughput vs stable low tail latency.

### 1.3 Fair
- When comparing frameworks, align hardware, kernel, client tooling, protocol, payload, concurrency model, and duration as much as possible.
- Do not present Linux kTLS optimization results vs another framework's user-space TLS path under mismatched conditions as a direct "framework superiority" claim.
- If a capability is unsupported or unconfigured in a comparison target, mark it explicitly instead of silently omitting it.

### 1.4 Transparent
- Disclose known invariants and prerequisites (see `design/constraints.en.md`).
- Disclose which protection mechanisms were enabled/disabled during tests (for example `TlsHandshakeWorkers`, `TlsHandshakeMaxPending`).
- If custom patches, experimental branches, or special kernel settings are used, state them clearly.

## 2. Test Categories and Goals

### 2.1 Performance
- **Throughput tests**: max request/response throughput, byte throughput
- **Latency tests**: P50/P90/P99/P99.9 (including tail latency)
- **Stability tests**: long-run throughput variance, error rate, memory growth trend
- **Degradation tests**: controllable behavior under spikes/overload (not only peak-path throughput)

### 2.2 Validation / Compatibility
- **TLS handshake and version validation** (TLS 1.2 / 1.3)
- **kTLS detection and enablement validation** (kernel module, kernel version, cipher support)
- **WebSocket protocol compatibility** (for example Autobahn)
- **Default TLS baseline validation** (`smoke`, `testssl`, `tlsanvil`)
- **Extended TLS-only checks** (`tlsfuzzer`, `bogo`) when stricter negative/interoperability coverage is needed

### 2.3 Regression
- before/after comparisons on the same host with the same parameters
- targeted retests for specific fixes (for example single-case regression reruns)
- consistency checks between kTLS and aTLS paths, including fallback behavior

## 3. Test-Dimension Matrix (Recommended Minimum Coverage)

### 3.1 Protocol Dimension
- TCP echo
- HTTP (short connection / Keep-Alive)
- WebSocket (plain WS / WSS)
- TLS handshake and pure TLS data path (aTLS vs kTLS)

### 3.2 Traffic Shape Dimension
- small-payload high-QPS (for example 64B / 256B)
- medium payloads (for example 1KB / 4KB)
- large payload / streaming-like transfers (for example 16KB+)
- long-lived bidirectional traffic vs short-lived connection churn

### 3.3 Concurrency Dimension
- connection count (low / medium / high)
- client worker concurrency
- pipeline / batching depth (if applicable)
- TLS handshake concurrency (especially handshake-spike scenarios)

### 3.4 TLS/kTLS Dimension
- TLS engine mode: `atls` / `ktls`
- protocol version: TLS 1.2 / TLS 1.3
- cipher suites (at least record the actual negotiated results)
- kTLS fallback occurrence (count and reason categories)
- Linux kernel version and distribution (strongly relevant for kTLS)

### 3.5 Resource and System Dimension
- CPU model / core count / frequency (including power-saving mode state)
- memory capacity
- NIC model and offload settings (if applicable)
- NUMA topology and CPU pinning (if applicable)
- bare metal / VM / container deployment

## 4. Minimum Reporting Set

### 4.1 Performance Metrics
- throughput: `req/s`, `MB/s` (at least one; preferably both)
- latency percentiles: P50 / P90 / P99 (P99.9 recommended)
- error rate: timeouts, connection errors, protocol errors, unexpected closes
- success rate: request success rate or handshake success rate

### 4.2 Resource Metrics
- server CPU utilization (overall and hot threads when possible)
- memory usage (RSS / heap trend)
- GC indicators (for Go, at least GC count or pause overview)
- fd/connection peaks (for high-connection scenarios)

### 4.3 TLS/kTLS-Specific Metrics
- kTLS enablement rate (kTLS-enabled TLS connections / total TLS connections)
- kTLS fallback statistics (by reason category)
- TLS handshake latency percentiles (if handshake is in scope)
- negotiated TLS version / cipher distribution (at least note the dominant combinations)

## 5. Environment Disclosure Template (Copy/Paste Friendly)

```text
Test Goal: (throughput / latency / compatibility / regression)
Date:
Commit:
Branch:
Server Host: (CPU / cores / RAM)
OS + Kernel:
Go Version:
Deployment: (bare metal / VM / container)
NIC / Offload Settings:
NUMA / CPU Pinning:
TLS Engine Mode: (atls / ktls)
TLS Version / Cipher (expected & observed):
Server Tuning: (MaxAcceptPerEvent / MaxReadBytesPerEvent / ...)
Client Tool + Version:
Client Params: (connections / concurrency / duration / warmup)
Workload Shape: (protocol / payload / keepalive / handshake ratio)
```

## 6. Reusable In-Repo Tools (Recommended)

The following tools/directories can be used as evidence and validation inputs before publishing results:

- `benchmark/compare`
  - cross-framework benchmark harness for TCP/HTTP/WS/TLS/WSS, including report/chart generation
- `benchmark/ws-autobahn`
  - WebSocket protocol compatibility tests and report generation
- `benchmark/tls-suite`
  - default TLS baseline via `smoke / testssl / tlsanvil`, with optional `tlsfuzzer / bogo` extended checks
- `cmd/ktlscheck`
  - quick kTLS critical-path check (ULP attach + crypto_info inject)
- `cmd/bogo_shim`
  - TLS interoperability/protocol-test helper path

Current interpretation rule:

- default release-facing TLS matrix: `smoke / testssl / tlsanvil`
- `tlsfuzzer` and `bogo`: keep as extended TLS-only checks and do not mix them into the default delivery baseline

Notes:
- Historical results under `benchmark/.../results` are useful reference samples, but external reports should prefer fresh results from the current commit and current environment.
- External reports should clearly distinguish "fresh reruns" from "historical regression records."

## 7. Execution Rules for Comparison Tests (Recommended)

### 7.1 Single-Run Rules
- Warm up before sampling.
- Use fixed test durations (avoid cases where early exits look artificially faster).
- Avoid running other heavy workloads on the same machine during the test.
- If obvious anomalies occur (error-rate spikes, host instability), invalidate the run and record the reason.

### 7.2 Repetition and Statistics Rules
- Run each parameter set at least 3 times (5 recommended).
- Report mean/median and a variability range (min-max or standard deviation).
- Do not publish only the single best run.

### 7.3 Framework Comparison Rules
- Use equivalent protocol semantics whenever possible (same keepalive behavior, payload, handler complexity, etc.).
- If configurations cannot be perfectly aligned, document the differences explicitly and explain the likely direction of impact.
- For kTLS results, always include an aTLS control group to avoid conflating kernel-path acceleration gains with framework baseline overhead differences.

## 8. Recommended External Report Structure (Template)

### 8.1 Executive Summary
- test goal
- summary conclusions (no more than 5 points)
- primary constraints (kernel, environment, protocol)

### 8.2 Setup
- environment disclosure template contents
- server and client command lines (sanitized if necessary)

### 8.3 Results
- tables: throughput / latency / error rate / resource usage
- charts: throughput over time, tail-latency curves (optional)
- kTLS section: enablement rate, fallback statistics, handshake behavior (if applicable)

### 8.4 Interpretation
- why the results look this way (explain using architecture, invariants, and fallback behavior)
- applicability boundaries (where the results generalize and where they do not)

### 8.5 Appendix
- raw logs/result paths
- protocol compatibility reports (Autobahn / TLS suite, etc.)
- relevant commits and config snippets

## 9. Pre-Publish Checklist

- [ ] Chinese/English conclusions are aligned (for bilingual releases)
- [ ] Test objective is explicit (peak throughput / stable latency / compatibility / regression)
- [ ] Environment and configuration disclosure is complete
- [ ] Error rate and success rate are reported, not just throughput
- [ ] TLS/kTLS path and fallback information is reported (if TLS is involved)
- [ ] Invariants and applicability boundaries are stated (reference `design/constraints.en.md`)
- [ ] Configuration differences in comparison targets are documented
- [ ] Raw result paths are traceable (at least internally)

## 10. Relationship to Other Design Docs

- Architecture explanation: `design/architecture_diagram.en.md`
- Implementation details: `design/design.en.md`
- Caller and runtime invariants: `design/constraints.en.md`
- Framework positioning comparison: `design/comparison.en.md`
- kTLS status and roadmap: `design/ktls_status_and_roadmap.en.md`
