# TLS 套件补充说明（中文）

当前维护的验证主入口是：

- `docs/test/linux-lab/README.zh-CN.md`
- `docs/test/linux-lab/TLS_LIMA_REPORT.zh-CN.md`

该目录保留为 `benchmark/tls-suite` 的补充说明，适用于你已经有一个运行中的 TLS 服务，只想单独执行 TLS suite，而不需要整套 Lima 双内核环境。

英文版：`README.md`

扩展检查说明：

- `docs/test/tls/TLS_EXTEND.zh-CN.md`

## 示例测试目标（请按环境修改）

- 连接地址：`127.0.0.1:30001`
- Host/SNI：`example.test`

## 前置条件

- 本地具备 `bash`、`openssl`、`docker`。
- 本地到 `127.0.0.1:30001` 网络可达。
- 目标服务已启动并监听 `30001` TLS 端口。

## 本地标准测试步骤

```sh
cd benchmark/tls-suite
cp targets.env.example targets.env
bash ./run_tls_suite.sh
```

该流程只使用本地脚本：

- `run_tls_smoke.sh`：openssl 握手 + 可选 WSS 升级校验
- `run_testssl.sh`：Docker 方式运行 `testssl.sh`

## 不落盘 `targets.env` 的一行命令

```sh
cd benchmark/tls-suite
TARGET_HOST=example.test \
SNI=example.test \
TLS_PORT=30001 \
SMOKE_CONNECT_HOST=127.0.0.1 \
TESTSSL_ADD_HOST=example.test:127.0.0.1 \
bash ./run_tls_suite.sh
```

## 结果输出位置

- 根目录：`benchmark/tls-suite/results/<RUN_ID>/`
- 汇总文件：`benchmark/tls-suite/results/<RUN_ID>/summary.txt`
- 握手结果：`benchmark/tls-suite/results/<RUN_ID>/smoke/handshake.txt`
- WSS 升级结果：`benchmark/tls-suite/results/<RUN_ID>/smoke/wss_upgrade.txt`
- testssl 报告：`benchmark/tls-suite/results/<RUN_ID>/testssl/testssl_report.txt`

## 结果判定

- `smoke=PASS`：TLS 握手成功（若启用 WSS 校验，也需升级成功）。
- `testssl=PASS|WARN|FAIL`：
  - `PASS`：`testssl` 返回码为 `0`。
  - `WARN(rc=N)`：返回非 `0`，但 `TESTSSL_STRICT=0`。
  - `FAIL`：`TESTSSL_STRICT=1` 且返回非 `0`。

## 默认矩阵

- `P0 阻塞项`
  - `smoke`
  - `testssl`
  - `WSS upgrade`
- `P1 强校验`
  - `tlsanvil`

说明：

- `P0/P1` 用来判断当前场景是否达到可交付基线。
- 默认 Lima matrix 只跑：`smoke / testssl / tlsanvil`
- `tlsfuzzer` 和 `bogo` 保留为可选扩展检查，不再属于默认 matrix

## 关键参数说明

- `TARGET_HOST`：suite/testssl 使用的目标主机名。
- `SNI`：openssl 握手时发送的 SNI。
- `TLS_PORT`：TLS 端口。
- `SMOKE_CONNECT_HOST`：openssl `-connect` 使用的真实地址（通常填 IP）。
- `TESTSSL_ADD_HOST`：给 Docker 加 `--add-host`，用于域名映射到指定 IP。
- `ENABLE_WSS_UPGRADE_CHECK`：`1` 或 `0`（默认 `1`）。
- `TESTSSL_STRICT`：`1` 或 `0`（默认 `0`）。
- `RUN_TLSFUZZER`：设为 `1` 启用 `tlsfuzzer` 阶段。
- `RUN_BOGO`：设为 `1` 启用 BoGo 阶段。
- `RUN_TLSANVIL`：设为 `1` 启用 TLS-Anvil 阶段。
- `TLSFUZZER_CMD`：`run_tlsfuzzer.sh` 实际执行的命令串。
- `BOGO_CMD`：`run_bogo.sh` 实际执行的命令串。
- `TLSANVIL_CMD`：`run_tlsanvil.sh` 实际执行的命令串。

## OCSP Stapling 与 Session Resumption（服务端配置）

这两项是服务端能力，不是扫描器参数。

如果你使用 `benchmark/ws-autobahn/connaxis_ws_server`，可用：

- `-ocsp-staple <path>`：加载 OCSP staple 响应（PEM/DER）到 `tls.Certificate.OCSPStaple`。
- `-disable-session-tickets`：显式关闭 session tickets。

默认行为：

- 不显式关闭时，session tickets 默认开启。
- 不配置 `-ocsp-staple` 时，OCSP stapling 默认关闭。

## tls-extend

`run_tls_suite.sh` 仍保留以下可选阶段：

- `run_tlsfuzzer.sh`
- `run_bogo.sh`
- `run_tlsanvil.sh`

这两个脚本本身是纯 shell 包装层，不依赖本机 Python；实际测试命令来自
`TLSFUZZER_CMD` / `BOGO_CMD` / `TLSANVIL_CMD`，并注入以下环境变量：

- `TARGET_HOST`
- `TLS_PORT`
- `SNI`
- `OUT_DIR`

如果命令为空，对应阶段会标记为 `SKIPPED`。
如果命令返回 `exit 3`，对应阶段会标记为 `N/A`。

`scripts/lima/run_tls_matrix.sh` 现在会默认自动接线：

- `tlsfuzzer`：本机 `tlsfuzzer` checkout + 本地 Python runner
- `bogo`：本机 `boringssl` runner + 本机 `cmd/bogo_shim`
- `tlsanvil`：本机 Docker `ghcr.io/tls-attacker/tlsanvil:latest`

当前默认 matrix 只汇总：

- `smoke`
- `testssl`
- `tlsanvil`

`tlsfuzzer` 和 `bogo` 如需启用，应当按 `tls-extend` 单独解读，不再和默认 matrix 混为一个口径。

示例：

```sh
cd benchmark/tls-suite
RUN_TLSFUZZER=1 \
TLSFUZZER_CMD='docker run --rm --network host -e TARGET_HOST -e TLS_PORT -e SNI myrepo/tlsfuzzer-runner:latest' \
bash ./run_tls_suite.sh
```

BoGo 示例（ssh 到远端 Linux，运行 BoringSSL runner + connaxis shim）：

```sh
cd benchmark/tls-suite
RUN_BOGO=1 \
BOGO_CMD='bash ./bogo/run_remote.sh' \
bash ./run_tls_suite.sh
```

TLS-Anvil 示例：

```sh
cd benchmark/tls-suite
RUN_TLSANVIL=1 \
TLSANVIL_CMD='docker run --rm -v "$OUT_DIR:/output/" ghcr.io/tls-attacker/tlsanvil:latest -zip -parallelHandshakes 1 -connectionTimeout 200 -strength 1 -identifier connaxis server -connect "${TARGET_HOST}:${TLS_PORT}"' \
bash ./run_tls_suite.sh
```
