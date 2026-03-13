# Benchmark 与验证入口

本文档汇总当前仓库维护中的验证入口。

## 主入口

当前维护的基线是基于 Lima 的双内核工作流：

- `docs/test/linux-lab/README.zh-CN.md`
- `docs/test/linux-lab/WSS_KTLS_MATRIX.zh-CN.md`
- `docs/test/linux-lab/TLS_LIMA_REPORT.zh-CN.md`

## 测试分层

- 默认矩阵：
  - `ws / wss`：Autobahn
  - `tls`：`smoke / testssl / tlsanvil`
  - 双内核：Ubuntu `22.04 / 5.15` 与 Ubuntu `24.04 / 6.8`
- TLS 扩展专项：
  - `docs/test/tls/TLS_EXTEND.zh-CN.md`
- 仓库内 unit/integration 覆盖补充：
  - `docs/test/ws/WS_TLS_TEST_SUITE.md`

## 补充文档

- TLS matrix 说明：`docs/test/tls/TLS_TEST_MATRIX.md`
- Benchmark 方法与报告：`docs/test/benchmark/`

## 跨框架性能对比（主入口）

使用 `benchmark/compare` 执行 TCP/HTTP/WS/TLS/WSS 对比。

```sh
cd benchmark/compare
NET=tcp4 CONNS_LIST=5000 PAYLOADS_LIST=128,512 DURATION=30s INCLUDE_TLS=1 ./run_full.sh :5000
```

如果要测试更多连接规模，可覆盖 `CONNS_LIST`，例如：`50,100,1000,5000`。

可选打包跑法（base + backpressure + soak）：

```sh
cd benchmark/compare
NET=tcp4 CONNS_LIST=5000 DURATION=60s INCLUDE_TLS=1 \
BP_READ_DELAY=5ms SOAK=1 DURATION_SOAK=5m CONNS_SOAK=5000 PAYLOAD_SOAK=512 \
./run_bundle.sh :5000
```

生成的报告目标：

- `docs/test/benchmark/PERF_REPORT.md`
- `docs/test/benchmark/PERF_REPORT_CHARTS.md`

## WS/WSS 一致性（Autobahn）

当前基线不再使用旧的 ad hoc Linux-host 说明。

统一使用 Lima 双内核流程：

- 环境准备：`docs/test/linux-lab/README.zh-CN.md`
- WS/WSS/kTLS 结论：`docs/test/linux-lab/WSS_KTLS_MATRIX.zh-CN.md`

## TLS 验证（Standalone Suite）

```sh
cd benchmark/tls-suite
cp targets.env.example targets.env
bash ./run_tls_suite.sh
```

如果要在本地双内核环境上跑，按以下入口执行：

- `docs/test/linux-lab/README.zh-CN.md`

如果只想检查 Linux kTLS 能力：

```sh
go run ./cmd/ktlscheck -bench=false
```

## Benchmark 证书材料

- 本地证书：`benchmark/certs/local/lima-local-cert.pem`
- 本地私钥：`benchmark/certs/local/lima-local-key.pem`
- 生成脚本：`benchmark/certs/buildssl.sh`

默认重新生成：

```sh
cd benchmark/certs
CERT_FILE=local/lima-local-cert.pem \
KEY_FILE=local/lima-local-key.pem \
bash ./buildssl.sh
```

显式指定 SAN/CN 的示例：

```sh
cd benchmark/certs
mkdir -p local
CERT_MODE=ca \
CERT_FILE=local/lima-local-cert.pem \
KEY_FILE=local/lima-local-key.pem \
CA_CERT_FILE=local/lima-local-ca.pem \
CA_KEY_FILE=local/lima-local-ca.key.pem \
SAN_DNS=localhost,example.test \
SAN_IPS=127.0.0.1,::1,192.0.2.10 \
CERT_CN=example.test \
bash ./buildssl.sh
```
