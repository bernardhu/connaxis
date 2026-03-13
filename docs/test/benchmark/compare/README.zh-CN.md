# 跨框架基准测试

本目录提供**可对比的服务端实现和统一压测客户端**，覆盖：
- `tidwall/evio`
- `panjf2000/gnet`
- `cloudwego/netpoll`
- `connaxis`

## 推荐流程（一条命令）

```sh
cd benchmark/compare
NET=tcp CONNS_LIST=5000 PAYLOADS_LIST=128,512 DURATION=30s INCLUDE_TLS=1 ./run_full.sh :5000
```

若要测试更多连接规模，可覆盖 `CONNS_LIST`（例如：`50,100,1000,5000`）。

可选 soak：

```sh
SOAK=1 DURATION_SOAK=30m CONNS_SOAK=50 PAYLOAD_SOAK=64 ./run_full.sh :5000
```

可选背压场景：

```sh
INCLUDE_BP=1 BP_READ_DELAY=5ms ./run_full.sh :5000
```

默认结果目录：`benchmark/compare/results/<YYYYMMDD_HHMMSS>/`。  
可通过 `RESULTS_DIR=...` 或 `RUN_ID=...` 覆盖输出路径。

### 组合流程（基础 + 背压 + Soak）

```sh
NET=tcp CONNS_LIST=5000 DURATION=60s INCLUDE_TLS=1 \
BP_READ_DELAY=5ms SOAK=1 DURATION_SOAK=5m CONNS_SOAK=5000 PAYLOAD_SOAK=512 \
./run_bundle.sh :5000
```

组合报告会输出到 `benchmark/compare/results/<RUN_ID>_combined/`。

## 手动构建

在本目录执行：

```sh
cd benchmark/compare

go mod tidy
```

构建服务端：

```sh
mkdir -p bin
go build -o bin/connaxis_server ./connaxis_server
go build -o bin/tidwall_server ./tidwall_server
go build -o bin/gnet_server ./gnet_server

# netpoll 需要 build tag
go build -tags netpoll -o bin/netpoll_server ./netpoll_server
```

构建客户端：

```sh
go build -o bin/client ./client
```

## 脚本职责（手动流程）

- `run.sh`: 单个 case（framework + mode + payload + conns）
- `run_matrix.sh`: 单个 framework 的多模式/多 payload 矩阵
- `run_all.sh`: 所有 framework，按 `CONNS_LIST` 依次执行
- `run_full.sh`: 一条命令封装（run_all + merge + report + charts [+ 可选 soak]）
- `run_bundle.sh`: 基础 + 背压 + 可选 soak 的组合流程
- `run_soak.sh`: 所有 framework 的长时 TCP soak
- `merge_results.sh`: 合并 `results_*.csv` 到 `results_all.csv`
- `merge_runs.sh`: 将多个 run 目录合并为一套组合结果
- `generate_report.sh`: 基于 CSV + env 生成 `docs/test/benchmark/PERF_REPORT.md`
- `generate_charts.sh`: 生成 `docs/test/benchmark/PERF_REPORT_CHARTS.md`

## 常用环境变量

- `CONNS_LIST=5000`（默认；`run_all.sh`/`run_full.sh` 使用）
- `DURATION=30s`（每个 case 的时长）
- `PAYLOADS_LIST=128,512`（默认 payload 集合）
- `INCLUDE_TLS=1`（为非 connaxis 框架开启 TLS/WSS 代理测试）
- `INCLUDE_BP=1` + `BP_READ_DELAY=5ms`（TCP 背压场景）
- `LOOPS=<n>`（强制 connaxis/tidwall/gnet 的 event-loop 数）
- `RESULTS_DIR=...` 或 `RUN_ID=...`（输出目录控制）
- `GENERATE_REPORTS=0`（`run_full.sh` 中跳过报告与图表，`run_bundle.sh` 会用到）

## 基准环境记录建议

每次压测建议记录（服务端 + 客户端）：

- OS / 内核：`uname -a`
- CPU 型号 / 核数：`lscpu | egrep 'Model name|CPU\\(s\\)'`
- 内存：`free -h`
- 网卡 / MTU：`ip link show`
- Go 版本：`go version`

网络 sysctl（若有调整请记录）：

```sh
sysctl net.core.somaxconn
sysctl net.core.netdev_max_backlog
sysctl net.ipv4.tcp_max_syn_backlog
sysctl net.ipv4.tcp_fin_timeout
sysctl net.ipv4.tcp_tw_reuse
sysctl net.ipv4.tcp_timestamps
sysctl net.ipv4.tcp_keepalive_time
sysctl net.ipv4.tcp_keepalive_intvl
sysctl net.ipv4.tcp_keepalive_probes
```

## 统计采集

```sh
# 先启动服务并拿到 PID
./collect_stats.sh <pid> stats.csv 1
```

## 汇总结果

```sh
./summarize_results.sh connaxis 5000
```

## CPU/RSS 汇总

```sh
./summarize_stats.sh results/connaxis_tcp_128_5000_cpu_rss.csv
```

## 环境与报告生成

```sh
./collect_env.sh results/env.json
./merge_results.sh results
./generate_report.sh
./generate_charts.sh results ../../docs/test/benchmark/PERF_REPORT_CHARTS.md
```

## 运行示例

### TCP Echo

```sh
./connaxis_server -mode tcp -addr :5000
./client -mode tcp -addr 127.0.0.1:5000 -c 200 -payload 64 -d 30s
```

### HTTP

```sh
./gnet_server -mode http -addr tcp://:5000
./client -mode http -addr 127.0.0.1:5000 -c 200 -d 30s
```

### WS

```sh
./tidwall_server -mode ws -addr :5000
./client -mode ws -addr 127.0.0.1:5000 -c 200 -payload 64 -d 30s
```

### TLS / WSS

- `connaxis_server` 原生支持 TLS/WSS。
- `gnet_server`、`tidwall_server`、`netpoll_server` 在此基准中通过 TLS 代理实现。
  - 在矩阵测试中设置 `INCLUDE_TLS=1` 即可纳入 TLS/WSS。

```sh
./connaxis_server -mode tls -addr :5000 -cert ../certs/cert.pem -key ../certs/key.pem
./client -mode tls -addr 127.0.0.1:5000 -c 200 -payload 64 -d 30s
```

---

## 说明

- 各服务端实现相同的最小协议处理逻辑，以减少偏置。
- TCP/TLS 使用固定长度 payload 回显（无额外 framing）。
- HTTP 仅判断 `\r\n\r\n`，返回固定响应。
- WS 支持单帧、小包、无分片场景。
- 可通过 `LOOPS=<n>` 指定 connaxis/tidwall/gnet 的 event-loop 数。此 harness 中 netpoll 为单 event-loop，但仍设置 `GOMAXPROCS` 以保持环境对齐。
- 正式性能结论只使用裸机 Linux 主机结果。`linux-lab`/Lima 仅用于功能验证，不作为最终性能基线。

## 当前正式基线

- 主机：专用裸机 Linux 压测主机
- 系统 / 内核：`Ubuntu 22.04`，`Linux 5.15.0-172-generic`
- 架构：`x86_64`
- 项目路径：压测主机上的 `<repo>/benchmark/compare`
- Run ID：`server_compare_20260312_2230`

执行命令：

```sh
cd <repo>/benchmark/compare
RUN_ID=server_compare_20260312_2230 NET=tcp CONNS_LIST=5000 PAYLOADS_LIST=128,512 DURATION=30s INCLUDE_TLS=1 ./run_full.sh :5000
```

结果入口：

- `benchmark/compare/results/server_compare_20260312_2230/results_all.csv`
- `docs/test/benchmark/PERF_REPORT.md`

详细方法论与报告格式见：`design/performance_methodology.en.md`。
