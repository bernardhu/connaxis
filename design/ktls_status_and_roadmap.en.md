# kTLS Implementation Status & Roadmap

### 1. Vision
In the standard TLS path (when kTLS is not enabled), `connaxis` uses the async TLS path (`atls`) with `crypto/tls` to process TLS data in user space. This introduces multiple memory copies and can block Reactor threads with cryptographic work.

The goal is to introduce **kTLS (Kernel TLS)** so symmetric encryption/decryption is offloaded to the Linux kernel (or even NIC hardware), normalizing the architecture:

- **Unified path**: after the handshake, a TCP socket is upgraded to a kTLS socket. The Reactor layer no longer needs to be aware of encryption and can read/write plaintext directly.
- **Performance gains**: leverage CPU AES instructions or NIC offload; support `sendfile` for true Zero-Copy HTTPS.

### 2. Constraints and Dependencies

#### 2.1 OS and Kernel
- **Kernel version**:
  - **Theoretical minimum**: Linux 4.17+ may expose bidirectional RX/TX kTLS primitives
  - **Project validation baseline**: Linux 5.15+ (matches the currently validated kernels and TLS 1.3 kTLS expectations in this repository)
  - **Warning**: CentOS 7 (Kernel 3.10) is unsupported and must fall back to the async TLS path
- **Kernel module**: the `tls` kernel module must be loaded (`modprobe tls`)

#### 2.2 Go Runtime
- **Stdlib limitation**: Go's `crypto/tls` does not directly expose a general API for exporting session keys/IVs; accessing lower-level handshake/record details usually requires extra mechanisms.
- **Current implementation path**: the current repository primarily uses `KeyLogWriter`, record-layer helpers, and internal `ktls` logic to obtain/derive key material and record sequence information (see Section 4).
- **Hardening options**: to further reduce dependency on runtime behavior/internal mechanisms, a full `crypto/tls` fork or `unsafe`/linkname-based approaches can be considered (with stability/maintenance tradeoffs).
- **TLS 1.3 limitation (raw stdlib)**: Go's stdlib cannot explicitly configure TLS 1.3 cipher suites through `tls.Config.CipherSuites` (it only applies to TLS 1.0-1.2). This project adds TLS 1.3 suite restriction helpers via `internal/tls`, but version compatibility still needs attention.

### 3. Applicable Scenarios

| Scenario | kTLS Benefit | Recommended Strategy |
| :--- | :--- | :--- |
| **API Gateway (JSON/RPC)** | High (CPU offload) | **Write + kTLS**: eliminate user-space encryption work and one user-space buffer copy. |
| **Static File Server** | Extreme (Zero-Copy) | **Sendfile + kTLS**: data bypasses user space entirely for maximum performance. |
| **Old Linux / Non-Linux** | None | **aTLS Mode (Current)**: keep the existing Worker Pool + RingBuffer path as fallback. |

### 4. Current Implementation Status (Repository Snapshot)

The following capabilities already exist in the current repository (source code is authoritative):

- **TLS engine path selection**: supports explicit `atls` / `ktls` selection (see `connection/tlsengine.go`), with fallback to the async TLS path when kTLS requirements are not met.
- **Important distinction**: there is no longer an `auto` engine-selection mode. The caller must explicitly choose `atls` or `ktls`. The remaining fallback is connection-level runtime fallback from requested `ktls` to `atls` when the current connection/kernel/cipher/runtime state does not qualify.
- **System capability detection**: includes Linux kTLS support checks and non-Linux downgrade paths (see `internal/ktls_support_linux.go` and `internal/ktls_support_other.go`).
- **kTLS key derivation and kernel injection**: `internal/ktls` includes TLS 1.2 / TLS 1.3 AES-GCM paths and `setsockopt(SOL_TLS, ...)` injection logic (see `internal/ktls/linux.go`).
- **Connection integration and fallback**: kTLS handshake, pre-read plaintext drain, and fallback to the async TLS path are integrated in the connection layer (see `connection/ktls_linux.go` and `connection/tlsconn.go`).
- **Basic verification tool**: `cmd/ktlscheck` is provided for critical kTLS flow checks.

Notes:

- The current implementation primarily relies on `KeyLogWriter` plus internal `ktls` helpers to obtain/derive key material. The forked `crypto/tls` approach below should be treated as a future hardening/compliance direction, not the only implementation path today.
- kTLS remains a Linux-specific optimization path. Actual behavior and gains depend on kernel version, cipher suites, deployment environment, and workload shape.

### 5. Future Evolution Roadmap (On Top of Current Implementation)

Before reading this section, it is important to separate two kinds of conclusions:

- **Covered test conclusions (currently valid)**: for the environments/kernel versions/protocol versions/ciphers/workload shapes that have already been tested, the current kTLS path can support valid conclusions (including functionality, compatibility, and some performance conclusions).
- **Unfinished roadmap items (continued improvement)**: many unchecked items are about capability expansion, observability, compliance, and broader environment coverage; they do not mean the current kTLS path is unusable.

In other words, the current state is closer to "the core path is implemented and validated across multiple test scenarios" than "kTLS is still only a concept."

#### Phase 1: Detection & Fallback
- [x] Implement runtime fallback after explicit `ktls` selection
  - Check `/sys/module/tls`
  - Try creating a dummy socket and calling `setsockopt(SOL_TCP, TCP_ULP, "tls")`
- [x] Respect explicit `atls` / `ktls` selection when `Server` starts
- [ ] Add finer-grained observability (enablement rate, fallback reasons, cipher distribution)

#### Phase 2: Handshake & Key Extraction (Fork Strategy)
- [ ] **Current-state clarification**: `internal/tls` is currently a lightweight compatibility layer plus TLS 1.3 suite-restriction extensions (not a full fork)
- [ ] **Hardening strategy (optional)**: fork (copy) Go standard library `crypto/tls` into `internal/tls`
  - **Rationale**: avoids `unsafe` reflection fragility and third-party dependencies; improves stability across Go upgrades
- [ ] **Modifications (if the fork strategy is adopted)**: add methods in `internal/tls` to export session secrets after handshake:
  - `MasterSecret` / `SessionKey` (Rx/Tx)
  - `IV` (implicit nonce)
  - `SequenceNumber` (current record sequence)
- [ ] **Integration**: switch handshake paths to the full-fork `internal/tls` implementation (only if the fork strategy is adopted)
- [ ] (Compliance/productization hardening) Add a compliance mode (no key-export path) with runtime controls

#### Phase 3: Kernel Injection
- [x] Implement `setsockopt` wrappers supporting TLS 1.2 (GCM) and TLS 1.3 structs
- [x] Inject TX key -> enable kernel-side send encryption
- [x] Inject RX key -> enable kernel-side receive decryption
- [ ] (Coverage expansion) Expand kernel/distribution compatibility matrices and automated regression coverage

#### Phase 4: Reactor Integration
- [x] Update `ATLSConn` / connection paths for kTLS integration and fallback:
  - In kTLS mode, `Read/Write` should pass through to the underlying `syscall` path directly, bypassing the RingBuffer encryption layer
  - Simplify `FlushN`; no longer assemble ciphertext frames via `writev`
- [ ] (Capability expansion) Complete end-to-end `sendfile + kTLS` path and benchmark validation (static file workloads)
- [ ] (Productization/operability hardening) Improve observability (aTLS vs kTLS latency/error/fallback metrics)

#### Phase 5: Quality and Benchmark Pipeline
- This section has been split into a dedicated document: [Quality and Benchmark Pipeline](./quality_and_benchmark_pipeline.en.md).

### 6. References
- Linux Kernel: `Documentation/networking/tls.txt`
- Go Issue: `golang/go#44506` (kTLS support discussion)
