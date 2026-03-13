# 性能与验证方法（对外发布版）

本文档用于规范 `connaxis` 的性能测试、协议验证与对外对比发布方式，目标是：

- 提高结果复现性与可解释性
- 避免不公平对比或误导性结论
- 将“性能结果”与“运行条件/约束/回退行为”一起报告

适用范围：

- 吞吐/时延性能测试（TCP / HTTP / WS / TLS）
- TLS/kTLS 功能与兼容性验证
- 与同类框架的工程向对比发布

## 1. 发布原则

### 1.1 可复现（Reproducible）
- 必须记录测试环境（CPU、内核、Go 版本、网卡、NUMA、容器/裸机等）。
- 必须记录服务端关键配置（worker 数、TLS 引擎模式、per-loop listener 模型前提、流控参数）。
- 必须记录客户端压测工具与参数（并发、连接数、持续时间、warmup 时间）。

### 1.2 可解释（Explainable）
- 不只给单一吞吐数字；至少同时给吞吐、延迟分位数、错误率。
- 对 TLS/kTLS 路径，必须说明是否发生回退（fallback）及原因。
- 明确测试目标：是“峰值吞吐”还是“稳定运行下的可控尾延迟”。

### 1.3 公平（Fair）
- 对比不同框架时，尽量统一：硬件、内核、客户端、协议、payload、并发模型、持续时间。
- 不把 Linux kTLS 优化路径与对手的纯用户态 TLS 在不同条件下直接当作“框架优劣”结论。
- 对某框架“不支持/未配置”的能力应明确标注，而不是隐含忽略。

### 1.4 透明（Transparent）
- 明确披露已知约束与前提（见 `design/constraints.zh.md`）。
- 明确披露测试过程中关闭/开启了哪些保护机制（例如 `TlsHandshakeWorkers`、`TlsHandshakeMaxPending`）。
- 若使用了自定义补丁、实验分支或特殊内核参数，必须注明。

## 2. 测试分类与目标

### 2.1 性能类（Performance)
- **吞吐测试**：最大请求/响应吞吐、字节吞吐
- **延迟测试**：P50/P90/P99/P99.9（含尾延迟）
- **稳定性测试**：长时间运行下吞吐波动、错误率、内存增长趋势
- **退化行为测试**：高负载/突发流量下的可控退化（而非只看峰值）

### 2.2 功能与兼容性类（Validation / Compatibility）
- **TLS 基础握手与协议版本验证**（TLS 1.2 / 1.3）
- **kTLS 能力探测与启用验证**（内核模块、内核版本、cipher 适配）
- **WebSocket 协议兼容性**（例如 Autobahn 测试）
- **默认 TLS 基线验证**（`smoke`、`testssl`、`tlsanvil`）
- **TLS-only 扩展检查**（`tlsfuzzer`、`bogo`），用于更严格的负面/互操作覆盖

### 2.3 回归类（Regression）
- 代码变更前后对比（同机同参数）
- 修复特定 bug 后的定向回归（如单 case 复测）
- kTLS 与 aTLS 路径行为一致性与回退一致性回归

## 3. 测试维度矩阵（建议最少覆盖）

### 3.1 协议维度
- TCP echo
- HTTP（短连接 / Keep-Alive）
- WebSocket（明文 WS / TLS 上的 WSS）
- TLS 握手与纯 TLS 数据路径（aTLS vs kTLS）

### 3.2 流量形态维度
- 小包高 QPS（如 64B / 256B）
- 中等 payload（如 1KB / 4KB）
- 大包/流式（如 16KB+）
- 长连接持续收发 vs 短连接频繁建连

### 3.3 并发维度
- 连接数（低 / 中 / 高）
- 客户端并发 worker 数
- 请求 pipeline / 批量发送深度（若适用）
- TLS 握手并发（尤其是握手高峰场景）

### 3.4 TLS/kTLS 维度
- TLS 引擎模式：`atls` / `ktls`
- 协议版本：TLS 1.2 / TLS 1.3
- Cipher suites（至少标记实际协商结果）
- 是否发生 kTLS fallback（发生次数、原因分类）
- Linux 内核版本与发行版（kTLS 强相关）

### 3.5 资源与系统维度
- CPU 型号/核数/频率（含节能模式状态）
- 内存容量
- NIC 型号与 offload 相关设置（如适用）
- NUMA 拓扑与绑核策略（如适用）
- 裸机 / VM / 容器环境

## 4. 必报指标（Minimum Reporting Set）

### 4.1 性能指标
- 吞吐：`req/s`、`MB/s`（至少一项，建议两项都报）
- 延迟分位数：P50 / P90 / P99（建议加 P99.9）
- 错误率：超时、连接错误、协议错误、非预期关闭
- 成功率：请求成功率或握手成功率

### 4.2 资源指标
- 服务端 CPU 使用率（总量与热点线程）
- 内存占用（RSS / heap 趋势）
- GC 指标（Go 场景建议至少给 GC 次数或 pause 概览）
- 文件描述符/连接数峰值（适用于高连接场景）

### 4.3 TLS/kTLS 专项指标
- kTLS 启用率（成功启用连接数 / TLS 连接总数）
- kTLS fallback 统计（原因分类）
- TLS 握手耗时分位数（如果测试覆盖握手）
- TLS 协商版本 / cipher 分布（至少在报告中注明主流组合）

## 5. 环境披露模板（建议直接复制）

```text
Test Goal: (throughput / latency / compatibility / regression)
Date:
Commit:
Branch:
Server Host: (CPU / cores / RAM)
OS + Kernel:
Go Version:
Deployment: (bare metal / VM / container)
NIC / Offload Settings:
NUMA / CPU Pinning:
TLS Engine Mode: (atls / ktls)
TLS Version / Cipher (expected & observed):
Server Tuning: (MaxAcceptPerEvent / MaxReadBytesPerEvent / ...)
Client Tool + Version:
Client Params: (connections / concurrency / duration / warmup)
Workload Shape: (protocol / payload / keepalive / handshake ratio)
```

## 6. 仓库内可复用工具（建议优先使用）

以下工具/目录可作为发布前验证与佐证材料来源：

- `benchmark/compare`
  - TCP/HTTP/WS/TLS/WSS 跨框架基准 harness，并支持报告/图表生成
- `benchmark/ws-autobahn`
  - WebSocket 协议兼容性测试与报告生成
- `benchmark/tls-suite`
  - 默认 TLS 基线（`smoke / testssl / tlsanvil`）以及可选的 `tlsfuzzer / bogo` 扩展检查
- `cmd/ktlscheck`
  - kTLS 关键链路（ULP attach + crypto_info inject）快速检查
- `cmd/bogo_shim`
  - TLS 互操作/协议测试辅助路径

当前解读规则：

- 面向交付的默认 TLS matrix：`smoke / testssl / tlsanvil`
- `tlsfuzzer` 和 `bogo`：保留为 `tls-extend`，不再与默认交付基线混为一套口径

注意：
- `benchmark/.../results` 下的历史结果可作为参考样本，但对外发布时应优先使用“当前 commit、当前环境”的新结果。
- 发布文档应明确哪些结果是“本地复测”，哪些是“历史回归记录”。

## 7. 对比测试执行规则（建议）

### 7.1 单次测试规则
- 预热（warmup）后再开始采样。
- 固定持续时间（避免某些实现因提前退出看起来更快）。
- 测试期间避免混跑其他重负载任务。
- 若出现明显异常（错误率飙升、系统抖动），该轮结果作废并记录原因。

### 7.2 重复与统计规则
- 每组参数至少运行 3 次（建议 5 次）。
- 报告均值/中位数，并标注波动范围（min-max 或标准差）。
- 不只展示“最佳一次”结果。

### 7.3 框架对比规则
- 尽可能采用等价协议语义（例如同样的 keepalive、同样的 payload、同样的 handler 逻辑复杂度）。
- 若配置无法完全等价，必须在报告中明确说明差异，并解释可能影响方向。
- 对使用 kTLS 的结果，必须给出 aTLS 对照组，避免把“内核路径优化收益”混同为“框架基础开销差异”。

## 8. 对外报告建议结构（模板）

### 8.1 Executive Summary
- 测试目标
- 结论摘要（不超过 5 条）
- 主要限制条件（内核、环境、协议）

### 8.2 Setup
- 环境披露模板内容
- 服务端与客户端命令行（可脱敏）

### 8.3 Results
- 表格：吞吐 / 延迟 / 错误率 / 资源占用
- 图表：随时间吞吐曲线、尾延迟曲线（可选）
- kTLS 专项：启用率、fallback 统计、握手行为（如适用）

### 8.4 Interpretation
- 为什么会得到这些结果（结合架构、约束、回退行为解释）
- 结果适用边界（什么场景可推广，什么场景不可直接外推）

### 8.5 Appendix
- 原始日志/结果路径
- 协议兼容性报告链接（如 Autobahn / TLS suite）
- 相关 commit 与配置片段

## 9. 发布前检查清单（Checklist）

- [ ] 中英文结论表述一致（若双语发布）
- [ ] 明确测试目标（峰值吞吐 / 稳定延迟 / 兼容性 / 回归）
- [ ] 环境与配置披露完整
- [ ] 给出错误率与成功率，不只给吞吐
- [ ] 给出 TLS/kTLS 路径与 fallback 信息（若涉及 TLS）
- [ ] 给出约束与适用边界（引用 `design/constraints.zh.md`）
- [ ] 对比对象配置差异已注明
- [ ] 原始结果路径可追溯（至少团队内部可追溯）

## 10. 与其他设计文档的关系

- 架构解释：`design/architecture_diagram.zh.md`
- 实现细节：`design/design.zh.md`
- 调用与运行时约束：`design/constraints.zh.md`
- 框架定位对比：`design/comparison.zh.md`
- kTLS 现状与路线图：`design/ktls_status_and_roadmap.zh.md`
