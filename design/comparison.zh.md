# 同类开源框架对比分析

本文档对比 `connaxis` 与常见 Go 高性能网络库（`tidwall/evio`、`panjf2000/gnet`、`cloudwego/netpoll`）的定位与差异，避免夸大性能结论，侧重工程特性。

本对比重点关注以下工程维度：
- TLS 引擎路径（`atls` / `ktls`）与 Linux/kTLS 运行时回退行为
- 实现 kTLS 类能力时是否需要额外引入外部 TLS 栈（以及由此带来的集成成本）
- TLS 握手并发与排队保护（`TlsHandshakeWorkers` / `TlsHandshakeMaxPending`）
- 协议适配器与路由能力（`ConnaxisHttpHandler`、`ConnaxisFastHTTPHandler`、`ConnaxisTcpHttpWsHandler`）
- 显式工程约束与内存所有权契约（见 `design/constraints.zh.md`）

## 快速对比矩阵（工程视角）

说明：下表不是“性能排名”，而是帮助快速识别定位与工程取舍。

| 维度 | `connaxis` | `tidwall/evio` | `gnet` | `cloudwego/netpoll` |
| :--- | :--- | :--- | :--- | :--- |
| 核心定位 | 网关型底座（长连接 + 连接治理） | 轻量事件循环 | 高性能通用事件驱动框架 | 高性能 I/O 组件（常用于 RPC） |
| Reactor / epoll/kqueue | Yes | Yes | Yes | Yes（抽象层不同） |
| 多 Reactor / 多核并行 | Yes | 基础支持 | Yes（成熟） | 常见用法可并行扩展 |
| 外向连接治理（保活/重连） | **内置 `DialerMng`** | 通常业务侧自建 | 需业务封装 | 需业务封装 |
| 协议适配层（HTTP/WS） | **内置适配器** | 较轻量 | 生态/示例较多 | 常与上层框架配合 |
| 对接 / 验证 helper（主流协议与场景） | **内置 `evhandler` 适配器 + `examples` / `benchmark` helper** | 通常按业务自行封装 | 生态示例较多，但业务 glue code 仍常需自建 | 常依赖上层框架或业务封装 |
| 内建 kTLS 集成与回退路径 | **有（连接层内建 + 运行时回退）** | 通常需业务侧自建 | 通常需业务侧自建 | 通常需业务侧自建 |
| 若实现 kTLS 类能力时的外部 TLS 栈依赖 | **通常不需要（内建 Go 路径 + Linux kTLS 集成）** | 常见做法为业务侧自建/外接 | 常见做法为业务侧自建/外接 | 常见做法为业务侧自建/外接 |
| TLS 引擎路径选择（含 kTLS） | **`atls` / `ktls`** | 视业务集成方式 | 视业务集成方式 | 视业务集成方式 |
| 过载保护（握手/队列/流控） | **显式参数较多** | 相对轻量 | 能力较完整 | 通常在上层治理 |
| 调用约束文档化 | **强（所有权/并发边界）** | 相对少 | 文档较全 | 依赖上层使用规范 |
| 适合场景 | 网关 / 代理 / 长连接入口 | 简单事件驱动服务 | 通用高性能服务 | RPC/短连接高并发 I/O |

补充：
- `Yes` 不代表能力形态完全一致；实现抽象层、约束模型和默认取舍差异很大。
- TLS/kTLS 一栏强调的是“工程集成路径是否被框架显式考虑”，不是单纯“是否能接 TLS”。
- 本文将 kTLS 视为 `connaxis` 的工程亮点之一：关键差异不只是“能不能启用”，还包括是否内建到连接层、是否带回退策略，以及是否需要额外引入外部 TLS 栈。
- 对 `gnet` / `netpoll` / `tidwall/evio` 的表述不代表“不能做 kTLS”，而是强调若框架未内建该路径，业务侧通常需要承担额外集成与验证成本。
- 若对外发布具体性能结论，建议同时附 `design/performance_methodology.zh.md` 作为方法学说明。

### 1) 对比：`tidwall/evio`

**相似点**
- Reactor 模式，直接使用 epoll/kqueue，适用于高连接数场景。

**差异点**
- 本项目内置 `DialerMng`（外向连接 / 保活 / 重连），更适合“入口 + 出口”型网关 / 代理场景。
- 本项目更强调写队列与内存池的“所有权转移”约束（`design/constraints.zh.md`）。
- 本项目提供显式 TLS 引擎选择（`atls` / `ktls`），在 Linux 上可走 kTLS，条件不满足时运行时回退。

**适用场景**
- 若只需轻量事件循环，`tidwall/evio` 上手更快；
- 若需要统一管理内外连接、TLS 路径选择与流控，本项目更贴近网关型需求。

### 2) 对比：`panjf2000/gnet`

**相似点**
- 高性能事件驱动、支持多 Reactor、多核并行。

**差异点**
- `gnet` 提供更完整的可用性生态与文档；
- 本项目强调 `Power-of-2` pool 与“可控生命周期”内存复用，适合长连接与生命周期不一致的场景。
- 本项目在 Accept 路径加入 TLS 握手工作池与 pending 上限，优先保护事件循环稳定性。
- 在 Go 生态的常见工程实践中，若希望在 `gnet` 这类框架上做出类似的 kTLS/内核 TLS 集成，通常需要业务侧额外接入外部 TLS 栈（常见路径是 OpenSSL 的 Go binding / cgo 封装等），这会带来跨边界调用、内存管理与运维复杂度；本项目则将 kTLS 路径内建在连接层并提供回退策略。

**适用场景**
- 若优先考虑成熟度与生态，`gnet` 更稳；
- 若业务需要定制内存管理、TLS 握手过载保护与更强的连接治理控制，本项目更适合二次开发。

### 3) 对比：`cloudwego/netpoll`

**相似点**
- 关注高并发 I/O，强调减少用户态开销。

**差异点**
- `netpoll` 偏 RPC / 短连接高吞吐场景；
- 本项目更关注长连接、连接治理与内存复用的可控性。
- 本项目自带 HTTP / FastHTTP / TCP+HTTP+WS 适配器，更偏“网关底座”而非纯 I/O 组件。

**适用场景**
- RPC 框架 / 短连接场景更适配 `netpoll`；
- 长连接网关 / 代理、需要协议接入层与连接治理的场景更适配本项目。

### 4) 对比时需要关注的工程前提

**TLS 与内核能力**
- 本项目已形成显式 `atls|ktls` TLS 引擎路径，比较时不能只看“是否支持 TLS”，还要看 Linux/kTLS、cipher 限制与失败回退策略。
- kTLS 属于 Linux 特定优化路径，收益高但依赖内核版本、密码套件与运行环境，不应在条件不一致时直接横向比较吞吐。
- 在 `gnet` 等未内建 kTLS 路径的框架中，若业务需要类似能力，往往需要额外引入 OpenSSL 等外部 TLS 栈（通过 binding/cgo 等方式）并自行处理集成细节，这里的调用边界开销与工程复杂度应计入比较结论。

**kTLS 作为工程亮点（`connaxis` 视角）**
- 本项目的亮点不只是“支持 kTLS”，而是把 kTLS 路径作为连接层的一等路径进行集成，并与 `atls` 路径放在同一 TLS 引擎选择模型（`atls` / `ktls`）下。
- 当运行环境或协商结果不满足条件时，系统会自动回退到 aTLS 路径，降低生产环境启用新路径的风险。
- 若采用外部 TLS 栈（例如 OpenSSL + Go binding/cgo）来在其他框架上实现类似能力，通常还需要额外处理调用边界、内存所有权、错误语义映射、部署依赖与运维升级（含安全补丁）等问题；这些工程成本应与纯吞吐收益一起评估。

**过载保护与可运维性**
- 本项目在握手阶段引入并发 / 队列 / pending 上限，更强调“高峰期可控退化”而非仅追求理想路径峰值吞吐。
- 对网关场景而言，这类保护机制通常与纯吞吐同等重要。

**协议适配层与工程边界**
- `ConnaxisHttpHandler`、`ConnaxisFastHTTPHandler`、`ConnaxisTcpHttpWsHandler` 降低 HTTP/WS 接入成本。
- 配套的 `examples` 与 `benchmark` helper（例如 `examples/http`、`examples/fasthttp`、`examples/tcphttpws`、`benchmark/compare`、`benchmark/ws-autobahn`、`benchmark/tls-suite`）降低了与主流协议/库对接、做对比验证与复现实验的工程成本。
- 同时也带来明确边界条件（例如同端口 TCP/HTTP/WS 首包 sniff 规则），应作为工程 tradeoff 而不是“透明能力”理解。

**工程约束显式化**
- 本项目把内存所有权、`AddCmd` 回写、RingBuffer 单线程访问等约束文档化（`design/constraints.zh.md`），对二次开发团队更友好，但也要求调用方遵守更强契约。

### 5) 综合定位

- 本项目定位为“高并发长连接 + 连接治理 + 可控内存复用”的通用网关型底座。
- 可进一步描述为“带 TLS 引擎选择（含 Linux kTLS 路径）、协议适配层 / 对接验证 helper 与过载保护策略的网关型底座”。
- 当需求偏向“成熟生态 / 零学习成本”，可优先考虑 `gnet` 或 `netpoll`。
- 当需求偏向“连接治理 + 协议接入 + kTLS 工程集成 + 运行时约束可控”，本项目的工程优势更明显。
