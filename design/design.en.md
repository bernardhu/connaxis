# Core Design and Implementation Details

This document describes the concrete implementation strategies used by `connaxis` to achieve high performance.

### 1. Design Goals and Their Implementations

#### 1.1 High Throughput and Low Tail Latency
- **I/O model**: uses the **Reactor pattern**.
  - **Per-loop listeners (`LoopConn`)**: each worker loop binds its own listener set and accepts connections directly.
  - **Reuseport fan-out**: the runtime relies on `SO_REUSEPORT` with one listener per loop, instead of a single central acceptor redispatching accepted connections.
  - **Connection loops (`LoopConn`)**: handle accept, reads, writes, and per-loop connection state for established connections.
- **Event triggering**: uses **Level Triggered (LT)** mode (the default epoll behavior). This requires the code to fully drain ready events in one pass, or to manage unfinished state correctly.
- **Memory management**:
  - Uses the global **`pool.GAlloctor`**.
  - Uses a **Power-of-2** size-class strategy (Step=1024, Rank=...) combined with `sync.Pool` to reuse memory blocks and significantly reduce GC pressure.
- **Syscall optimizations**:
  - Batch event processing (`EpollWait` returns multiple events)
  - Dynamically resize the event buffer based on load

#### 1.2 Receive-Side Optimization
- **Shared receive buffer**: each worker loop (`LoopConn`) owns a large shared buffer (`recvbuf`).
- **Read strategy**:
  - Prefer reading into the shared buffer to reduce per-connection allocations.
  - **Zero-copy attempt**: if a full packet is available in the shared buffer, pass a slice directly to the upper layer (requires upper-layer lifecycle discipline).
  - **Private buffer**: copy into the connection's private `RingBuffer` only when a partial/sticky packet occurs or the shared buffer is insufficient.

#### 1.3 Send-Side Optimization
- **Vectorized write**: uses the `writev` syscall.
- **Write coalescing**: automatically merges the head/tail segments of the `RingBuffer` and blocks from the `ZeroCopy` queue, then sends them in one syscall to reduce kernel transitions.
- **Write queue**: provides an efficient queue that supports mixed scheduling of normal buffers and ZeroCopy buffers.

#### 1.4 Lightweight Callback and Async Processing
- **Design philosophy**: the `OnData` callback must be non-blocking and very fast.
- **Worker pool mode**: `OnData` should preferably only decode protocol data, while expensive business logic is dispatched to an external worker pool.
- **Async write-back (`AddCmd`)**: after worker processing completes, response data is safely injected back into the I/O loop's send queue through `AddCmd`, enabling thread-safe async writes.

#### 1.5 Fairness and Flow Control
To prevent a single connection or event type from starving others, the system enforces quotas on critical paths (see `eventloop/tuning.go`):

- **Accept limit**: `MaxAcceptPerEvent` (default 128) - max new connections accepted in a single event loop iteration
- **Read limit**: `MaxReadBytesPerEvent` (default 256KB) - max bytes read from a single connection in one iteration
- **Write limit**: `MaxFlushBytesPerEvent` (default 256KB) - max bytes flushed to a single connection in one iteration
- **Command limit**: `MaxCmdPerEvent` (default 1024) - max external commands handled per iteration, preventing I/O starvation under external flooding
- **TLS handshake limit**: caps concurrent handshakes via `TlsHandshakeWorkers` and `TlsHandshakeMaxPending` to avoid CPU overload

#### 1.6 TLS Engine Paths (aTLS / kTLS)
- **Engine selection**: the TLS path supports explicit `atls` and `ktls` modes.
- **Runtime fallback**: even when kTLS is requested, the connection may fall back to the standard async TLS path if system capability, kernel version, cipher support, or negotiated parameters do not qualify.
- **Accept-path protection**: TLS handshakes remain bounded by controls such as `TlsHandshakeWorkers` / `TlsHandshakeMaxPending` to avoid blocking the Accept Loop during handshake spikes.
- **Design boundary**: kTLS is a Linux-specific optimization path and should be treated as an optional acceleration layer, not the single TLS implementation strategy for all environments.
- **Related docs**:
  - kTLS status and roadmap: `design/ktls_status_and_roadmap.en.md`
  - caller invariants and compliance notes: `design/constraints.en.md`

### 2. Architecture Highlights

#### 2.1 End-to-End Zero-Copy Pipeline
From receive to send, the implementation minimizes unnecessary memory copies:

1. **Read path**: data is read directly into the loop `Shared Buffer`. If a complete protocol frame is available, it is passed to `OnData` as a slice. Only when packet sticking/fragmentation occurs and the shared buffer cannot hold a full frame does one copy occur into the `Connection RingBuffer`.
2. **Write path**: `EnqueueWrite(owner, size)` transfers ownership of a memory block (pointer), not a copied payload. Internally, `writev` with `iovec` submits `RingBuffer` head/tail segments and `ZeroQueue` blocks to the kernel in a single syscall.

#### 2.2 Lock Contention Elimination (Lock-Free Philosophy)
- **Per-loop design**: each loop is an independent goroutine managing a set of file descriptors. All readable/writable/timeout operations inside a loop are handled as **serialized single-threaded logic**.
- **Lock-free RingBuffer**: because each connection is strictly owned by one loop, its internal `RingBuffer` requires no mutex, eliminating lock contention on hot read/write paths.
- **Command queue**: the only cross-goroutine interaction (such as worker write-back) goes through a buffered channel (`CmdChan`). Channels have internal locking, but this aggregated command handling greatly reduces critical-section collisions compared with per-connection locks.

#### 2.3 Strong Memory Efficiency
- **Power-of-2 Allocator**: a `sync.Pool` wrapper tailored for network I/O. It manages memory in 1k, 2k, 4k... classes, matching packet-size distributions well and driving reuse close to 100%.
- **Buffer reuse**: when a connection closes, buffers are returned to the pool instead of being destroyed. This borrow-and-return model keeps heap allocation rates low and GC overhead nearly invisible under high-concurrency short-lived connections.

#### 2.4 Scalability and Overload Protection
- **Adaptive buffer**: each connection's private `RingBuffer` can grow under bursts and shrink (`Trim`) when idle to reclaim memory.
- **Backpressure**: built-in multidimensional flow control (accept rate and read/write byte limits). Combined with TLS handshake queue limits, the system degrades gracefully under traffic spikes instead of failing abruptly.
- **Idle check**: efficient idle-connection scanning (time wheel or linked-list based) to promptly close dead connections and free file descriptors.
