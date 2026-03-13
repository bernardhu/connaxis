# 质量与基准流水线

本文档用于说明 `connaxis` 在仓库内的质量门禁与基准对比流程，覆盖 CI 和本地执行两条路径。

## 1. 目标
- 建立 Linux 目标下可持续运行的基础质量门禁（lint / test / race / goleak）。
- 统一脚本入口，降低本地复现和 CI 排查成本。
- 为性能回归提供可重复的基准对比流程（benchstat）。

## 2. 入口与职责
- CI 主流程：`.github/workflows/go.yml`
  - 负责 `lint`、`go test`、`go test -race`、`goleak`。
- 基准对比 Workflow：`.github/workflows/benchstat.yml`
  - 通过 `workflow_dispatch` 手动触发基线与目标版本对比。
- 质量聚合脚本：`scripts/run_quality_pipeline.sh`
  - 在本地串联 lint/test/race/goleak/benchstat 等步骤，产出汇总。
- 单项脚本：
  - race：`scripts/run_race.sh`
  - goleak：`scripts/run_goleak_pilot.sh`
  - benchstat：`scripts/benchstat_compare.sh`

## 3. 当前覆盖范围
- [x] `golangci-lint` + `go test` 基线检查（Linux 目标）
- [x] `go test -race` 专用 CI Job（`race`）
- [x] `goleak` 覆盖 `connection` / `evhandler` / `eventloop` / `websocket`
- [x] `benchstat` 手动对比 Workflow（`workflow_dispatch`）
- [x] 一键质量脚本：`scripts/run_quality_pipeline.sh`

## 4. 本地执行方式
- 快速质量检查（默认 quick）：
  - `./scripts/run_quality_pipeline.sh`
- 指定 profile：
  - `./scripts/run_quality_pipeline.sh quick`
  - `./scripts/run_quality_pipeline.sh full`
- 仅执行指定步骤（示例）：
  - `RUN_LINT=0 RUN_BENCHSTAT=0 ./scripts/run_quality_pipeline.sh`
- 单独执行 goleak（推荐显式指定 `GOCACHE`）：
  - `GOCACHE="${TMPDIR:-/tmp}/connaxis-gocache" ./scripts/run_goleak_pilot.sh`
- 单独执行 benchstat（示例）：
  - `./scripts/benchstat_compare.sh main HEAD ./websocket . 5`

## 5. 结果产物与排查建议
- 聚合脚本默认输出到：`benchmark/quality-results/<run_id>/`
  - 关键文件：`summary.txt`、`logs/*.log`
- 建议排查顺序：
  - 先看 `summary.txt` 中失败步骤
  - 再看对应步骤日志
  - 最后按步骤脚本单独复跑定位

## 6. 后续演进
- [ ] 增加 `connection/eventloop` 更高价值场景用例（不仅是基础单测）
- [ ] 稳定化 `race` 的更大包覆盖策略（避免一次性全量导致 CI 抖动）
- [ ] 明确 `full` profile 的行为契约并补充回归测试
- [ ] 将 WS/WSS 压测场景纳入可选夜间任务
