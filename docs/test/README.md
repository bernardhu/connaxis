# Benchmark and Validation

This document tracks the maintained validation entry points in this repository.

## Primary Entry

Use the Lima-based dual-kernel workflow as the maintained baseline:

- `docs/test/linux-lab/README.zh-CN.md`
- `docs/test/linux-lab/WSS_KTLS_MATRIX.zh-CN.md`
- `docs/test/linux-lab/TLS_LIMA_REPORT.zh-CN.md`

## Test Tiers

- Default matrix:
  - `ws / wss` via Autobahn
  - `tls` via `smoke / testssl / tlsanvil`
  - dual kernels: Ubuntu 22.04 / 5.15 and Ubuntu 24.04 / 6.8
- Extended TLS-only checks:
  - `docs/test/tls/TLS_EXTEND.zh-CN.md`
- Repo-local unit/integration coverage notes:
  - `docs/test/ws/WS_TLS_TEST_SUITE.md`

## Supporting Docs

- TLS matrix notes: `docs/test/tls/TLS_TEST_MATRIX.md`
- Benchmark methodology and reports: `docs/test/benchmark/`

## Cross-Framework Performance (Primary)

Use `benchmark/compare` for TCP/HTTP/WS/TLS/WSS comparisons.

```sh
cd benchmark/compare
NET=tcp4 CONNS_LIST=5000 PAYLOADS_LIST=128,512 DURATION=30s INCLUDE_TLS=1 ./run_full.sh :5000
```

To run additional connection scales, override `CONNS_LIST` (for example: `50,100,1000,5000`).

Optional bundle run (base + backpressure + soak):

```sh
cd benchmark/compare
NET=tcp4 CONNS_LIST=5000 DURATION=60s INCLUDE_TLS=1 \
BP_READ_DELAY=5ms SOAK=1 DURATION_SOAK=5m CONNS_SOAK=5000 PAYLOAD_SOAK=512 \
./run_bundle.sh :5000
```

Generated report targets:

- `docs/test/benchmark/PERF_REPORT.md`
- `docs/test/benchmark/PERF_REPORT_CHARTS.md`

## WS/WSS Conformance (Autobahn)

Do not use the older ad hoc Linux-host instructions here as the current baseline.

Use the Lima-based dual-kernel workflow instead:

- environment setup: `docs/test/linux-lab/README.zh-CN.md`
- reproducible WS/WSS/kTLS matrix and conclusions: `docs/test/linux-lab/WSS_KTLS_MATRIX.zh-CN.md`

## TLS Validation (Standalone Suite)

```sh
cd benchmark/tls-suite
cp targets.env.example targets.env
bash ./run_tls_suite.sh
```

For local dual-kernel execution and port-forward layout, follow:

- `docs/test/linux-lab/README.zh-CN.md`

For Linux kTLS capability checks:

```sh
go run ./cmd/ktlscheck -bench=false
```

## Benchmark Certificate Material

- Generated local certificate path: `benchmark/certs/local/lima-local-cert.pem`
- Generated local private key path: `benchmark/certs/local/lima-local-key.pem`
- Regenerate script: `benchmark/certs/buildssl.sh`

Default regenerate:

```sh
cd benchmark/certs
CERT_FILE=local/lima-local-cert.pem \
KEY_FILE=local/lima-local-key.pem \
bash ./buildssl.sh
```

Regenerate with explicit SAN/CN (example):

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
