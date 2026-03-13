# aTLS 运行时设计

本文档专门说明 `connaxis` 的用户态异步 TLS（`atls`）路径：为什么需要这条路径、它如何接入 event loop，以及哪些取舍是有意为之。

## 1. 范围

本文档只讨论用户态 TLS 路径：

- `ATLSConn`
- `tlsBufferConn`
- `tls.Conn`
- 非阻塞 fd 与 `crypto/tls` 之间的读写桥接

Linux kTLS 快路径的细节见：

- [`design/ktls_status_and_roadmap.zh.md`](./ktls_status_and_roadmap.zh.md)

## 2. 为什么需要这条路径

`crypto/tls.Conn` 的设计前提是 `net.Conn` 风格的流式 I/O。

而 `connaxis` 的运行时模型是：

- 非阻塞 fd
- 事件驱动的读写通知
- 显式背压与有界缓冲

这两种模型不能直接拼起来，所以 `atls` 路径的作用就是：在不重写标准库 TLS 的前提下，把两者桥接起来。

这里有一个容易混淆但必须明确的区分：

- 项目已经不再支持 `auto` 这种 TLS 引擎自动选择模式
- 调用方必须显式选择 `atls` 或 `ktls`
- 但当调用方显式选择 `ktls` 时，运行时仍可能在“当前连接级别”回退到 `atls`

也就是说，去掉的是“配置层自动选引擎”，保留的是“`ktls` 失败后的连接级回退”。

换句话说：

- `crypto/tls` 继续负责 TLS 状态机
- `connaxis` 继续负责 event loop 运行时
- `tlsBufferConn` 负责把两种模型接起来

## 3. 主要对象

### 3.1 `ATLSConn`

`ATLSConn` 是用户态 TLS 连接的运行时 owner。

它持有：

- 面向 event loop 的连接状态（`fd`、写队列、回调、统计）
- 用户态 TLS 状态机（`Conn *tls.Conn`）
- 桥接对象（`bio *tlsBufferConn`）
- 握手时序与状态
- kTLS 过渡/回退路径上可能用到的 `preRead` 明文

这个 ownership 是有意保持在连接对象上的：`ATLSConn` 是连接本体，`tlsBufferConn` 只是适配器。

### 3.2 `tlsBufferConn`

`tlsBufferConn` 是一个假的 `net.Conn`，只用来驱动 `tls.Conn`。

它有两种工作状态：

- 握手阶段：`direct net.Conn` 非空，`tls.Conn` 直接对这个阻塞连接读写
- event-loop 阶段：`direct == nil`，`tls.Conn` 从 `cin` 读取密文、把密文写入 `cout`

它本身不实现 TLS，只负责 I/O 形态适配。

### 3.3 `tls.Conn`

`tls.Conn` 仍然是真正的 TLS 引擎，负责：

- 握手
- record framing
- 加密
- 解密
- 连接状态 / ALPN / SNI / exporter

## 4. 读路径

底层 fd 上来的是真正的 TLS 密文，调用方要的是明文。

用户态 `atls` 读路径是：

```text
fd read event
-> ATLSConn.Read()
   -> tls.Conn.Read()
      -> tlsBufferConn.Read()
         -> cin.Read()
```

只有当 `cin` 里已经有 TLS 密文时，这条链路才能直接产出明文。

如果 `cin` 是空的，运行时会这样补料：

```text
ATLSConn.Read()
-> tls.Conn.Read()
   -> tlsBufferConn.Read()
      -> would-block
-> ATLSConn.readCiphertext()
   -> unix.Read(fd, ...)
   -> 把密文写入 cin
-> 再次 tls.Conn.Read()
```

所以关键点是：

- `tls.Conn` 从 `cin` 消费 TLS 密文 record
- `ATLSConn.readCiphertext()` 负责从底层 fd 把密文补进 `cin`

## 5. 写路径

调用方写入的是明文，底层 fd 最终发出去的是 TLS 密文。

用户态 `atls` 写路径是：

```text
ATLSConn.Write(plaintext)
-> tls.Conn.Write(plaintext)
   -> tlsBufferConn.Write(ciphertext)
      -> cout.Write(ciphertext)
-> ATLSConn.FlushN()
   -> writev()/write() to fd
```

关键点是：

- 加密发生在 `tls.Conn.Write` 里
- `tlsBufferConn.Write` 接到的已经是加密后的 TLS record
- `cout` 只是中转缓冲，不是 TLS 引擎

## 6. 为什么看起来很绕

因为这套设计本质上就是桥接模型，而不是原生 event-loop TLS 栈。

这意味着：

- 内核可读/可写事件决定读写时机
- `tls.Conn` 决定 TLS record 什么时候可消费
- `tlsBufferConn` 夹在中间，把这两套模型拼在一起

它比自定义 record-driver 更绕，但比“自己拥有一套 userspace TLS 实现”便宜得多。

## 7. copy 和缓冲取舍

`atls` 路径不是零拷贝 TLS 设计。

典型读路径里至少有：

- `fd -> cin`
- `cin -> tls.Conn`
- `tls.Conn -> 调用方 buffer`

典型写路径里至少有：

- `调用方 buffer -> tls.Conn`
- `tls.Conn -> cout`
- `cout -> fd`

这是有意接受的现实。`atls` 的目标是兼容性和维护成本可控，不是最小 copy 的 record 处理。

## 8. 慢读 / 慢写行为

### 8.1 慢读

如果应用层消费明文很慢：

- `tls.Conn` 无法快速排空解密后的处理进度
- `ATLSConn` 仍可能持续收到 fd 可读事件
- `readCiphertext()` 会持续把密文喂进 `cin`，直到缓冲压力出现

这条路径是有界的，不提供无界隐藏缓冲。

一旦桥接层容量耗尽，代码会 fail-fast，而不是静默积压压力。

### 8.2 慢写

如果底层 socket 一时发不动：

- `tls.Conn.Write()` 仍会先产出密文
- `tlsBufferConn.Write()` 把密文放进 `cout`
- `FlushN()` 再逐步把 `cout` 往 fd 排空

`cout` 的设计目标是中转，不是“大 backlog 水库”。

容量耗尽时，路径同样会 fail-fast，而不是把压力变成不透明的队列增长。

## 9. 为什么这条路径仍然值得保留

即便多了一层桥接，`atls` 仍有明确价值：

- 它是 Linux / 非 Linux 都能工作的可移植 TLS 路径
- 它基于 Go 标准库 TLS，实现维护成本更低
- 当 kTLS 不可用或不适用时，它是明确的回退基线
- 它在 event loop 模型下保持了清晰的连接 ownership

一句话：

- `ktls` 是加速路径
- `atls` 是可移植的用户态基线

## 10. 为什么不直接改成自定义 TLS record driver

理论上可以，但代价大很多。

如果要去掉大部分桥接逻辑，项目就需要拥有更深的 TLS 实现边界，例如 fork 并重组 `crypto/tls`，让它围绕显式 record buffer 工作，而不是围绕 `net.Conn`。

这样确实能减少绕路，但同时意味着：

- 更高维护成本
- 更强的 Go TLS 内部耦合
- 更大的 TLS 1.2 / 1.3 行为验证负担

对当前项目目标来说，桥接模型仍然是更简单、也更容易自证正确的选择。
