# Lima 双内核 TLS Matrix

本文档是当前维护的稳定入口，汇总本仓库在本地 Lima 双内核环境上的默认 TLS matrix。

## 默认口径

默认 TLS matrix 固定为 6 个场景：

- `atls-tls12`
- `atls-tls13`
- `ktls-tls12-tx`
- `ktls-tls12-rxtx`
- `ktls-tls13-tx`
- `ktls-tls13-rxtx`

每个场景固定 3 个必跑槽位：

- `smoke`
- `testssl`
- `tlsanvil`

扩展检查：

- `tlsfuzzer`
- `bogo`

不属于默认 matrix，单独归到 `tls-extend`。

## 环境基线

| VM | Ubuntu | 内核 | 宿主机 TLS 端口 |
| --- | --- | --- | --- |
| `ub2204-k515` | `22.04` | `5.15.0-171-generic` | `127.0.0.1:32001` |
| `ub2404-k61plus` | `24.04` | `6.8.0-101-generic` | `127.0.0.1:34001` |

矩阵执行脚本：

- `scripts/lima/run_tls_matrix.sh`

## 结论

### Ubuntu 24.04 / 6.8

- `atls-tls12 = PASS / PASS / PASS`
- `atls-tls13 = PASS / PASS / PASS`
- `ktls-tls12-tx = PASS / PASS / PASS`
- `ktls-tls12-rxtx = PASS / PASS / PASS`
- `ktls-tls13-tx = PASS / PASS / PASS`
- `ktls-tls13-rxtx = PASS / PASS / PASS`

### Ubuntu 22.04 / 5.15

- `atls-tls12 = PASS / PASS / PASS`
- `atls-tls13 = PASS / PASS / PASS`
- `ktls-tls12-tx = PASS / PASS / PASS`
- `ktls-tls12-rxtx = PASS / PASS / PASS`
- `ktls-tls13-tx = PASS / WARN(rc=1) / PASS`
- `ktls-tls13-rxtx = PASS / PASS / PASS`

## 已知说明

`Ubuntu 22.04 + ktls-tls13-tx` 的 `WARN(rc=1)` 来自 `testssl.sh` 返回非 0，不是主功能失败：

- `smoke=PASS`
- `tlsanvil=PASS`
- `testssl` 的告警来自自签证书评分和退出码，不代表握手或协议链路失败

## 使用方式

环境准备：

- `scripts/lima/setup_linux_test_lab.sh`
- `scripts/lima/verify_linux_test_lab.sh`

执行：

```sh
cd <repo-root>
VM_NAME=ub2204-k515 bash ./scripts/lima/run_tls_matrix.sh
VM_NAME=ub2404-k61plus bash ./scripts/lima/run_tls_matrix.sh
```

更多执行命令、结果目录和历史 run id 保留在归档记录中，不再作为主入口。
