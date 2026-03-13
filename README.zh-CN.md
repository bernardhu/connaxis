## connaxis

![Go](https://github.com/bernardhu/connaxis/workflows/Go/badge.svg)
[![LICENSE](https://img.shields.io/badge/LICENSE-MIT-blue)](/LICENSE)

`connaxis` 是一个面向 Go 的 Linux-first 事件驱动网络框架，集成了 `TLS/kTLS`、`WebSocket/WSS`、长连接场景支持，以及可复现的协议验证工作流。它直接使用 [epoll](https://en.wikipedia.org/wiki/Epoll) / [kqueue](https://en.wikipedia.org/wiki/Kqueue) 系统调用，而不是标准 Go [net](https://golang.org/pkg/net/) server 路径，整体模型接近 [libuv](https://github.com/libuv/libuv) 和 [libevent](https://github.com/libevent/libevent)。

项目面向高连接数场景，例如网关、代理和长期在线服务。它的主要构建和验证平台是 Linux；对于 macOS 和 BSD，目标是保证基础链路可跑通，而不是追求与 Linux 同等级的特性覆盖。

## 定位

`connaxis` 的定位是一个面向网关/代理和长连接场景的 Linux-first Go 网络运行时，内置 `TLS/kTLS` 与 `WS/WSS` 支持。

相比主要优化纯 TCP event-loop 吞吐的项目，`connaxis` 更强调协议集成和验证：

- 面向 TCP/HTTP/WebSocket 的事件驱动核心
- 内置 TLS，并提供 kTLS 集成路径
- 可复现的兼容性验证流程

本项目不定位为 OpenSSL 的替代品，也不宣称在所有 TLS 边界行为上与 OpenSSL 完全等价。

参见：

- `docs/README.md`
- `docs/test/README.md`

## 项目状态

- 持续开发中
- 公开模块路径：`github.com/bernardhu/connaxis`
- CI：GitHub Actions 执行 `go test ./...` 与 `go vet ./...`

## 社区与治理

- 贡献指南：`CONTRIBUTING.md`
- 贡献路线图：`ROADMAP.zh-CN.md`
- 安全策略：`SECURITY.md`
- 行为准则：`CODE_OF_CONDUCT.md`
- 变更记录：`CHANGELOG.md`

## 特性

- 低开销事件循环架构
- 默认启用 `SO_REUSEPORT` 的 per-loop listener 模型
- 面向 TCP 的运行时模型
- 可选 `TLS / kTLS` 集成路径
- `WebSocket / WSS` 支持（通过插件包）

## 快速开始

### 安装

需要 Go `1.24.2`。

```sh
go get github.com/bernardhu/connaxis
```

## 新贡献者从这里开始

如果你准备做第一次贡献：

1. 先读 `CONTRIBUTING.md`，了解基本规则和验证步骤。
2. 再读 `ROADMAP.zh-CN.md`，了解当前任务边界和优先方向。
3. 跑一遍 `examples/README.md` 中的示例，建立基本运行时认知。
4. 在改动前后各运行一次 `go test ./...`。

推荐从这些方向入手：

- 文档和示例修正
- 协议边界行为的 focused tests
- 小范围的验证或 benchmark 脚本改进

如果准备修改较大的 runtime 或 API 行为，建议先读：

- `design/constraints.en.md`
- `docs/API_BOUNDARY.md`
- `design/README.en.md`

### 用法

下面是一个监听 `5000` 端口的 echo server：

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

用户需要提供一个 handler：

- `ParsePacket` 告诉 eventloop 当前包长和期望长度
- `OnData` 处理输入数据并决定是否返回输出

同时需要提供一个 JSON 配置文件，例如：

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

## 示例

可运行示例见 `examples/README.md`。

## 配置

配置项与默认值见 `docs/config/CONFIG.md`。

## 公开包面

当前面向外部的公开包面是：

- `github.com/bernardhu/connaxis`
- `github.com/bernardhu/connaxis/connection`
- `github.com/bernardhu/connaxis/eventloop`
- `github.com/bernardhu/connaxis/websocket`
- `github.com/bernardhu/connaxis/evhandler`

`internal/` 下的包属于实现细节。根包中的 listener/dial/server wiring 也属于实现细节，不是单独支持的框架模块。

更多文档：

- `docs/config/HTTP_ADAPTERS.md`
- `docs/config/METRICS.md`
- `ROADMAP.zh-CN.md`
- `design/constraints.en.md`
- `design/performance_methodology.en.md`

## 变更记录

见 `CHANGELOG.md`。

## 并发 / 线程安全

- Handler 回调运行在 event-loop goroutine 上。
- `connection.EngineConn.Recvbuf()` 以及 `Read`/`Write`/`FlushN` 不是 goroutine-safe，只能在 handler 回调或同 loop 上下文中使用。
- 跨 goroutine 发送数据时，使用 `connection.AppConn.AddCmd(...)`。传入的 `[]byte` 会被复制，调用返回后可以复用原切片。

## 联系方式

- 一般问题和功能建议：GitHub Issues
- 直接联系：`bernard.hu@gmail.com`

## License

`connaxis` 使用 MIT [License](/LICENSE)。
