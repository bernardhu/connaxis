# 核心设计与实现细节

本文档详细描述了 `connaxis` 为了达成高性能目标所采用的具体实现策略。

### 1. 核心设计目标与实现对应

#### 1.1 高吞吐与低延迟（High Throughput & Tail Latency）
- **I/O 模型**: 采用 **Reactor 模式**。
  - **Per-loop listener (`LoopConn`)**: 每个工作 loop 都绑定自己的一组 listener，并直接执行 accept。
  - **Reuseport 分流**: 运行时依赖 `SO_REUSEPORT` 形成“一 loop 一 listener”的接入模型，而不是单个中心 acceptor 收到连接后再二次分发。
  - **连接 loop (`LoopConn`)**: 负责 accept、读写以及所属连接状态管理。
- **事件触发**: 使用 **Level Triggered（水平触发）** 模式（默认 Epoll 行为）。这要求代码必须一次性处理完就绪事件，或者在未处理完时正确管理状态。
- **内存管理**:
  - 使用全局 **`pool.GAlloctor`**。
  - 采用 **Power-of-2（2 的幂次）** 分级策略（Step=1024, Rank=...），结合 `sync.Pool` 复用内存块，极大降低 GC 压力。
- **系统调用优化**:
  - 批量处理事件（`EpollWait` 获取多条事件）
  - 动态调整事件缓冲区大小（根据负载自动扩容）

#### 1.2 接收端优化（Recv Optimization）
- **共享接收缓冲**: 每个工作循环（`LoopConn`）持有一个大的共享 buffer（`recvbuf`）。
- **读取策略**:
  - 优先将数据读入共享 buffer，减少单连接的内存分配。
  - **Zero-Copy 尝试**: 如果数据包完整且在共享 buffer 内，直接切片传递给上层（需上层配合生命周期管理）。
  - **Private Buffer**: 仅在遇到半包（Sticky Packet）或共享 buffer 不足时，才将数据拷贝到连接私有的 `RingBuffer`。

#### 1.3 发送端优化（Write Optimization）
- **向量写（Vectorized Write）**: 使用 `writev` 系统调用。
- **合并写**: 自动合并 `RingBuffer` 的头尾部分和 `ZeroCopy` 队列中的数据块，通过一次系统调用发送，减少内核态切换开销。
- **写队列**: 实现了一个高效的写队列，支持普通 buffer 和 ZeroCopy buffer 的混合排队。

#### 1.4 轻量级回调与异步处理（Lightweight OnData）
- **设计哲学**: `OnData` 回调必须非阻塞且极快。
- **Worker Pool 模式**: 建议在 `OnData` 中仅进行协议解码，将耗时的业务逻辑派发给外部 Worker Pool。
- **异步回写（`AddCmd`）**: Worker 处理完毕后，通过 `AddCmd` 接口将响应数据安全地“注入”回 I/O 循环的发送队列，实现线程安全的异步回写。

#### 1.5 公平性与流控（Fairness & Flow Control）
为防止单个连接或某类事件饿死其他事件，系统在关键路径上设置了配额限制（见 `eventloop/tuning.go`）：

- **Accept 限制**: `MaxAcceptPerEvent`（默认 128）- 限制单次事件循环接受的新连接数
- **Read 限制**: `MaxReadBytesPerEvent`（默认 256KB）- 限制单次事件循环从单个连接读取的字节数
- **Write 限制**: `MaxFlushBytesPerEvent`（默认 256KB）- 限制单次事件循环写入单个连接的字节数
- **Command 限制**: `MaxCmdPerEvent`（默认 1024）- 限制单次循环处理的外部命令数量，防止外部洪泛攻击导致 I/O 饿死
- **TLS 握手限制**: 控制并发握手数量，通过 `TlsHandshakeWorkers` 和 `TlsHandshakeMaxPending` 避免 CPU 过载

#### 1.6 TLS 引擎路径（aTLS / kTLS）
- **引擎选择**: TLS 路径支持显式 `atls` 与 `ktls` 模式。
- **运行时回退**: 即使请求 kTLS，若系统能力、内核版本、密码套件或协商结果不满足条件，也会回退到标准异步 TLS 路径，以保证服务可用性优先。
- **Accept 路径保护**: TLS 握手仍受 `TlsHandshakeWorkers` / `TlsHandshakeMaxPending` 等限制约束，避免在握手高峰时阻塞 Accept Loop。
- **设计边界**: kTLS 是 Linux 特定优化路径，应视为“可选加速层”，而不是替代所有 TLS 场景的唯一实现。
- **相关文档**:
  - kTLS 现状与路线图：`design/ktls_status_and_roadmap.zh.md`
  - 调用约束与合规注意事项：`design/constraints.zh.md`

### 2. 设计亮点总结（Architecture Highlights）

#### 2.1 全链路零拷贝设计（Zero-Copy Pipeline）
从读取到发送，我们尽可能减少了一切不必要的内存拷贝：

1. **Read Path**: 数据直接读入 Loop `Shared Buffer`。如果协议包完整，通过切片（Slice）直接传递给 `OnData` 回调。仅在“粘包”发生且 Shared Buffer 空间不足时，才由于无法完整存放而发生一次拷贝到 `Connection RingBuffer`。
2. **Write Path**: 用户调用 `EnqueueWrite(owner, size)` 传递的是内存块的所有权（Pointer），而非数据拷贝。底层利用 `writev` 配合 `iovec`，将 `RingBuffer` 的头尾部分和 `ZeroQueue` 中的内存块一次性提交给内核。

#### 2.2 锁竞争消除（Lock-Free Philosophy）
- **Per-Loop Design**: 每个 Loop 是一个独立的 Goroutine，管理一组从属的 FDs。在此 Loop 内的所有可读、可写、超时检查操作均是**串行化的单线程逻辑**。
- **RingBuffer 无锁化**: 由于每个连接严格归属于一个 Loop，其内部的 `RingBuffer` 不需要互斥锁（Mutex），彻底消除了高频读写下的锁竞争开销。
- **Command Queue**: 唯一的跨协程交互（如外部 Worker 回写数据）通过带缓冲的 Channel（`CmdChan`）进行。虽然 Channel 本身有锁，但相比于对每个连接加锁，这种聚合式的命令处理大大减少了临界区碰撞。

#### 2.3 极致的内存友好（Memory Efficiency）
- **Power-of-2 Allocator**: 针对网络 I/O 特征定制的 `sync.Pool` 包装器。它将内存按 1k, 2k, 4k... 分级管理，极好地契合了网络包大小分布，使得内存复用率接近 100%。
- **Buffer Reuse**: 连接关闭时，其持有的 Buffer 并不销毁，而是归还给 Pool。这种“借用-归还”机制使得在高并发短连接场景下，堆内存分配率极低，GC 几乎无感。

#### 2.4 弹性伸缩与过载保护（Scalability & Protection）
- **Adaptive Buffer**: 连接的私有 `RingBuffer` 支持动态扩容，适应突发流量；同时在空闲时支持缩容（Trim），回收内存。
- **Backpressure**: 内置多维度的流控机制（Accept 频率、读写字节数限制）。配合 `TLS Handshake` 的队列限制，系统在面对突发流量洪峰时表现为“优雅降级”而非“直接崩溃”。
- **Idle Check**: 基于时间轮（或链表）的高效空闲连接扫描，及时断开死链，释放文件描述符资源。
