# 本地 Linux 双内核测试环境（Ubuntu 22.04 + 24.04）

本文档说明如何在本机用两台 Ubuntu VM 搭一个可重复使用的 `ws / wss / tls / ktls` 测试环境：

- `Ubuntu 22.04`：目标内核 `5.15.x`
- `Ubuntu 24.04`：目标内核 `6.1+`（当前通常是 `6.8.x`）

## 目标

- `ws / wss`：复用 `benchmark/ws-autobahn`
- `tls`：复用 `benchmark/tls-suite`
- `ktls`：复用 `cmd/ktlscheck`

当前维护的稳定结论入口见：

- `docs/test/linux-lab/WSS_KTLS_MATRIX.zh-CN.md`
- `docs/test/linux-lab/TLS_LIMA_REPORT.zh-CN.md`
- `docs/test/tls/TLS_EXTEND.zh-CN.md`（仅当需要更严格的 TLS-only 扩展检查）

TLS 场景矩阵执行脚本：

- `scripts/lima/run_tls_matrix.sh`

该脚本默认把这 3 个必跑槽位纳入每个 TLS 场景：

- `smoke`
- `testssl`
- `tlsanvil`

当前推荐的测试分层：

- 默认基线：
  - `ws / wss` 走 Autobahn
  - `tls` 走 `smoke / testssl / tlsanvil`
- 扩展专项：
  - `tlsfuzzer`
  - `bogo`

宿主机假设为 `macOS arm64`，使用 [Lima](https://lima-vm.io/) 启动本地 Ubuntu VM。

## 1. 安装 Lima

```sh
brew install lima
```

## 2. 一键准备 VM、证书和 Linux 二进制

```sh
cd <repo-root>
bash ./scripts/lima/setup_linux_test_lab.sh
```

该脚本会完成这些事情：

- 生成本地 WSS/TLS 用证书：`benchmark/certs/local/lima-local-cert.pem`
- 交叉编译 `linux/arm64` 二进制到 `benchmark/linux-lab/bin/`
- 创建两台 VM：
  - `ub2204-k515`
  - `ub2404-k61plus`
- 建立宿主机端口转发：
  - `ub2204-k515`: `32000 -> 30000`, `32001 -> 30001`
  - `ub2404-k61plus`: `34000 -> 30000`, `34001 -> 30001`

## 3. 验证内核和 kTLS

```sh
cd <repo-root>
bash ./scripts/lima/verify_linux_test_lab.sh
```

也可以单独检查：

```sh
limactl shell ub2204-k515 -- bash -lc 'uname -r'
limactl shell ub2404-k61plus -- bash -lc 'uname -r'
limactl shell ub2204-k515 -- bash -lc "cd <repo-root> && ./benchmark/linux-lab/bin/ktlscheck -bench=false"
limactl shell ub2404-k61plus -- bash -lc "cd <repo-root> && ./benchmark/linux-lab/bin/ktlscheck -bench=false"
```

## 4. 在 VM 内启动 WS / WSS 服务

### Ubuntu 22.04 / WS

```sh
limactl shell ub2204-k515 -- bash -lc '
cd <repo-root>
./benchmark/linux-lab/bin/connaxis_ws_server -addr :30000 -net tcp -log-level info
'
```

### Ubuntu 22.04 / WSS

```sh
limactl shell ub2204-k515 -- bash -lc '
cd <repo-root>
./benchmark/linux-lab/bin/connaxis_ws_server \
  -tls \
  -cert ./benchmark/certs/local/lima-local-cert.pem \
  -key ./benchmark/certs/local/lima-local-key.pem \
  -addr :30001 \
  -net tcp \
  -log-level info
'
```

如果要强制走 kTLS 路径，可追加：

```sh
-tls-engine ktls
```

需要对比 `6.1+` 内核时，把 VM 名替换成 `ub2404-k61plus` 即可。

## 5. 从宿主机跑现有测试

### WS

```sh
cd <repo-root>/benchmark/ws-autobahn
REPORTS_DIR=$PWD/reports/ws_local_jammy_$(date +%Y%m%d_%H%M%S) \
TARGET_URL=ws://127.0.0.1:32000 \
./run.sh
```

### WSS

```sh
cd <repo-root>/benchmark/ws-autobahn
REPORTS_DIR=$PWD/reports/wss_local_jammy_$(date +%Y%m%d_%H%M%S) \
TARGET_URL=wss://127.0.0.1:32001 \
./run.sh
```

### TLS Smoke / testssl

`run_tls_suite.sh` 已经支持把 `SMOKE_CONNECT_HOST` / `TESTSSL_ADD_HOST` 这类变量继续传给子脚本，所以本地端口转发场景可以直接使用。

在 macOS 上，`testssl.sh` 的 Docker 容器访问宿主机时，推荐把 `TARGET_HOST` 设为 `host.docker.internal`：

```sh
cd <repo-root>/benchmark/tls-suite
RUN_ID=local_jammy_tls_$(date +%Y%m%d_%H%M%S) \
TARGET_HOST=host.docker.internal \
SMOKE_CONNECT_HOST=127.0.0.1 \
SNI=localhost \
TLS_PORT=32001 \
ENABLE_WSS_UPGRADE_CHECK=1 \
RUN_TLSFUZZER=0 \
RUN_BOGO=0 \
RUN_TLSANVIL=0 \
bash ./run_tls_suite.sh
```

测试 `Ubuntu 24.04` 时，把端口改成 `34001`。

## 6. 停止 VM

```sh
limactl stop ub2204-k515
limactl stop ub2404-k61plus
```

删除 VM：

```sh
limactl delete ub2204-k515
limactl delete ub2404-k61plus
```
