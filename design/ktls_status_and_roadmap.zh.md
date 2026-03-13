# kTLS 实施现状与路线图

### 1. 设想
在未启用 kTLS 的标准 TLS 路径中，`connaxis` 使用异步 TLS 路径（`atls`）配合 `crypto/tls` 在用户态处理 TLS 数据，存在多层内存拷贝和 Reactor 线程被加密计算阻塞的问题。

目标是引入 **kTLS (Kernel TLS)** 技术，将对称加密/解密逻辑下沉至 Linux 内核（甚至网卡硬件），实现架构上的归一化：

- **统一路径**：握手结束后，TCP Socket 升级为 kTLS Socket，Reactor 层不再感知加密，直接读写明文。
- **性能提升**：利用 CPU AES 指令集或 NIC 硬件卸载；支持 `sendfile` 实现真正的 Zero-Copy HTTPS。

### 2. 约束与依赖

#### 2.1 OS 与 Kernel
- **内核版本**:
  - **理论最低能力**: Linux 4.17+ 可能具备 RX/TX 双向 kTLS 基础能力
  - **项目当前验证基线**: Linux 5.15+（与仓库当前已验证内核和 TLS 1.3 kTLS 预期一致）
  - **警告**: CentOS 7（Kernel 3.10）无法支持，必须降级回异步 TLS 路径
- **内核模块**: 需要加载 `tls` 内核模块（`modprobe tls`）

#### 2.2 Go Runtime
- **标准库限制**: Go 标准库 `crypto/tls` 未直接暴露通用的 Session Key/IV 导出接口；若要稳定获取更底层握手/记录细节，通常需要额外机制。
- **当前实现路径**: 当前仓库主要通过 `KeyLogWriter`、记录层辅助逻辑与内部 `ktls` 工具完成密钥材料获取/派生与序号处理（见第 4 节说明）。
- **增强方案**: 若希望进一步降低对运行时行为/内部机制的依赖，可考虑完整 fork `crypto/tls` 或使用 `unsafe`/linkname 等机制（需权衡稳定性与维护成本）。
- **TLS 1.3 限制（原生 stdlib）**: Go 标准库无法通过 `tls.Config.CipherSuites` 显式配置 TLS 1.3 cipher suites（仅适用于 TLS 1.0-1.2）；本项目通过 `internal/tls` 增加了 TLS 1.3 suite 约束能力，但仍需关注版本兼容性。

### 3. 适用场景

| 场景 | kTLS 收益 | 推荐策略 |
| :--- | :--- | :--- |
| **API Gateway (JSON/RPC)** | High（CPU Offload） | **Write + kTLS**：消除用户态加密计算，减少一次用户态 Buffer 拷贝。 |
| **Static File Server** | Extreme（Zero-Copy） | **Sendfile + kTLS**：数据完全不经过用户态，性能最大化。 |
| **Old Linux / Non-Linux** | None | **aTLS Mode (Current)**：保持现有的 Worker Pool + RingBuffer 方案作为兜底。 |

### 4. 当前实现状态（仓库快照）

以下能力在当前仓库中已经存在（以代码为准）：

- **TLS 引擎路径选择**：支持显式 `atls` / `ktls`（见 `connection/tlsengine.go`），在 kTLS 条件不满足时回退至异步 TLS 路径。
- **关键区分**：项目已经不再提供 `auto` 这种“自动选择 TLS 引擎”的模式。调用方必须显式选择 `atls` 或 `ktls`。当前保留的只是连接级运行时回退：当显式请求 `ktls` 但当前连接/内核/cipher/运行时状态不满足条件时，回退到 `atls`。
- **系统能力探测**：包含 Linux kTLS 支持判断与非 Linux 降级路径（见 `internal/ktls_support_linux.go`、`internal/ktls_support_other.go`）。
- **kTLS 密钥派生与内核注入**：`internal/ktls` 已包含 TLS 1.2 / TLS 1.3 的 AES-GCM 相关路径与 `setsockopt(SOL_TLS, ...)` 注入逻辑（见 `internal/ktls/linux.go`）。
- **连接集成与回退**：kTLS 握手、预读明文、失败回退到异步 TLS 路径已接入连接层（见 `connection/ktls_linux.go`、`connection/tlsconn.go`）。
- **基础验证工具**：提供 `cmd/ktlscheck` 用于 kTLS 关键链路检查。

说明：

- 当前实现主要通过 `KeyLogWriter` 与内部 `ktls` 辅助逻辑获取/派生密钥材料；下文中的“fork `crypto/tls`”更适合作为后续稳健性/合规增强方向，而不是当前唯一实现路径。
- kTLS 仍是 Linux 特定优化路径；实际收益与行为取决于内核版本、密码套件、部署环境与业务流量形态。

### 5. 后续演进路线图（在当前实现基础上）

在阅读本节前，需要明确区分两类结论：

- **已覆盖测试结论（当前有效）**：在已测试的环境/内核版本/协议版本/cipher/负载形态下，当前 kTLS 路径可以得出有效结论（包括功能性、兼容性与部分性能结论）。
- **路线图未完成项（继续增强）**：未勾选项很多属于“能力扩展、可观测性、合规性、跨环境覆盖面扩大”，并不等价于“当前 kTLS 路径不可用”。

换句话说：当前状态更接近“核心链路已打通并已覆盖多类测试场景”，而不是“kTLS 仍处于纯概念阶段”。

#### Phase 1: Detection & Fallback
- [x] 在显式 `ktls` 选择下实现运行时回退
  - 检查 `/sys/module/tls`
  - 尝试创建一个 dummy socket 并调用 `setsockopt(SOL_TCP, TCP_ULP, "tls")`
- [x] 在 `Server` 启动时遵循显式 `atls` / `ktls` 选择
- [ ] 补充更细粒度的观测指标（启用率、回退原因、密码套件分布）

#### Phase 2: Handshake & Key Extraction（Fork Strategy）
- [ ] **现状澄清**：当前 `internal/tls` 是轻量兼容层 + TLS 1.3 suite 约束扩展（并非完整 fork）
- [ ] **增强策略（可选）**：Fork（复制）Go 标准库 `crypto/tls` 到 `internal/tls`
  - **原因**：避免 `unsafe` 反射脆弱性和外部第三方库依赖，确保 Go 版本升级时的稳定性
- [ ] **修改点（若采用 fork）**：在 `internal/tls` 中增加握手后导出会话密钥的方法：
  - `MasterSecret` / `SessionKey`（Rx/Tx）
  - `IV`（Implicit Nonce）
  - `SequenceNumber`（当前 record 序号）
- [ ] **集成**：在握手路径中切换为“完整 fork 版 `internal/tls`”实现（仅在采用 fork 策略时）
- [ ] （合规/产品化增强）补齐合规模式（禁用密钥导出路径）与运行时开关

#### Phase 3: Kernel Injection
- [x] 实现 `setsockopt` 封装，支持 TLS 1.2（GCM）和 TLS 1.3 结构体
- [x] 注入 TX Key -> 开启内核发送加密
- [x] 注入 RX Key -> 开启内核接收解密
- [ ] （覆盖面增强）补充更多内核版本/发行版兼容性矩阵与自动化回归验证

#### Phase 4: Reactor Integration
- [x] 修改 `ATLSConn` / 连接路径以支持 kTLS 集成与回退：
  - 如果是 kTLS 模式，`Read/Write` 直接透传给底层 `syscall`，绕过 RingBuffer 加密层
  - `FlushN` 逻辑简化，不再需要 `writev` 拼凑密文 Frame
- [ ] （能力扩展）补齐 `sendfile + kTLS` 的端到端路径与基准验证（静态文件场景）
- [ ] （产品化/运维增强）增强可观测性（区分 aTLS/kTLS 路径的时延、错误、回退统计）

#### Phase 5: 质量与基准流水线
- 该部分已拆分为独立文档，详见：[质量与基准流水线](./quality_and_benchmark_pipeline.zh.md)。

### 6. 参考资料
- Linux Kernel: `Documentation/networking/tls.txt`
- Go Issue: `golang/go#44506`（kTLS support discussion）
