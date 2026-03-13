# Cross-Framework Benchmarks

This folder provides **comparable servers and a unified load generator** for:
- `tidwall/evio`
- `panjf2000/gnet`
- `cloudwego/netpoll`
- `connaxis`

## Recommended Workflow (One-Command)

```sh
cd benchmark/compare
NET=tcp CONNS_LIST=5000 PAYLOADS_LIST=128,512 DURATION=30s INCLUDE_TLS=1 ./run_full.sh :5000
```

To run additional connection scales, override `CONNS_LIST` as needed (for example: `50,100,1000,5000`).

Optional soak:

```sh
SOAK=1 DURATION_SOAK=30m CONNS_SOAK=50 PAYLOAD_SOAK=64 ./run_full.sh :5000
```

Optional backpressure:

```sh
INCLUDE_BP=1 BP_READ_DELAY=5ms ./run_full.sh :5000
```

Results are written to `benchmark/compare/results/<YYYYMMDD_HHMMSS>/` by default.
Override with `RESULTS_DIR=...` or `RUN_ID=...`.

### Combined (Base + Backpressure + Soak)

```sh
NET=tcp CONNS_LIST=5000 DURATION=60s INCLUDE_TLS=1 \
BP_READ_DELAY=5ms SOAK=1 DURATION_SOAK=5m CONNS_SOAK=5000 PAYLOAD_SOAK=512 \
./run_bundle.sh :5000
```

This will produce a combined report in `benchmark/compare/results/<RUN_ID>_combined/`.

## Build (Manual)

From this folder:

```sh
cd benchmark/compare

go mod tidy
```

Build servers:

```sh
mkdir -p bin
go build -o bin/connaxis_server ./connaxis_server
go build -o bin/tidwall_server ./tidwall_server
go build -o bin/gnet_server ./gnet_server

# netpoll requires build tag
go build -tags netpoll -o bin/netpoll_server ./netpoll_server
```

Build client:

```sh
go build -o bin/client ./client
```

## Script Roles (Manual)

- `run.sh`: one case (framework + mode + payload + conns)
- `run_matrix.sh`: one framework across payloads/modes
- `run_all.sh`: all frameworks across `CONNS_LIST`
- `run_full.sh`: one-command wrapper (run_all + merge + report + charts [+ optional soak])
- `run_bundle.sh`: combined base + backpressure + optional soak workflow
- `run_soak.sh`: long TCP soak for all frameworks
- `merge_results.sh`: merge `results_*.csv` into `results_all.csv`
- `merge_runs.sh`: merge multiple run directories into one combined result set
- `generate_report.sh`: write `docs/test/benchmark/PERF_REPORT.md` from CSV + env
- `generate_charts.sh`: write `docs/test/benchmark/PERF_REPORT_CHARTS.md`

## Environment Variables (Most Used)

- `CONNS_LIST=5000` (default; used by `run_all.sh`/`run_full.sh`)
- `DURATION=30s` (per case)
- `PAYLOADS_LIST=128,512` (default payload set)
- `INCLUDE_TLS=1` (enable TLS/WSS for non-connaxis via proxy)
- `INCLUDE_BP=1` + `BP_READ_DELAY=5ms` (TCP backpressure scenario)
- `LOOPS=<n>` (force event-loop count for connaxis/tidwall/gnet)
- `RESULTS_DIR=...` or `RUN_ID=...` (output location)
- `GENERATE_REPORTS=0` (skip report/charts in `run_full.sh`, used by `run_bundle.sh`)

## Benchmark Environment

Record these for each run (server + client):

- OS / kernel: `uname -a`
- CPU model / cores: `lscpu | egrep 'Model name|CPU\\(s\\)'`
- RAM: `free -h`
- NIC / MTU: `ip link show`
- Go version: `go version`

Network sysctl (document any changes):

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

## Stats Collection

```sh
# Start server in one shell, record PID
./collect_stats.sh <pid> stats.csv 1
```

## Summarize Results

```sh
./summarize_results.sh connaxis 5000
```

## CPU/RSS Summary

```sh
./summarize_stats.sh results/connaxis_tcp_128_5000_cpu_rss.csv
```

## Environment & Report

```sh
./collect_env.sh results/env.json
./merge_results.sh results
./generate_report.sh
./generate_charts.sh results ../../docs/test/benchmark/PERF_REPORT_CHARTS.md
```

## Run Examples

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

- `connaxis_server` supports TLS/WSS directly.
- `gnet_server`, `tidwall_server`, and `netpoll_server` use a TLS proxy in this harness.
  - Set `INCLUDE_TLS=1` to include TLS/WSS for non-connaxis frameworks in matrix runs.

```sh
./connaxis_server -mode tls -addr :5000 -cert ../certs/cert.pem -key ../certs/key.pem
./client -mode tls -addr 127.0.0.1:5000 -c 200 -payload 64 -d 30s
```

---

## Notes

- All servers implement the same minimal protocol handling to reduce bias.
- TCP/TLS uses fixed-size payload echo (no framing).
- HTTP parsing only checks for `\r\n\r\n` and returns a fixed response.
- WS supports single-frame, no fragmentation, small payloads.
- Use `LOOPS=<n>` to force event-loop count for connaxis/tidwall/gnet. netpoll uses a single event loop in this harness, but `GOMAXPROCS` is still set for parity.
- Use bare-metal Linux hosts for formal benchmark conclusions. The `linux-lab`/Lima environments are reserved for function validation, not final performance claims.

## Official Baseline

Current formal baseline:

- Host: dedicated bare-metal Linux benchmark host
- OS / Kernel: `Ubuntu 22.04`, `Linux 5.15.0-172-generic`
- Arch: `x86_64`
- Project path: `<repo>/benchmark/compare` on the benchmark host
- Run id: `server_compare_20260312_2230`

Command:

```sh
cd <repo>/benchmark/compare
RUN_ID=server_compare_20260312_2230 NET=tcp CONNS_LIST=5000 PAYLOADS_LIST=128,512 DURATION=30s INCLUDE_TLS=1 ./run_full.sh :5000
```

Result set:

- `benchmark/compare/results/server_compare_20260312_2230/results_all.csv`
- `docs/test/benchmark/PERF_REPORT.md`

See `design/performance_methodology.en.md` for methodology and reporting format.
