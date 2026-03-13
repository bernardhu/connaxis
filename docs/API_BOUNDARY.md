# Public API and Package Boundary

This document defines the intended public package surface of `connaxis` and clarifies which directories are implementation detail.

## Public Packages

These packages are the intended entry points for external users:

- `github.com/bernardhu/connaxis`
  - server bootstrap
  - config loading
  - top-level runtime entry
- `github.com/bernardhu/connaxis/connection`
  - connection-facing types used by handlers
- `github.com/bernardhu/connaxis/eventloop`
  - handler interfaces and loop-facing contracts
- `github.com/bernardhu/connaxis/websocket`
  - WebSocket support
- `github.com/bernardhu/connaxis/evhandler`
  - HTTP/WS adapter helpers

## Not Public API

These are not treated as stable public API:

- `internal/`
- event-loop internals under `eventloop/` that are not part of exported handler contracts
- root-package wiring details such as listener/dial/server bootstrapping internals
- benchmark helpers and test runners under `benchmark/` and `scripts/`

## Stability Intent

- exported names in the packages listed under `Public Packages` should be treated as the supported surface
- implementation detail may change to simplify the runtime model, improve performance, or reduce state duplication
- tests and docs should point users to package-level behavior, not to internal wiring files

## Design Rule

When deciding whether a new type or helper should become public:

- keep it public only if an external caller must own it directly
- keep it internal if it only coordinates runtime wiring between server, listener, dialer, and loops
- prefer fewer public packages with clearer ownership over splitting implementation detail into many small packages
