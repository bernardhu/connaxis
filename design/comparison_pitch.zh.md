# 同类框架对比（省流版）

## 一句话定位（建议）

`connaxis` 是一个面向网关/代理场景的高并发连接底座，重点在于连接治理、可控内存复用、协议接入适配，以及内建 Linux kTLS 路径与回退策略。

## 重点亮点

### 1. kTLS 是工程亮点，而不仅是“可选功能”
- `connaxis` 在连接层内建了 `atls` / `ktls` TLS 引擎路径。
- kTLS 不是外部旁路组件，而是和标准异步 TLS 路径处于同一套连接模型中。
- 条件不满足（内核、cipher、协商结果、运行环境）时可自动回退到 aTLS，降低生产启用风险。

### 2. 相比“业务侧自行拼装”，工程集成成本更低
- 在 Go 生态中，如果其他框架要做类似 kTLS/内核 TLS 集成，常见路径是业务侧额外接 OpenSSL（Go binding / cgo 等）并自行处理整合。
- 这类方案通常会引入额外的调用边界、内存管理、错误语义映射、部署依赖和运维升级成本。
- `connaxis` 的优势在于把这类集成路径前移到框架连接层，减少业务侧重复造轮子。

### 3. 协议接入与验证成本低（工程落地优势）
- 内置 `evhandler` 适配器：`ConnaxisHttpHandler`、`ConnaxisFastHTTPHandler`、`ConnaxisTcpHttpWsHandler`
- 配套 `examples` 与 `benchmark` helper：便于接入主流协议、做 side-by-side 对比与回归验证
- 对网关型业务团队来说，这些 helper 往往直接减少首版落地时间与验证成本

### 4. 高峰期行为更可控（不只看峰值吞吐）
- TLS 握手工作池与 pending 上限（`TlsHandshakeWorkers` / `TlsHandshakeMaxPending`）
- 显式流控与公平性参数（Accept / Read / Write / Cmd 配额）
- 更适合“长期运行、流量波动明显”的入口服务

### 5. 对二次开发团队更友好
- 关键约束（内存所有权、`AddCmd` 回写、RingBuffer 并发边界）有明确文档化说明
- 优点是行为更可预期；代价是调用方需要遵守更强契约

## 与常见框架的对比

### 对 `tidwall/evio`
- 更轻、更快上手；适合简单事件循环需求。
- `connaxis` 的优势在于连接治理、协议适配与 kTLS 工程集成更完整，更偏网关型底座。

### 对 `gnet`
- `gnet` 生态和文档成熟度高，通用场景非常强。
- `connaxis` 的差异化在于：连接治理、kTLS 内建路径与回退策略、以及面向网关接入的适配/helper 工程能力。

### 对 `cloudwego/netpoll`
- `netpoll` 在 RPC/短连接高并发 I/O 场景很有优势。
- `connaxis` 更偏长连接入口、协议接入与连接治理一体化能力。

## 结论

> `connaxis` 的核心差异化不只是高性能事件驱动本身，而是把网关场景真正需要的工程能力做成了框架内建路径：连接治理、协议适配、可控流控，以及 Linux kTLS 的连接层集成与自动回退。  
> 这使得团队在追求性能的同时，不必把大量精力花在外部 TLS 栈集成、协议 glue code 和运行时保护策略的重复建设上。

## 使用边界

- kTLS 是 Linux 特定优化路径，效果依赖内核版本、cipher 与部署环境。
- 不应将不同环境下的 TLS/kTLS 测试结果直接做“框架优劣”结论。
- 如需对外给出具体数据，请附方法学文档与测试环境说明：`design/performance_methodology.zh.md`。
