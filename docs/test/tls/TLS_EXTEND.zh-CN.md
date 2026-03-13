# TLS 扩展检查（tls-extend）

本文档单独说明不属于默认 Lima TLS matrix 的扩展检查：

- `tlsfuzzer`
- `bogo`

默认 matrix 仍只包含：

- `smoke`
- `testssl`
- `tlsanvil`

主入口：

- `docs/test/linux-lab/README.zh-CN.md`
- `docs/test/linux-lab/TLS_LIMA_REPORT.zh-CN.md`
- `docs/test/tls/README.zh-CN.md`

## 1. 定位

`tls-extend` 用于补充默认 matrix 覆盖不到的严格负面测试和实现对比测试，不作为当前日常回归的默认门槛。

建议解读口径：

- `P0`：`smoke` / `testssl` / `WSS upgrade`
- `P1`：`tlsanvil`
- `P2`：`tls-extend`（`tlsfuzzer` / `bogo`）

## 2. tlsfuzzer

用途：

- 覆盖 TLS 负面输入
- 观察 alert 语义
- 检查 TLS 1.3 边界行为

当前建议：

- 保留为专项扩展检查
- 不作为默认 matrix 的阻塞项
- 如果需要与历史 `20260224` TLS-only 报告对比，应单独跑，不要和默认 Lima matrix 混合解读

示例：

```sh
cd <repo-root>/benchmark/tls-suite
RUN_TLSFUZZER=1 \
RUN_BOGO=0 \
RUN_TLSANVIL=0 \
TLSFUZZER_CMD='python3 ./tlsfuzzer/run_local.py --host "$TARGET_HOST" --port "$TLS_PORT" --out-dir "$OUT_DIR" --tlsfuzzer-root /private/tmp/tlsfuzzer-src --script-glob "test_tls13_*.py"' \
bash ./run_tls_suite.sh
```

其中 `--tlsfuzzer-root` 应替换成你本机实际的 `tlsfuzzer` checkout 路径。

## 3. bogo

用途：

- 使用 BoringSSL runner 驱动 shim
- 对 TLS 栈做更结构化的兼容性验证

当前建议：

- 保留为专项扩展检查
- 不作为默认 matrix 的阻塞项
- `ktls + bogo` 应优先在 Linux 环境内执行，不建议直接用当前 macOS 宿主默认拓扑解读 `ktls` 结果

示例：

```sh
cd <repo-root>/benchmark/tls-suite
RUN_TLSFUZZER=0 \
RUN_BOGO=1 \
RUN_TLSANVIL=0 \
BOGO_CMD='bash ./bogo/run_remote.sh' \
bash ./run_tls_suite.sh
```

## 4. 何时需要跑 tls-extend

以下场景建议额外跑：

- 对比 `20260224` TLS-only 历史口径
- 需要分析 `atls` 与 `ktls` 的严格负面行为差异
- 需要判断某次 TLS 握手修复是否影响 alert / close 语义

以下场景不必默认跑：

- 日常 `atls/ktls` 6 场景基线回归
- `22.04 / 24.04` 双内核稳定性复验
- `WSS + kTLS` 主路径交付前检查
