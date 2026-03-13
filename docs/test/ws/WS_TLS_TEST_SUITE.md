# WS/TLS Test Coverage Supplement

Primary system-level validation now lives in:

- `docs/test/linux-lab/README.zh-CN.md`
- `docs/test/linux-lab/WSS_KTLS_MATRIX.zh-CN.md`

This document remains as a repo-level coverage note. It describes the unit and integration tests inside the codebase, which is a different scope from the Linux lab's end-to-end WS/WSS/TLS/kTLS validation.

## How to run

Unit tests (pure parsing/codec, no networking):

```sh
go test ./websocket
```

Integration tests (requires binding a local TCP port):

- File: `websocket/ws_tls_integration_test.go`
- The tests will **auto-skip** when `net.Listen("tcp4", "127.0.0.1:0")` is not permitted (e.g. in some sandboxes).

To force running only TLS/WSS integration tests:

```sh
go test ./websocket -run 'TestConnaxis(TLSEcho|WSS)$'
```

## Coverage (what we verify)

### WebSocket handshake

- `Upgrade: websocket` value is accepted case-insensitively.
- `Connection` header supports a **token list** (e.g. `keep-alive, Upgrade`).
- `Sec-WebSocket-Accept` generation matches RFC6455 examples.

Tests:
- `websocket/handshake_test.go`

### WebSocket frames

- Client masking requirement (`WSRequireClientMask`) and protocol close (1002) on unmasked frames.
- Ping → Pong control frame behavior.
- Fragmentation reassembly and invalid continuation handling.
- Partial frame parsing behavior (`parseFrame` expects more bytes when incomplete).

Tests:
- `websocket/frame_test.go`

### TLS / WSS integration (connaxis)

- TLS echo with self-signed ECDSA cert, for TLS 1.2 and TLS 1.3.
- WSS upgrade + message echo using `github.com/gorilla/websocket` client.

Tests:
- `websocket/ws_tls_integration_test.go`

## Known gaps / TODOs (intentional)

These are **feature gaps** compared with full-featured WebSocket stacks (e.g. gorilla/websocket):

- Subprotocol negotiation (`Sec-WebSocket-Protocol`) is not implemented.
- Extensions negotiation (`Sec-WebSocket-Extensions`) is not implemented (no permessage-deflate).
- No Origin validation / allowlist at handshake layer (needs app-level policy).
- Limited TLS surface area tests (no mTLS, no session resumption, no cipher-suite matrix).

If you want, we can add a “feature matrix” table (Supported / Not supported / Planned) and tie each row to a test case.

### Feature matrix (quick view)

| Area | Feature | Status | Notes |
|---|---|---|---|
| Handshake | `Upgrade: websocket` case-insensitive | ✅ | `websocket/handshake_test.go` |
| Handshake | `Connection` token list contains `Upgrade` | ✅ | `websocket/handshake_test.go` |
| Handshake | `Sec-WebSocket-Accept` (RFC example) | ✅ | `websocket/handshake_test.go` |
| Handshake | Subprotocol negotiation | ❌ | TODO in `websocket/handshake.go` |
| Handshake | Extensions negotiation / compression | ❌ | TODO in `websocket/handshake.go` |
| Frames | Client masking requirement | ✅ | `websocket/frame_test.go` |
| Frames | Ping/Pong | ✅ | `websocket/frame_test.go` |
| Frames | Fragmentation reassembly | ✅ | `websocket/frame_test.go` |
| TLS | TLS echo (TLS1.2/TLS1.3) | ✅ | `websocket/ws_tls_integration_test.go` |
| WSS | WSS upgrade + echo | ✅ | `websocket/ws_tls_integration_test.go` |
