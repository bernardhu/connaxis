## connaxis

![Go](https://github.com/bernardhu/connaxis/workflows/Go/badge.svg)
[![LICENSE](https://img.shields.io/badge/LICENSE-MIT-blue)](/LICENSE)

`connaxis` is a Linux-first event-driven networking framework for Go with integrated `TLS/kTLS`, `WebSocket/WSS`, long-lived connection support, and reproducible protocol validation workflows. It uses direct [epoll](https://en.wikipedia.org/wiki/Epoll) / [kqueue](https://en.wikipedia.org/wiki/Kqueue) syscalls instead of the standard Go [net](https://golang.org/pkg/net/) server path, and follows a model similar to [libuv](https://github.com/libuv/libuv) and [libevent](https://github.com/libevent/libevent).

The project targets high-connection-count workloads such as gateways, proxies, and long-lived connection services. It is primarily built and validated for Linux; on macOS and BSD, the goal is basic end-to-end path validation rather than feature parity with Linux.

## Positioning

`connaxis` is positioned as a Linux-first Go networking runtime for gateway/proxy and long-lived connection scenarios, with integrated `TLS/kTLS` and `WS/WSS` support.

Compared with projects that mainly optimize plain TCP event-loop throughput, `connaxis` focuses on protocol integration and validation:

- event-driven core for TCP/HTTP/WebSocket workloads
- built-in TLS support with kTLS integration path
- reproducible compatibility validation workflows

This project is not positioned as a replacement for OpenSSL, and it does not claim full parity with OpenSSL on all TLS edge-case behaviors.

See:

- `docs/README.md`
- `docs/test/README.md`

## Project Status

- Active development
- Public module path: `github.com/bernardhu/connaxis`
- CI: `go test ./...` and `go vet ./...` via GitHub Actions

## Community / Governance

- Contributing guide: `CONTRIBUTING.md`
- Contributor roadmap: `ROADMAP.md`
- Security policy: `SECURITY.md`
- Code of Conduct: `CODE_OF_CONDUCT.md`
- Changelog: `CHANGELOG.md`

## Features

- event-loop based architecture with low overhead
- per-loop listener model with `SO_REUSEPORT` enabled by default
- TCP-focused runtime model
- optional TLS / kTLS integration path
- WebSocket / WSS support (via plugin package)

## Getting Started

### Installing

Requires Go 1.24.2.

```sh
go get github.com/bernardhu/connaxis
```

## New Contributors Start Here

If you want to make a first contribution:

1. Read `CONTRIBUTING.md` for the contribution rules and validation steps.
2. Read `ROADMAP.md` for the current task buckets and change boundaries.
3. Run one of the examples in `examples/README.md` to get a local mental model.
4. Run `go test ./...` before and after your change.

Recommended first-change areas:

- docs and example fixes
- focused tests for protocol edge cases
- narrow validation or benchmark script improvements

Before making a large runtime or API change, read:

- `design/constraints.en.md`
- `docs/API_BOUNDARY.md`
- `design/README.en.md`

### Usage
Example echo server that binds to port 5000:

```go
package main

import (
	"flag"
	"log"

	"github.com/bernardhu/connaxis"
	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
)

var path string

type handler struct {
}

func (h *handler) OnReady(s eventloop.IServer) {
	log.Printf("ready: listen on %v (loops: %d)", s.GetListenAddrs(), s.GetWorkerNum())
}

func (h *handler) OnClosed(c connection.AppConn, err error) {}

func (h *handler) OnConnected(c connection.ProtoConn) {
	c.SetPktHandler(h)
}

func (h *handler) ParsePacket(c connection.ProtoConn, in *[]byte) (int, int) {
	return len(*in), len(*in)
}

func (h *handler) OnData(c connection.ProtoConn, in *[]byte) ([]byte, bool) {
	out := *in
	return out, false
}

func (h *handler) Stat(bool) {}

func main() {
	flag.StringVar(&path, "p", "connaxis.conf", "config file path")

	flag.Parse()
	var h handler
	if err, _ := connaxis.Serve(&h, path); err != nil {
		log.Fatal(err)
	}
}

```

User should provider a handler, which ParsePacket tell the eventloop the packet length and expected length, and OnData which handle the data.

Also a config file should be provided in JSON format, like:
```json
{
	"ncpu": -1,
	"lbStrategy": "rr",
	"sslPem": "",
	"sslKey": "",
	"sslMode": "",
	"bufSize": 1048576,
	"chanSize": 8192,
	"listenAddrs": ["tcp://:5000?reuseport=false"]
}

```

## Examples

See `examples/README.md` for runnable echo/TLS/HTTP/FastHTTP/WebSocket examples.

## Configuration

See `docs/config/CONFIG.md` for config fields and defaults.

## Public Package Surface

The intended public package surface is:

- `github.com/bernardhu/connaxis`
- `github.com/bernardhu/connaxis/connection`
- `github.com/bernardhu/connaxis/eventloop`
- `github.com/bernardhu/connaxis/websocket`
- `github.com/bernardhu/connaxis/evhandler`

Packages under `internal/` are implementation details. Listener/dial/server wiring in the root package is also implementation detail, not a separately supported framework module.

Further docs:

- `docs/config/HTTP_ADAPTERS.md`
- `docs/config/METRICS.md`
- `ROADMAP.md`
- `design/constraints.en.md`
- `design/performance_methodology.en.md`

## Changelog

See `CHANGELOG.md`.

## Concurrency / Thread-safety

- Handler callbacks run on the event-loop goroutine.
- `connection.EngineConn.Recvbuf()` and `Read`/`Write`/`FlushN` are not goroutine-safe; only use them from handler callbacks (or other code running on the owning loop).
- To send from other goroutines, use `connection.AppConn.AddCmd(...)`. The `[]byte` payload is copied; you may reuse it after the call returns.


## Contact

- General questions and feature requests: GitHub Issues
- Direct contact: `bernard.hu@gmail.com`

## License

`connaxis` source code is available under the MIT [License](/LICENSE).
