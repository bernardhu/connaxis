# Comparison with Similar Open-Source Frameworks

This document compares `connaxis` with common high-performance Go networking libraries (`tidwall/evio`, `panjf2000/gnet`, `cloudwego/netpoll`). It focuses on engineering characteristics and positioning, rather than making exaggerated performance claims.

This comparison focuses on the following engineering dimensions:
- TLS engine paths (`atls` / `ktls`) and Linux kTLS runtime fallback behavior
- Whether enabling kTLS-like capabilities requires introducing an external TLS stack (and the associated integration cost)
- TLS handshake concurrency/queue protection (`TlsHandshakeWorkers` / `TlsHandshakeMaxPending`)
- Protocol adapters and routing capabilities (`ConnaxisHttpHandler`, `ConnaxisFastHTTPHandler`, `ConnaxisTcpHttpWsHandler`)
- Explicit engineering constraints and memory ownership contracts (see `design/constraints.en.md`)

## Quick Comparison Matrix (Engineering View)

Note: this is not a performance ranking. It is meant to quickly identify positioning and engineering tradeoffs.

| Dimension | `connaxis` | `tidwall/evio` | `gnet` | `cloudwego/netpoll` |
| :--- | :--- | :--- | :--- | :--- |
| Core positioning | Gateway-oriented foundation (long connections + connection governance) | Lightweight event loop | General high-performance event-driven framework | High-performance I/O component (often used in RPC stacks) |
| Reactor / epoll/kqueue | Yes | Yes | Yes | Yes (with a different abstraction layer) |
| Multi-reactor / multicore parallelism | Yes | Basic support | Yes (mature) | Commonly scalable in parallel usage patterns |
| Outbound connection governance (keepalive/reconnect) | **Built-in `DialerMng`** | Usually built in business code | Usually wrapped in business code | Usually wrapped in business code |
| Protocol adapter layer (HTTP/WS) | **Built-in adapters** | More lightweight | Richer ecosystem/examples | Usually paired with upper-layer frameworks |
| Integration / validation helpers (mainstream protocols & scenarios) | **Built-in `evhandler` adapters + `examples` / `benchmark` helpers** | Usually built in business code | Rich ecosystem examples, but business glue code is still often custom | Often depends on upper-layer frameworks or business wrappers |
| Built-in kTLS integration and fallback path | **Yes (in connection layer + runtime fallback)** | Usually business-side custom integration | Usually business-side custom integration | Usually business-side custom integration |
| External TLS stack dependency when implementing kTLS-like capability | **Usually unnecessary (built-in Go path + Linux kTLS integration)** | Commonly business-side custom/external integration | Commonly business-side custom/external integration | Commonly business-side custom/external integration |
| TLS engine path selection (incl. kTLS) | **`atls` / `ktls`** | Depends on integration approach | Depends on integration approach | Depends on integration approach |
| Overload protection (handshake/queue/flow control) | **Many explicit knobs** | Relatively lightweight | More complete capabilities | Often handled at upper layers |
| Caller invariants documented | **Strong (ownership/concurrency boundaries)** | Relatively limited | Good documentation | Often depends on upper-layer usage rules |
| Best-fit workloads | Gateways / proxies / long-connection ingress | Simple event-driven services | General high-performance services | RPC / short-connection high-concurrency I/O |

Additional notes:
- `Yes` does not mean the capability is implemented in the same way; abstraction layers, invariant models, and defaults differ significantly.
- The TLS/kTLS row emphasizes whether the framework explicitly considers the engineering integration path, not simply whether TLS can be attached somehow.
- This document treats kTLS as one of `connaxis`'s engineering highlights: the key difference is not only whether it can be enabled, but whether it is integrated into the connection layer with fallback behavior and without requiring an external TLS stack.
- Statements about `gnet` / `netpoll` / `tidwall/evio` do not mean kTLS is impossible there; the point is that if the framework does not ship the path, application teams usually carry extra integration and validation cost.
- When publishing concrete performance claims externally, include `design/performance_methodology.en.md` alongside the results as the methodology reference.

### 1) Comparison: `tidwall/evio`

**Similarities**
- Uses the Reactor pattern and directly uses epoll/kqueue, suitable for high-connection-count scenarios.

**Differences**
- This project includes `DialerMng` (outbound connections / keepalive / reconnect), making it better suited for gateway/proxy scenarios with both ingress and egress traffic.
- This project places stronger emphasis on write-queue and memory-pool ownership-transfer invariants (see `design/constraints.en.md`).
- This project provides explicit TLS engine selection (`atls` / `ktls`), with a Linux kTLS path and runtime fallback when requirements are not met.

**When to choose**
- If you only need a lightweight event loop, `tidwall/evio` is easier to get started with.
- If you need unified inbound/outbound connection management, TLS path selection, and flow control, this project is a closer fit for gateway workloads.

### 2) Comparison: `panjf2000/gnet`

**Similarities**
- High-performance event-driven networking, supports multi-reactor and multicore parallelism.

**Differences**
- `gnet` provides a more complete ecosystem and documentation.
- This project emphasizes a `Power-of-2` pool and lifecycle-controlled memory reuse, which is useful for long-lived connections and mismatched object lifetimes.
- This project adds TLS handshake worker/pending limits on the accept path to prioritize event-loop stability.
- In common Go engineering practice, achieving a similar kTLS/kernel-TLS integration on a framework like `gnet` often requires business-side integration of an external TLS stack (commonly OpenSSL via Go bindings / cgo wrappers), which adds cross-boundary call overhead plus memory-management and operational complexity. This project instead builds the kTLS path and fallback behavior into the connection layer.

**When to choose**
- If maturity and ecosystem are your top priorities, `gnet` is the safer option.
- If your workload needs custom memory management, TLS-handshake overload protection, and stronger connection-governance control, this project is better for secondary development.

### 3) Comparison: `cloudwego/netpoll`

**Similarities**
- Focuses on high-concurrency I/O and reducing user-space overhead.

**Differences**
- `netpoll` is more oriented to RPC / short-connection high-throughput scenarios.
- This project focuses more on long-lived connections, connection governance, and controllable memory reuse.
- This project ships HTTP / FastHTTP / TCP+HTTP+WS adapters, making it more of a gateway foundation than a pure I/O component.

**When to choose**
- RPC frameworks and short-connection workloads are generally a better fit for `netpoll`.
- Long-connection gateways/proxies, especially with protocol adapter and connection-governance needs, are generally a better fit for this project.

### 4) Engineering Preconditions to Consider in Comparisons

**TLS and kernel capabilities**
- The project now has explicit `atls|ktls` TLS engine paths, so comparisons should not stop at "supports TLS vs not"; they should account for Linux kTLS, cipher constraints, and fallback behavior.
- kTLS is a Linux-specific optimization path. It can provide high upside, but depends on kernel version, cipher suite support, and runtime environment. It should not be compared directly with user-space TLS under mismatched conditions.
- In frameworks such as `gnet` that do not ship a built-in kTLS path, similar capabilities often require bringing in an external TLS stack (for example OpenSSL via bindings/cgo) and handling the integration in application code. The call-boundary overhead and engineering complexity should be counted in comparisons.

**kTLS as an engineering highlight (`connaxis` perspective)**
- The key highlight is not only that `connaxis` can use kTLS, but that kTLS is integrated as a first-class connection-layer path under the same TLS engine selection model (`atls` / `ktls`).
- When runtime conditions or negotiated parameters do not qualify, the system can fall back to the aTLS path automatically, reducing production rollout risk.
- If a similar capability is implemented in another framework via an external TLS stack (for example OpenSSL + Go bindings/cgo), teams usually also need to handle call-boundary behavior, memory ownership, error semantic mapping, deployment dependencies, and operational upgrades (including security patching). Those costs should be evaluated together with throughput gains.

**Overload protection and operability**
- The project adds handshake-stage concurrency/queue/pending limits, emphasizing controllable degradation during peaks instead of only ideal-path throughput.
- For gateway workloads, these protections are often as important as raw throughput.

**Protocol adapter layer and engineering boundaries**
- `ConnaxisHttpHandler`, `ConnaxisFastHTTPHandler`, and `ConnaxisTcpHttpWsHandler` reduce integration cost for HTTP/WS entry points.
- Companion `examples` and `benchmark` helpers (for example `examples/http`, `examples/fasthttp`, `examples/tcphttpws`, `benchmark/compare`, `benchmark/ws-autobahn`, and `benchmark/tls-suite`) also reduce the engineering cost of integrating with mainstream protocols/libraries, running side-by-side validations, and reproducing experiments.
- They also introduce explicit constraints (for example first-packet sniffing rules for same-port TCP/HTTP/WS multiplexing), which should be viewed as engineering tradeoffs rather than transparent capabilities.

**Explicit engineering invariants**
- The project documents memory ownership, `AddCmd` write-back behavior, and single-threaded RingBuffer access rules (`design/constraints.en.md`), which helps secondary development teams but also imposes stronger caller contracts.

### 5) Overall Positioning

- This project is positioned as a general-purpose gateway foundation for "high-concurrency long connections + connection governance + controllable memory reuse".
- A more complete description is "a gateway-oriented foundation with TLS engine selection (including a Linux kTLS path), protocol adapters/integration-validation helpers, and overload-protection controls".
- When the requirement is more about "mature ecosystem / near-zero learning cost", `gnet` or `netpoll` should be considered first.
- When the requirement emphasizes connection governance, protocol integration, kTLS engineering integration, and controllable runtime constraints, this project has clearer engineering advantages.
