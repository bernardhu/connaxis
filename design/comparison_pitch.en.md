# Framework Comparison (TL;DR)

## One-Sentence Positioning (Suggested)

`connaxis` is a high-concurrency connection foundation for gateway/proxy workloads, with emphasis on connection governance, controllable memory reuse, protocol integration/adapters, plus a built-in Linux kTLS path and fallback strategy.

## Key Highlights

### 1. kTLS is an engineering highlight, not just an "optional feature"
- `connaxis` provides built-in `atls` / `ktls` TLS engine paths in the connection layer.
- kTLS is not an external sidecar path; it runs under the same connection model as the standard async TLS path.
- When conditions do not qualify (kernel, cipher, negotiated parameters, runtime environment), it can automatically fall back to aTLS, reducing production rollout risk.

### 2. Lower engineering integration cost than “assemble it in business code”
- In the Go ecosystem, if other frameworks need similar kTLS/kernel-TLS integration, a common path is business-side OpenSSL integration (Go bindings / cgo, etc.) with custom plumbing.
- Such solutions often introduce extra call-boundary overhead, memory-management complexity, error semantic mapping, deployment dependencies, and operational upgrade cost.
- `connaxis`'s advantage is moving this integration path into the framework connection layer, reducing repeated infrastructure work in application code.

### 3. Lower protocol integration and validation cost (engineering delivery advantage)
- Built-in `evhandler` adapters: `ConnaxisHttpHandler`, `ConnaxisFastHTTPHandler`, `ConnaxisTcpHttpWsHandler`
- Companion `examples` and `benchmark` helpers: easier integration with mainstream protocols plus side-by-side comparisons and regression validation
- For gateway teams, these helpers often directly reduce time-to-first-version and validation effort

### 4. More controllable behavior under peaks (not just peak throughput)
- TLS handshake worker pool and pending caps (`TlsHandshakeWorkers` / `TlsHandshakeMaxPending`)
- Explicit flow-control and fairness knobs (Accept / Read / Write / Cmd budgets)
- Better fit for ingress services that run long-term under bursty traffic

### 5. More friendly to secondary development teams
- Key invariants (memory ownership, `AddCmd` write-back, RingBuffer concurrency boundaries) are documented explicitly
- Benefit: more predictable behavior; tradeoff: callers need to follow stricter contracts

## Comparison with Common Frameworks

### `tidwall/evio`
- Lighter and faster to get started; suitable for simple event-loop use cases.
- `connaxis` is stronger in connection governance, protocol adapters, and kTLS engineering integration, and is more aligned with a gateway foundation role.

### `gnet`
- `gnet` has mature ecosystem and documentation, and is very strong for general-purpose scenarios.
- `connaxis` differentiates on connection governance, built-in kTLS path + fallback strategy, and gateway-facing adapter/helper engineering capabilities.

### `cloudwego/netpoll`
- `netpoll` is strong in RPC / short-connection high-concurrency I/O scenarios.
- `connaxis` is more oriented to long-lived ingress connections, protocol integration, and unified connection governance.

## Conclusion

> `connaxis`'s core differentiation is not only high-performance event-driven I/O itself, but turning the engineering capabilities that gateway scenarios actually need into built-in framework paths: connection governance, protocol adapters, controllable flow protection, and Linux kTLS connection-layer integration with automatic fallback.  
> This allows teams to pursue performance without repeatedly spending effort on external TLS stack integration, protocol glue code, and runtime protection logic.

## Usage Boundaries

- kTLS is a Linux-specific optimization path; results depend on kernel version, ciphers, and deployment environment.
- TLS/kTLS test results under different environments should not be used directly as pure “framework superiority” conclusions.
- If publishing concrete external numbers, include methodology and environment disclosure: `design/performance_methodology.en.md`.
