# Constraints (Invariants)

To achieve high throughput and controllable tail latency, this project uses "strong invariants" on critical paths to reduce overhead. Violating these invariants can directly cause:

- slice out-of-bounds panics (for example `size > len(owner)`)
- `sync.Pool` corruption (double put / reusing the same memory block twice), resulting in cross-packet contamination or strange crashes
- buffers not being returned to the pool (length mismatch), causing steady memory growth

The following constraints are internal component contracts: callers must follow them; the implementation intentionally avoids excessive defensive checks.

### 1. Write Path: `owner + size` Protocol

#### 1.1 `owner` Must Be a Full Bucket
- `owner` must come from `pool.GAlloctor.Get(n)` and **must keep the original length** returned by the allocator (do not do `owner = owner[:n]`).
- `size` is the payload length and must satisfy `0 < size && size <= len(owner)`.

Reason: `pool.GAlloctor.Put(buf)` uses `len(buf)` to choose the target `sync.Pool` (`pool/pool.go:73`). If `owner` is resliced to `owner[:size]` before enqueueing, a later `Put` cannot find the matching pool, causing "return failure / memory growth" (and worse, wrong-pool returns may corrupt the pool).

#### 1.2 Ownership Transfers After `EnqueueWrite`
- After calling `EngineConn.EnqueueWrite(owner, size)` (`connection/base.go:225`), ownership of `owner` is transferred to the connection write queue.
- Callers must not `Put`, reuse, or write to `owner` anymore, and must not retain a slice reference for later async writes.

The write queue may return `owner` at these times:

- After the data is fully written (`connection/writequeue.go:82`)
- When small-packet coalescing succeeds, the newly enqueued `owner` is **immediately** `Put` (`connection/writequeue.go:43`)
- When the connection closes or the write queue is cleaned up (`connection/writequeue.go:103`)

In other words, once `EnqueueWrite` returns, `owner` may already have been returned to the pool (coalesce path). Any external access or mutation becomes use-after-free behavior.

### 2. `AddCmd`: Caller-Facing Contract
- For business code / handlers, the public cross-goroutine write entry is `EngineConn.AddCmd(cmd, data)` (`connection/base.go:196-198`).
- `AddCmd` copies `data` into an engine-managed buffer before enqueueing (`eventloop/loopConn.go:588-598`), and the engine owns that buffer lifecycle afterward.
- Therefore, callers neither need nor are able to manage the internal pool lifecycle, and should not treat `AddCmd` as a zero-copy or ownership-transfer API.
- After `AddCmd` returns, callers may reuse or modify their original `data` slice (subject to their own synchronization rules); the engine no longer depends on future contents of that slice.
- If `AddCmd` returns an error (for example allocation failure or a full command queue; see `eventloop/loopConn.go:590-594` and `eventloop/loopConn.go:604-612`), the command was not successfully enqueued and the engine cleans up any internal resources allocated for that attempt.

This document intentionally does not expose internal ownership details among `CmdData`, `reset()`, and the write queue; those are implementation-level invariants maintained by the engine and are not part of the external caller contract.

### 3. RingBuffer: Single-Thread / Single-Loop Constraint
- `ringbuffer.RingBuffer` uses `internal.Fakelock` internally (a no-op lock, `internal/spinlock.go:21`), so **RingBuffer is not goroutine-safe**.
- A connection's `Recvbuf()` must only be accessed and modified within its owning event-loop goroutine (for example inside `processRead` and the handler callback stack).
- For cross-goroutine sends, use `AddCmd`. Do not directly read/write connection state such as `Recvbuf/FlushN/EnqueueWrite` from a worker pool.

### 4. Poller Constraint: Level Triggered
- This library uses Linux epoll in **Level Triggered (LT)** mode (default behavior).
- Once a read-ready event fires, if the socket buffer is not drained in one pass (or until EAGAIN), the poller will keep reporting readiness.
- This adds syscall overhead and can cause busy polling CPU usage. Therefore, `processRead` should keep reading via the buffer loop until no data is available.

### 5. TLS Handshake Constraint
- TLS handshakes are CPU-intensive.
- To avoid blocking the Accept Loop with handshake requests, the system limits `TlsHandshakeWorkers`, typically based on CPU core count.
- It also enforces `TlsHandshakeMaxPending`; new connections beyond this limit may be rejected or delayed to protect service quality.

### 6. kTLS Key Export Constraint (`KeyLogWriter`)
- The current kTLS implementation depends on `crypto/tls` `KeyLogWriter` to export session keys.
- Enabling kTLS is effectively equivalent to allowing session key export. Even though the implementation does not write keys to disk, it may still be considered "keys are exportable" from a compliance/audit perspective.
- For strict compliance or a no-key-export requirement, switch to a forked `crypto/tls` or an OpenSSL/BoringSSL-based solution.

### 7. TCP/HTTP/WS Same-Port Multiplexing Constraint (`ConnaxisTcpHttpWsHandler`)
This handler sniffs the first packet's method token to route protocols (TCP vs HTTP, then HTTP upgrade to WS).

- **Hard constraint**: the first packet of a custom TCP protocol must not start with HTTP methods like `GET ` / `POST ` / `PUT `, otherwise it may be misclassified as HTTP.
- **Fragment behavior**: if the first packet is too short but looks like an HTTP method prefix (for example `GE`), routing is deferred until the loop reads more bytes.
- **`OnConnected` timing**: `TcpProtoHandler.OnConnected` is triggered only after the connection is identified as TCP (typically when the first packet arrives), not immediately on accept.

If you need completely unambiguous protocol separation, use separate ports / listeners instead of same-port multiplexing.

### 8. HTTP Adapter Constraints (`ConnaxisHttpHandler`)

- `ConnaxisHttpHandler` is best treated as a **dispatch-only** adapter: keep `OnData` focused on parsing/dispatching and avoid CPU-heavy or blocking business logic there.
- For cross-goroutine response writes, always inject responses back to the owning loop via `AddCmd`; do not mutate connection state directly from workers.
- The current `ConnaxisHttpHandler` supports `Content-Length` request bodies only (no chunked request bodies, no streaming request bodies).
- For long-running or async business logic, copy the required request data early and hand the expensive work off to a worker pool.

### 9. FastHTTP Adapter Constraints (`ConnaxisFastHTTPHandler`)

- `ConnaxisFastHTTPHandler` follows `fasthttp` object-reuse semantics, so **request object lifetimes are very short**.
- `fasthttp` request objects (or references to their internal fields) obtained in `OnData` / dispatchers must not be retained across goroutines.
- For async handling, call `CopyTo` into a new request object (or copy the required fields) before the current request context is released.
- As with `ConnaxisHttpHandler`, use `AddCmd` for cross-goroutine response writes instead of writing the connection directly.
