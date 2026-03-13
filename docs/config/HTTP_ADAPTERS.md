# HTTP Adapters

This document describes the HTTP adapters and their constraints.

## evhandler HTTP (net/http-compatible, subset)

`evhandler` is designed to interop with `net/http`-style handlers (Gin/mux).
It is **dispatch-only** in the event loop.

Key constraints:

- Only `Content-Length` bodies are supported.
- No chunked requests or streaming responses.
- No `Hijacker`/`Flusher` semantics.
- No `Expect: 100-continue` handling.
- No HTTP/2.
- Default limits:
  - `evhandler.MaxHeaderBytes = 64KB`
  - `evhandler.MaxBodyBytes = 8MB`
  - Set to `0` to disable limits.
- Handler must not block the event loop. Use worker pools.
- Cross-goroutine responses must go through `AddCmd`.

Recommended flow:

1. Event loop parses HTTP and builds `*http.Request`.
2. `DispatchHTTP` pushes work to a worker pool.
3. Worker builds response and writes back via `AddCmd`.

Unsupported inputs:

- `Transfer-Encoding: chunked` is rejected with `501 Not Implemented`.
- `Expect: 100-continue` is rejected with `417 Expectation Failed`.

Minimal async pattern:

```go
type job struct {
	conn connection.EngineConn
	req  *http.Request
}

func (h *handler) DispatchHTTP(c connection.EngineConn, req *http.Request) {
	h.queue <- &job{conn: c, req: req}
}

func (h *handler) worker() {
	for j := range h.queue {
		w := new(evhandler.ResponseWriter)
		w.Init()
		h.http.ServeHTTP(w, j.req)
		_ = j.conn.AddCmd(connection.CMD_DATA, w.Bytes())
	}
}
```

Example configuration:

```go
// Tune limits at init.
evhandler.MaxHeaderBytes = 32 * 1024
evhandler.MaxBodyBytes = 4 * 1024 * 1024
```

## evhandler FastHTTP (fasthttp-compatible)

`evhandler` is optimized for performance and uses fasthttp request/response
types. It is **dispatch-only** in the event loop.

Key constraints:

- Only `Content-Length` bodies are supported.
- No chunked requests or streaming responses.
- No `Expect: 100-continue` handling.
- No HTTP/2.
- Default limits:
  - `evhandler.MaxHeaderBytes = 64KB`
  - `evhandler.MaxBodyBytes = 8MB`
  - Set to `0` to disable limits.
- fasthttp request objects are not goroutine-safe:
  - For async handling, `CopyTo` a new request and `ReleaseRequest` the original.

Recommended flow:

1. Event loop parses HTTP and builds `*fasthttp.Request`.
2. `DispatchFast` copies the request and pushes to a worker pool.
3. Worker builds `fasthttp.Response`, encodes to bytes, and writes via `AddCmd`.

Unsupported inputs:

- `Transfer-Encoding: chunked` is rejected with `501 Not Implemented`.
- `Expect: 100-continue` is rejected with `417 Expectation Failed`.

Minimal async pattern:

```go
type job struct {
	conn connection.EngineConn
	req  *fasthttp.Request
}

func (h *handler) DispatchFast(c connection.EngineConn, req *fasthttp.Request) {
	dup := fasthttp.AcquireRequest()
	req.CopyTo(dup)
	evhandler.ReleaseRequest(req)
	h.queue <- &job{conn: c, req: dup}
}

func (h *handler) worker() {
	for j := range h.queue {
		resp := fasthttp.AcquireResponse()
		// build response
		out, _ := evhandler.EncodeResponse(resp)
		fasthttp.ReleaseResponse(resp)
		evhandler.ReleaseRequest(j.req)
		_ = j.conn.AddCmd(connection.CMD_DATA, out)
	}
}
```

Example configuration:

```go
// Tune limits at init.
evhandler.MaxHeaderBytes = 32 * 1024
evhandler.MaxBodyBytes = 4 * 1024 * 1024
```

## WebSocket

WebSocket upgrade is supported by the internal `websocket` implementation.
For HTTP+WS on the same port, use `evhandler` + `ConnaxisHttpWsHandler`.
