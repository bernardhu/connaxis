# TLS Suite Supplement (English)

Primary maintained validation entry:

- `docs/test/linux-lab/README.zh-CN.md`
- `docs/test/linux-lab/TLS_LIMA_REPORT.zh-CN.md`

This directory is kept as a supplement for standalone `benchmark/tls-suite` usage. Use it when you already have a running TLS target and only want the TLS suite semantics, without the full Lima dual-kernel setup.

Chinese version: `README.zh-CN.md`

Extended checks:

- `TLS_EXTEND.zh-CN.md`

## Example Target (Edit for Your Environment)

- Connect address: `127.0.0.1:30001`
- Host/SNI: `example.test`

## Prerequisites

- Local machine has `bash`, `openssl`, `docker`.
- Network path from local machine to `127.0.0.1:30001`.
- Server is already running and serving TLS on port `30001`.

## Standard Local Test Steps

```sh
cd benchmark/tls-suite
cp targets.env.example targets.env
bash ./run_tls_suite.sh
```

This uses local scripts only:

- `run_tls_smoke.sh` (openssl handshake + optional WSS upgrade check)
- `run_testssl.sh` (Docker `testssl.sh`)

## One-Line Run (without `targets.env`)

```sh
cd benchmark/tls-suite
TARGET_HOST=example.test \
SNI=example.test \
TLS_PORT=30001 \
SMOKE_CONNECT_HOST=127.0.0.1 \
TESTSSL_ADD_HOST=example.test:127.0.0.1 \
bash ./run_tls_suite.sh
```

## Output Location

- Root: `benchmark/tls-suite/results/<RUN_ID>/`
- Summary: `benchmark/tls-suite/results/<RUN_ID>/summary.txt`
- Smoke handshake: `benchmark/tls-suite/results/<RUN_ID>/smoke/handshake.txt`
- Smoke WSS upgrade: `benchmark/tls-suite/results/<RUN_ID>/smoke/wss_upgrade.txt`
- testssl report: `benchmark/tls-suite/results/<RUN_ID>/testssl/testssl_report.txt`

## Result Interpretation

- `smoke=PASS`: TLS handshake succeeded (and WSS upgrade succeeded if enabled).
- `testssl=PASS|WARN|FAIL`:
  - `PASS`: `testssl` exit code is `0`.
  - `WARN(rc=N)`: non-zero exit code but `TESTSSL_STRICT=0`.
  - `FAIL`: strict mode enabled (`TESTSSL_STRICT=1`) and non-zero exit.

## Default Matrix

Interpret the default TLS matrix in two tiers:

- `P0 blocking`
  - `smoke`
  - `testssl`
  - `WSS upgrade`
- `P1 strong validation`
  - `tlsanvil`

Notes:

- `P0/P1` determine whether a scenario reaches the current delivery baseline.
- The default Lima matrix runs only: `smoke / testssl / tlsanvil`.
- `tlsfuzzer` and `bogo` remain available as optional extended checks and are no longer part of the default matrix.

## Important Variables

- `TARGET_HOST`: host used by suite and testssl target.
- `SNI`: SNI sent by openssl smoke checks.
- `TLS_PORT`: TLS port.
- `SMOKE_CONNECT_HOST`: raw connect IP/host for openssl `-connect`.
- `TESTSSL_ADD_HOST`: Docker `--add-host` mapping for domain-to-IP routing.
- `ENABLE_WSS_UPGRADE_CHECK`: `1` or `0` (default `1`).
- `TESTSSL_STRICT`: `1` or `0` (default `0`).
- `RUN_TLSFUZZER`: `1` to enable `tlsfuzzer` stage.
- `RUN_BOGO`: `1` to enable BoGo stage.
- `RUN_TLSANVIL`: `1` to enable TLS-Anvil stage.
- `TLSFUZZER_CMD`: command string executed by `run_tlsfuzzer.sh`.
- `BOGO_CMD`: command string executed by `run_bogo.sh`.
- `TLSANVIL_CMD`: command string executed by `run_tlsanvil.sh`.

## OCSP Stapling and Session Resumption (Server Side)

These are server settings, not scanner settings.

If you run `benchmark/ws-autobahn/connaxis_ws_server`, you can use:

- `-ocsp-staple <path>`: load OCSP staple response (PEM/DER) into `tls.Certificate.OCSPStaple`.
- `-disable-session-tickets`: disable session tickets explicitly.

Default behavior:

- session tickets are enabled unless explicitly disabled.
- OCSP stapling is off unless `-ocsp-staple` is provided.

## tls-extend

`run_tls_suite.sh` still supports:

- `run_tlsfuzzer.sh`
- `run_bogo.sh`
- `run_tlsanvil.sh`

These scripts are shell-only wrappers and do not require Python in the wrapper itself.
They execute commands from `TLSFUZZER_CMD` / `BOGO_CMD` / `TLSANVIL_CMD`, and pass:

- `TARGET_HOST`
- `TLS_PORT`
- `SNI`
- `OUT_DIR`

If command is empty, stage status is `SKIPPED`.
If command exits with code `3`, stage status is `N/A`.

When explicitly enabled, `scripts/lima/run_tls_matrix.sh` can auto-wire local runners for:

- `tlsfuzzer`: local checkout + local Python runner
- `bogo`: local BoringSSL runner + local `cmd/bogo_shim`
- `tlsanvil`: local Docker `ghcr.io/tls-attacker/tlsanvil:latest`

The default matrix now summarizes only:

- `smoke`
- `testssl`
- `tlsanvil`

Use `tlsfuzzer` and `bogo` as `tls-extend` targeted checks when needed. Do not interpret them as part of the default matrix baseline.

Example:

```sh
cd benchmark/tls-suite
RUN_TLSFUZZER=1 \
TLSFUZZER_CMD='docker run --rm --network host -e TARGET_HOST -e TLS_PORT -e SNI myrepo/tlsfuzzer-runner:latest' \
bash ./run_tls_suite.sh
```

BoGo example (remote BoringSSL runner + connaxis shim via ssh):

```sh
cd benchmark/tls-suite
RUN_BOGO=1 \
BOGO_CMD='bash ./bogo/run_remote.sh' \
bash ./run_tls_suite.sh
```

TLS-Anvil example:

```sh
cd benchmark/tls-suite
RUN_TLSANVIL=1 \
TLSANVIL_CMD='docker run --rm -v "$OUT_DIR:/output/" ghcr.io/tls-attacker/tlsanvil:latest -zip -parallelHandshakes 1 -connectionTimeout 200 -strength 1 -identifier connaxis server -connect "${TARGET_HOST}:${TLS_PORT}"' \
bash ./run_tls_suite.sh
```
