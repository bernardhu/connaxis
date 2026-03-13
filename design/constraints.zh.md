# 约束（Invariants）

本项目为了追求吞吐与可控尾延迟，在一些关键路径上采用了“强约束”来换取更低的开销。违反这些约束可能直接导致：

- slice 越界 panic（`size > len(owner)` 等）
- `sync.Pool` 污染（double put / 重复复用同一块内存）导致数据串包、诡异崩溃
- buffer 无法归还 pool（len 不匹配），造成内存持续增长

下述约束属于组件内部契约：调用方必须遵守；实现层不做过多防御。

### 1. 写路径：`owner + size` 协议

#### 1.1 `owner` 必须是“整块 bucket”
- `owner` 必须来自 `pool.GAlloctor.Get(n)`，并且**保持返回时的长度不变**（不要 `owner = owner[:n]`）。
- `size` 表示有效载荷长度，必须满足：`0 < size && size <= len(owner)`。

原因：`pool.GAlloctor.Put(buf)` 依赖 `len(buf)` 来定位对应的 `sync.Pool`（`pool/pool.go:73`）。如果把 `owner` reslice 成 `owner[:size]` 再入队，后续 `Put` 会找不到匹配池，造成“归还失败/内存上涨”（更糟糕的是误归还导致 pool 污染）。

#### 1.2 `EnqueueWrite` 之后所有权转移
- 调用 `EngineConn.EnqueueWrite(owner, size)`（`connection/base.go:225`）之后，`owner` 的所有权转交给连接写队列。
- 调用方不得再对 `owner` 做任何 `Put/复用/写入`；也不得保留切片引用用于后续异步写。

写队列会在以下时机归还 `owner`：

- 数据被完全写出后（`connection/writequeue.go:82`）
- 小包合并（coalesce）成功时，新入队的 `owner` 会被**立即** `Put`（`connection/writequeue.go:43`）
- 连接关闭/清理写队列时（`connection/writequeue.go:103`）

换句话说：`EnqueueWrite` 返回后，`owner` 可能已经被立即归还给 pool（coalesce 路径），外部继续持有/读写都会变成“用已释放内存”。

### 2. `AddCmd`：调用方可见契约
- 对业务代码 / handler 来说，跨 goroutine 回写的公开入口是 `EngineConn.AddCmd(cmd, data)`（`connection/base.go:196-198`）。
- `AddCmd` 会在入队前把 `data` 复制到引擎内部 buffer（`eventloop/loopConn.go:588-598`），后续生命周期由引擎维护。
- 因此调用方不需要、也不能参与内部 buffer 的 pool 回收；也不应假设 `AddCmd` 是“零拷贝”或“所有权转移”接口。
- `AddCmd` 返回后，调用方可以继续复用或修改自己传入的 `data` 切片（前提是调用方自己保证并发同步）；引擎不会依赖该切片的后续内容。
- 若 `AddCmd` 返回错误（例如分配失败或命令队列已满，见 `eventloop/loopConn.go:590-594`、`eventloop/loopConn.go:604-612`），本次命令未成功入队，引擎会回收已申请的内部资源。

文档不展开 `CmdData` / `reset()` / 写队列之间的内部所有权细节；这些属于实现层约束，由引擎维护，不作为外部调用方的使用前提。

### 3. RingBuffer：单线程 / 单 loop 约束
- `ringbuffer.RingBuffer` 内部锁是 `internal.Fakelock`（空实现，`internal/spinlock.go:21`），因此 **RingBuffer 不是 goroutine-safe**。
- 必须保证同一个连接的 `Recvbuf()` 仅在所属 event-loop goroutine 内访问/修改（例如 `processRead` 及 handler 回调栈内）。
- 跨 goroutine 发送数据走 `AddCmd`，不要在 worker-pool 里直接读写连接的 `Recvbuf/FlushN/EnqueueWrite` 等状态。

### 4. Poller 约束：Level Triggered
- 本库使用 Linux Epoll 的 **Level Triggered (LT)** 模式（默认行为）。
- 一旦产生读就绪事件，如果不一次性将 Socket 缓冲区读空（或者一直读到 EAGAIN），Poller 会持续触发就绪事件。
- 这不仅会产生额外的系统调用开销，还可能导致“忙轮询”占用 CPU。因此，`processRead` 必须尽可能通过 buffer 循环读取，直到无数据可读。

### 5. TLS 握手约束
- TLS 握手是 CPU 密集型操作。
- 为了防止 Accept Loop 被大量握手请求阻塞，系统限制了 `TlsHandshakeWorkers` 的数量，默认与 CPU 核心数相关。
- 同时设置了 `TlsHandshakeMaxPending` 上限，超过此限制的新连接可能会被拒绝或延迟处理，以保护现有服务质量。

### 6. kTLS 密钥导出约束（KeyLogWriter）
- 当前 kTLS 实现依赖 `crypto/tls` 的 `KeyLogWriter` 导出会话密钥。
- 启用 kTLS 等同于允许导出会话密钥，虽然实现不写文件，但在合规/审计视角仍被视为“密钥可导出”。
- 若需严格合规或禁止密钥导出，必须改为 fork `crypto/tls` 或采用 OpenSSL/BoringSSL 方案。

### 7. TCP/HTTP/WS 同端口复用约束（`ConnaxisTcpHttpWsHandler`）
该 handler 通过 sniff 首包的“方法 token”来决定协议路由（TCP vs HTTP，HTTP 再升级到 WS）。

- **硬约束**：自定义 TCP 协议的首包不能以 `GET ` / `POST ` / `PUT ` 等 HTTP 方法开头，否则会被误判为 HTTP。
- **分片行为**：如果首包过短且看起来像 HTTP 方法前缀（例如 `GE`），路由会延迟，要求 loop 再读更多字节后再决定协议。
- **OnConnected 时机**：`TcpProtoHandler.OnConnected` 只会在连接被判定为 TCP 之后触发（通常是首包到达时），不是 accept 时立刻触发。

如果需要完全无歧义的协议区分，建议拆端口 / 拆 listener，不要做同端口复用。

### 8. HTTP 适配器约束（`ConnaxisHttpHandler`）

- `ConnaxisHttpHandler` 更适合作为 **dispatch-only** 适配层：`OnData` 中应尽量只做解析与分发，不要执行重 CPU / 阻塞业务逻辑。
- 跨 goroutine 回写响应时，必须通过 `AddCmd` 注入回所属 loop，不要直接在 worker 中操作连接状态。
- 当前 `ConnaxisHttpHandler` 仅支持 `Content-Length` 请求体语义，不支持 chunked / streaming 请求体。
- 若业务需要长时间处理或异步返回，应在 handler 中尽早复制必要数据并把重活下放到 worker-pool。

### 9. FastHTTP 适配器约束（`ConnaxisFastHTTPHandler`）

- `ConnaxisFastHTTPHandler` 基于 `fasthttp` 对象复用模型，**请求对象生命周期非常短**。
- 从 `OnData` / dispatcher 中拿到的 `fasthttp` 请求对象（或其内部字段引用）不能直接跨 goroutine 持有使用。
- 若需要异步处理，必须先 `CopyTo` 到新的请求对象（或复制必要字段），再释放/结束当前请求上下文。
- 与 `ConnaxisHttpHandler` 一样，跨 goroutine 回写响应请使用 `AddCmd`，不要绕过 loop 直接写连接。
