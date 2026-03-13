# Connaxis 库架构设计

本图展示了 `connaxis` 库的高层架构设计。

```mermaid
graph TD
    %% 定义子图和节点
    subgraph InitScope ["初始化与控制 (Initialization & Control)"]
        Server["服务器核心 (Server)"]
        DialerMng["拨号管理器 (DialerMng)"]
        TLSW["TLS 握手协程池"]
    end

    subgraph WorkScope ["统一的每 loop 运行时 (LoopConn)"]
        WL["LoopConn 实例"]
        PollW["轮询器 (Epoll LT/Kqueue)"]
        Listener["每 loop 监听器"]
        AcceptPlain["Accept 明文连接"]
        AcceptTLS["Accept TLS 连接"]
        CmdChan["命令通道 (CmdChan)"]
        AttachConn["挂接连接"]
        ConnDispatch{"分发逻辑"}
    end

    subgraph ConnScope ["连接状态 (Conn)"]
        RecvBuf["接收环形缓冲 (RecvBuf)"]
        SendBuf["发送环形缓冲 (SendBuf)"]
        ZeroCopy["零拷贝逻辑 (ZeroCopy)"]
        AppLogic["用户应用逻辑"]
    end

    subgraph MemScope ["内存管理"]
        Pool["全局分配器 (Global Allocator)"]
    end

    %% 定义连线
    Server -->|管理| DialerMng
    Server -->|启动 loops| WL
    Server -->|TLS 打开时启动| TLSW

    WL -->|轮询| PollW
    PollW -->|accept 就绪| Listener
    Listener -->|明文 accept| AcceptPlain
    Listener -->|TLS accept| AcceptTLS
    AcceptPlain -->|同 loop 直接挂接| AttachConn
    AcceptTLS -->|提交 pending tls conn| TLSW
    TLSW -->|握手完成后回到所属 loop| AttachConn
    WL -->|消费| CmdChan
    AttachConn -->|注册| ConnDispatch

    PollW -->|读事件| ConnDispatch
    PollW -->|写事件| ConnDispatch

    ConnDispatch -->|读取| RecvBuf
    ConnDispatch -->|刷新| SendBuf

    RecvBuf -->|回调 OnData| AppLogic
    AppLogic -->|入队写| SendBuf
    AppLogic -->|入队零拷贝| ZeroCopy

    RecvBuf -. "申请内存 (Power-of-2)" .-> Pool
    SendBuf -. "申请内存 (Power-of-2)" .-> Pool

    %% 组件样式
    classDef core fill:#f9f,stroke:#333,stroke-width:2px,color:#000;
    classDef loop fill:#ccf,stroke:#333,stroke-width:2px,color:#000;
    classDef memory fill:#bfb,stroke:#333,stroke-width:2px,color:#000;

    class Server,DialerMng,TLSW core;
    class WL,PollW,Listener,AcceptPlain,AcceptTLS,AttachConn loop;
    class Pool memory;
```

说明：
- 本图聚焦统一的 per-loop 运行模型、TLS 握手卸载路径以及内存管理主路径，未展开 `atls / ktls` TLS 引擎分支细节。
- kTLS 路径属于连接层的可选加速实现，实际是否启用取决于运行环境与协商结果（详见 `design/ktls_status_and_roadmap.zh.md`）。

### 组件详细说明

1. **Server（服务器核心）**
   - 程序的主入口。负责初始化多个 `LoopConn`，并给每个 loop 绑定 listener。
   - 在 TLS 模式下，会启动 TLS 握手协程池。
   - 通过 `DialerMng` 管理外拨连接，支持客户端模式。

2. **LoopConn**
   - 每个 loop 都持有自己的一组 listener，并直接负责连接的 accept、读、写、关闭。
   - 运行时依赖 `SO_REUSEPORT` 做接入分流，而不是单个中心 acceptor 收到连接后再分发。
   - 明文连接在 accept 后直接在本 loop 挂接。
   - TLS 连接在 accept 后先创建 pending TLS 连接对象，再进入 TLS 握手协程池。

3. **TLS 握手协程池**
   - 将服务端 TLS 握手从 loop 热路径中卸载出来。
   - 通过 `TlsHandshakeWorkers` 和 `TlsHandshakeMaxPending` 控制并发握手数和排队上限。
   - 握手成功后，把连接回挂到原始 accept loop。

4. **Loop 运行时细节**
   - **Poller**：使用 **Level Triggered（水平触发）** 的 `epoll`/`kqueue` 等待 I/O 事件。
   - **Channels**:
     - `CmdChan`：接收来自用户层或其他 Goroutine 的异步指令（如 `Write`, `Close`）
   - **Flow Control**：内置了 `MaxRead/Write/Cmd` 等多个流控参数，防止单个连接饿死整个 Loop。
   - **TLS Path**：连接层可能运行在异步 TLS（`atls`）或 Linux kTLS 路径上；对上层回调模型保持尽量一致。

5. **Connection（连接状态）**
   - 每个连接维护独立的 `RecvBuf` 和 `SendBuf`（均为 RingBuffer 实现）。
   - **Zero-Copy**：支持直接将 `GAllocator` 分配的 `owner` buffer 传递给用户，或者直接入队发送，减少内存拷贝。

6. **Global Allocator（全局分配器）**
   - 基于 `sync.Pool` 的分级内存池。
   - 采用 **Power-of-2（2 的幂次）** 策略（1k, 2k, 4k...），极大降低了高频 I/O 场景下的 GC 压力。
