# Ubuntu 22.04 / 24.04 WSS kTLS Matrix

本文档是当前维护的稳定入口，汇总本仓库在本地 Lima 双内核环境上的 `ws / wss / kTLS` 复现方法和结论。

## 环境基线

宿主机：

- `macOS arm64`
- VM 管理：`Lima`

Linux VM：

| VM | Ubuntu | 内核 | 宿主机端口 |
| --- | --- | --- | --- |
| `ub2204-k515` | `22.04` | `5.15.0-171-generic` | `32000/32001/32002 -> 30000/30001/30002` |
| `ub2404-k61plus` | `24.04` | `6.8.0-101-generic` | `34000/34001/34002 -> 30000/30001/30002` |

环境准备与验证：

- `scripts/lima/setup_linux_test_lab.sh`
- `scripts/lima/verify_linux_test_lab.sh`

## 被测矩阵

`ws`：

- `Ubuntu 22.04 / 5.15`
- `Ubuntu 24.04 / 6.8`

`wss + ktls`：

- `tls12-tx`
- `tls12-rxtx`
- `tls13-tx`
- `tls13-rxtx`

## 执行方法

1. 准备 Lima 双内核环境。
2. 在目标 VM 内启动 `benchmark/ws-autobahn/connaxis_ws_server`。
3. 在宿主机执行 `benchmark/ws-autobahn/run.sh`。
4. 仅接受 `517` case 全部落盘的 Autobahn 报告作为最终结论。

详细步骤见：

- `docs/test/linux-lab/README.zh-CN.md`

## 结论

`ws`：

- `Ubuntu 22.04 / 5.15`：`517 / 512 OK / 2 NON-STRICT / 3 INFORMATIONAL / 0 FAILED`
- `Ubuntu 24.04 / 6.8`：`517 / 512 OK / 2 NON-STRICT / 3 INFORMATIONAL / 0 FAILED`

`wss + ktls`：

- `Ubuntu 22.04 / 5.15`
  - `tls12-tx`：`517 / 512 OK / 2 NON-STRICT / 3 INFORMATIONAL / 0 FAILED`
  - `tls12-rxtx`：`517 / 512 OK / 2 NON-STRICT / 3 INFORMATIONAL / 0 FAILED`
  - `tls13-tx`：`517 / 512 OK / 2 NON-STRICT / 3 INFORMATIONAL / 0 FAILED`
  - `tls13-rxtx`：`517 / 512 OK / 2 NON-STRICT / 3 INFORMATIONAL / 0 FAILED`
- `Ubuntu 24.04 / 6.8`
  - `tls12-tx`：`517 / 512 OK / 2 NON-STRICT / 3 INFORMATIONAL / 0 FAILED`
  - `tls12-rxtx`：`517 / 512 OK / 2 NON-STRICT / 3 INFORMATIONAL / 0 FAILED`
  - `tls13-tx`：`517 / 512 OK / 2 NON-STRICT / 3 INFORMATIONAL / 0 FAILED`
  - `tls13-rxtx`：`517 / 512 OK / 2 NON-STRICT / 3 INFORMATIONAL / 0 FAILED`

固定非 `OK` case：

- `6.4.3`
- `6.4.4`
- `7.1.6`
- `7.13.1`
- `7.13.2`

## 说明

当前主结论是：

- `ws` 与 `wss + ktls` 在 `5.15` 和 `6.8` 两套内核上行为一致。
- `TLS 1.2 / 1.3` 与 `TX-only / RX+TX` 的 4 个 `ktls` 组合都已打通。
- 更细的执行命令、结果目录和历史 run id 保留在归档记录中，不再作为主入口。
