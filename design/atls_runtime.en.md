# aTLS Runtime Design

This document explains the userspace async TLS (`atls`) path in `connaxis`: why it exists, how it is wired into the event loop, and which tradeoffs are intentional.

## 1. Scope

This document only covers the userspace TLS path:

- `ATLSConn`
- `tlsBufferConn`
- `tls.Conn`
- the read/write bridge between non-blocking file descriptors and `crypto/tls`

It does not describe the Linux kTLS fast path in detail. See:

- [`design/ktls_status_and_roadmap.en.md`](./ktls_status_and_roadmap.en.md)

## 2. Why This Path Exists

`crypto/tls.Conn` is built around a `net.Conn` abstraction and expects stream I/O semantics.

`connaxis` is built around:

- non-blocking file descriptors
- event-driven read/write notifications
- explicit backpressure and bounded buffering

Those two models do not match directly. The `atls` path exists to bridge them without rewriting the standard TLS stack.

Important distinction:

- the project does not support `auto` TLS engine selection anymore
- callers explicitly choose `atls` or `ktls`
- when `ktls` is explicitly requested, the runtime may still fall back per connection to `atls` if the current connection cannot stay on the kTLS path

In other words:

- `crypto/tls` remains the TLS state machine
- `connaxis` remains the event-loop runtime
- `tlsBufferConn` is the glue between them

## 3. Main Objects

### 3.1 `ATLSConn`

`ATLSConn` is the runtime owner for a userspace TLS connection.

It owns:

- the event-loop-facing connection state (`fd`, write queue, callbacks, metrics)
- the userspace TLS state machine (`Conn *tls.Conn`)
- the bridge object (`bio *tlsBufferConn`)
- handshake timing/state
- optional pre-read plaintext used by kTLS fallback/transition paths

This ownership is deliberate: `ATLSConn` is the connection object, while `tlsBufferConn` is only an adapter.

### 3.2 `tlsBufferConn`

`tlsBufferConn` is a fake `net.Conn` used only to drive `tls.Conn`.

It has two modes:

- handshake mode: `direct net.Conn` is set, and `tls.Conn` reads/writes directly to that blocking connection
- event-loop mode: `direct == nil`, and `tls.Conn` reads encrypted bytes from `cin` and writes encrypted bytes into `cout`

It does not implement TLS logic. It only adapts I/O shape.

### 3.3 `tls.Conn`

`tls.Conn` remains the actual TLS engine:

- handshake
- record framing
- encryption
- decryption
- connection state / ALPN / SNI / exporters

## 4. Read Path

The file descriptor contains TLS ciphertext. Callers want plaintext.

The userspace `atls` read path is:

```text
fd read event
-> ATLSConn.Read()
   -> tls.Conn.Read()
      -> tlsBufferConn.Read()
         -> cin.Read()
```

That direct path only succeeds when `cin` already contains encrypted TLS records.

If `cin` is empty, the runtime does this:

```text
ATLSConn.Read()
-> tls.Conn.Read()
   -> tlsBufferConn.Read()
      -> would-block
-> ATLSConn.readCiphertext()
   -> unix.Read(fd, ...)
   -> append ciphertext into cin
-> tls.Conn.Read() again
```

So the important point is:

- `tls.Conn` consumes encrypted records from `cin`
- `ATLSConn.readCiphertext()` refills `cin` from the underlying fd

## 5. Write Path

Callers write plaintext. The file descriptor must eventually send TLS ciphertext.

The userspace `atls` write path is:

```text
ATLSConn.Write(plaintext)
-> tls.Conn.Write(plaintext)
   -> tlsBufferConn.Write(ciphertext)
      -> cout.Write(ciphertext)
-> ATLSConn.FlushN()
   -> writev()/write() to fd
```

The key point is:

- encryption happens inside `tls.Conn.Write`
- `tlsBufferConn.Write` only receives already-encrypted TLS records
- `cout` is a staging buffer, not the TLS engine

## 6. Why It Looks Indirect

This design is intentionally a bridge, not a native event-loop TLS stack.

That means:

- the kernel readiness model drives when reads/writes happen
- `tls.Conn` drives when TLS records are consumable
- `tlsBufferConn` sits in the middle and makes those two models compatible

This is more indirect than a custom record-layer driver, but much cheaper to maintain than owning a forked userspace TLS implementation.

## 7. Copy and Buffering Tradeoffs

The `atls` path is not a zero-copy TLS design.

Typical read-side flow includes:

- `fd -> cin`
- `cin -> tls.Conn`
- `tls.Conn -> caller buffer`

Typical write-side flow includes:

- `caller buffer -> tls.Conn`
- `tls.Conn -> cout`
- `cout -> fd`

This is accepted by design. The goal of `atls` is compatibility and operational simplicity, not minimum-copy record handling.

## 8. Slow Reader / Slow Writer Behavior

### 8.1 Slow Reader

If application plaintext consumption is slow:

- `tls.Conn` cannot drain decrypted output quickly
- `ATLSConn` may still see fd readability events
- `readCiphertext()` keeps feeding encrypted data into `cin` until buffer pressure appears

This path is intentionally bounded. It does not provide unbounded hidden buffering.

If bridge capacity is exhausted, the code fails fast instead of silently accumulating pressure.

### 8.2 Slow Writer

If the socket cannot flush encrypted output quickly:

- `tls.Conn.Write()` produces ciphertext
- `tlsBufferConn.Write()` stages it in `cout`
- `FlushN()` drains `cout` to the fd over time

`cout` is intentionally a bridge buffer, not a large backlog reservoir.

If capacity is exhausted, the path fails fast instead of converting pressure into opaque queue growth.

## 9. Why This Is Still Worth Keeping

Even with the extra bridge layer, `atls` remains valuable because it provides:

- a portable TLS path across Linux / non-Linux
- a maintenance-friendly userspace TLS implementation based on Go stdlib
- a fallback path when kTLS is unavailable or inapplicable
- consistent runtime ownership under the event loop

In short:

- `ktls` is the acceleration path
- `atls` is the portable userspace baseline

## 10. Why It Is Not Replaced by a Custom TLS Record Driver

That alternative is possible, but much more expensive.

To remove most of this bridge logic, the project would need to own a deeper TLS implementation boundary, for example by forking and restructuring `crypto/tls` around explicit record buffers rather than a `net.Conn`.

That would reduce indirection, but would also mean:

- higher maintenance cost
- tighter coupling to Go TLS internals
- larger verification burden for TLS 1.2 / 1.3 behavior

For the current project goals, the bridge model is the simpler and more defensible choice.
