# Connaxis 设计文档总览（中文）

本文档集用于说明 `connaxis` 的核心架构设计、工程约束、性能与验证方法、kTLS 路线与同类框架对比，面向以下读者：

- 需要评估 `connaxis` 是否适合网关/代理类场景的架构与平台工程师
- 准备在 `connaxis` 上做二次开发的服务端开发者
- 需要理解性能取舍、约束边界与运行时保护策略的维护者

边界说明：

- 对外公开包面见 `docs/API_BOUNDARY.md`
- `listener/dial/server` 这类 wiring 代码以及 `internal/` 下内容应视为实现细节，不应作为对外稳定架构模块理解

## 阅读顺序（推荐）

1. [`design/architecture_diagram.zh.md`](./architecture_diagram.zh.md)
   - 先看整体组件分层、主从 Reactor 与内存池位置
2. [`design/design.zh.md`](./design.zh.md)
   - 了解性能目标与具体实现策略（零拷贝、写队列、流控、过载保护）
3. [`design/atls_runtime.zh.md`](./atls_runtime.zh.md)
   - 理解用户态异步 TLS 路径如何把 `crypto/tls` 接入非阻塞 event loop
4. [`design/constraints.zh.md`](./constraints.zh.md)
   - 理解调用契约与高性能路径的“强约束”边界（尤其是内存所有权与 `AddCmd`）
5. [`design/comparison.zh.md`](./comparison.zh.md)
   - 了解与 `tidwall/evio`、`gnet`、`netpoll` 的定位差异与工程 tradeoff
6. [`design/performance_methodology.zh.md`](./performance_methodology.zh.md)
   - 查看性能/兼容性测试的发布规范、对比方法学与报告模板
7. [`design/quality_and_benchmark_pipeline.zh.md`](./quality_and_benchmark_pipeline.zh.md)
   - 查看 CI 与本地质量门禁、goleak 覆盖范围、benchstat 对比流程
8. [`design/ktls_status_and_roadmap.zh.md`](./ktls_status_and_roadmap.zh.md)
   - 查看 kTLS 的现状、风险与后续演进方向

## 文档地图

- **架构图**: [`design/architecture_diagram.zh.md`](./architecture_diagram.zh.md)
- **核心设计细节**: [`design/design.zh.md`](./design.zh.md)
- **aTLS 运行时设计**: [`design/atls_runtime.zh.md`](./atls_runtime.zh.md)
- **运行时与调用约束**: [`design/constraints.zh.md`](./constraints.zh.md)
- **同类框架对比**: [`design/comparison.zh.md`](./comparison.zh.md)
- **对外宣讲版对比摘要**: [`design/comparison_pitch.zh.md`](./comparison_pitch.zh.md)
- **性能与验证方法**: [`design/performance_methodology.zh.md`](./performance_methodology.zh.md)
- **质量与基准流水线**: [`design/quality_and_benchmark_pipeline.zh.md`](./quality_and_benchmark_pipeline.zh.md)
- **kTLS 现状与路线图**: [`design/ktls_status_and_roadmap.zh.md`](./ktls_status_and_roadmap.zh.md)

## 阅读说明

- 文档侧重工程能力、约束与边界条件，不以单一 benchmark 数字做结论。
- kTLS 相关内容包含 Linux 特定路径，需结合内核版本、密码套件与运行环境理解。
- 文中涉及的代码位置（文件/行号）可能随版本演进发生漂移，应以当前仓库代码为准。
