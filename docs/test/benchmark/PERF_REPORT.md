# Performance Report

## Scope

This report records the current formal bare-metal benchmark baseline for:

- `connaxis`
- `tidwall/evio`
- `panjf2000/gnet`
- `cloudwego/netpoll`

The run uses the shared `benchmark/compare` harness on a real Linux host. Lima is not used for performance conclusions.

## Environment

- Date: `2026-03-12`
- Host: dedicated bare-metal Linux benchmark host
- OS / Kernel: `Ubuntu 22.04`, `Linux 5.15.0-172-generic`
- Arch: `x86_64`
- CPU: `8 vCPU`
- Project path: `<repo>/benchmark/compare` on the benchmark host
- Run id: `server_compare_20260312_2230`

## Command

```sh
cd <repo>/benchmark/compare
RUN_ID=server_compare_20260312_2230 NET=tcp CONNS_LIST=5000 PAYLOADS_LIST=128,512 DURATION=30s INCLUDE_TLS=1 ./run_full.sh :5000
```

## Result Files

- `benchmark/compare/results/server_compare_20260312_2230/results_all.csv`
- `benchmark/compare/results/server_compare_20260312_2230/results_connaxis_5000.csv`
- `benchmark/compare/results/server_compare_20260312_2230/results_tidwall_5000.csv`
- `benchmark/compare/results/server_compare_20260312_2230/results_gnet_5000.csv`
- `benchmark/compare/results/server_compare_20260312_2230/results_netpoll_5000.csv`

## Interpretation Notes

- `connaxis` serves `tls/wss` natively.
- `tidwall/evio`, `gnet`, and `netpoll` use `benchmark/compare/tls_proxy` for `tls/wss`.
- Therefore, `tls/wss` numbers are useful as delivery data, but they are not a pure framework-only TLS comparison.
- This report is a single baseline run, not a multi-run median study.

## Results

### TCP

| framework | payload | throughput | p50 | p95 | p99 |
|---|---:|---:|---:|---:|---:|
| connaxis | 128 | 278970.22 | 15ms | 40.6ms | 57.7ms |
| connaxis | 512 | 259073.60 | 15.8ms | 45.2ms | 65ms |
| tidwall | 128 | 317678.96 | 100µs | 1.9ms | 8.9ms |
| tidwall | 512 | 289660.40 | 100µs | 1.8ms | 9.9ms |
| gnet | 128 | 281777.09 | 15.1ms | 39.6ms | 56.5ms |
| gnet | 512 | 266486.21 | 15.9ms | 43.4ms | 65.1ms |
| netpoll | 128 | 107813.52 | 40.3ms | 53.7ms | 64.7ms |
| netpoll | 512 | 114168.27 | 35.1ms | 48.8ms | 56.3ms |

### HTTP

| framework | throughput | p50 | p95 | p99 |
|---|---:|---:|---:|---:|
| connaxis | 272413.99 | 15.5ms | 41.1ms | 58.6ms |
| tidwall | 310930.34 | 200µs | 2.3ms | 9.6ms |
| gnet | 275295.50 | 15.8ms | 40ms | 57.7ms |
| netpoll | 82459.66 | 33.8ms | 51.3ms | 166.2ms |

### WS

| framework | payload | throughput | p50 | p95 | p99 |
|---|---:|---:|---:|---:|---:|
| connaxis | 128 | 248746.50 | 16.4ms | 46ms | 72.2ms |
| connaxis | 512 | 248526.46 | 16.8ms | 45.2ms | 66.3ms |
| tidwall | 128 | 266706.13 | 200µs | 2.6ms | 9.9ms |
| tidwall | 512 | 255724.94 | 100µs | 2ms | 10.7ms |
| gnet | 128 | 246391.59 | 16.9ms | 45.5ms | 69.5ms |
| gnet | 512 | 248392.68 | 16.7ms | 45.6ms | 68.8ms |
| netpoll | 128 | 85445.37 | 28ms | 50.7ms | 151.3ms |
| netpoll | 512 | 74373.47 | 43.5ms | 57.4ms | 199.9ms |

### TLS

| framework | payload | throughput | p50 | p95 | p99 |
|---|---:|---:|---:|---:|---:|
| connaxis | 128 | 259196.45 | 700µs | 8.2ms | 17ms |
| connaxis | 512 | 242613.18 | 300µs | 5.9ms | 13.9ms |
| tidwall | 128 | 93122.24 | 4.7ms | 15.1ms | 24.7ms |
| tidwall | 512 | 77314.22 | 4.6ms | 20.5ms | 32.8ms |
| gnet | 128 | 79894.40 | 57.3ms | 75.4ms | 102.1ms |
| gnet | 512 | 78747.44 | 57.8ms | 83.2ms | 107.9ms |
| netpoll | 128 | 72170.80 | 59.3ms | 108.3ms | 131.7ms |
| netpoll | 512 | 70933.54 | 60.8ms | 110.8ms | 132.3ms |

### WSS

| framework | payload | throughput | p50 | p95 | p99 |
|---|---:|---:|---:|---:|---:|
| connaxis | 128 | 230194.40 | 400µs | 6.3ms | 16.2ms |
| connaxis | 512 | 236158.18 | 700µs | 8.5ms | 20.2ms |
| tidwall | 128 | 79017.11 | 2.7ms | 16ms | 27.3ms |
| tidwall | 512 | 80891.42 | 6.2ms | 21.2ms | 33.7ms |
| gnet | 128 | 80431.87 | 54.2ms | 74.8ms | 100.6ms |
| gnet | 512 | 78646.13 | 58.4ms | 77.1ms | 107ms |
| netpoll | 128 | 37167.55 | 72.4ms | 199.9ms | 199.9ms |
| netpoll | 512 | 36913.16 | 64.3ms | 199.9ms | 199.9ms |

## Summary

- `connaxis` is competitive on `tcp/http/ws` and clearly ahead on this harness's `tls/wss` runs.
- `tidwall/evio` leads the plain `tcp/http/ws` cases in this single baseline run.
- `gnet` stays close to `connaxis` on plain `tcp/http/ws`, but its proxied `tls/wss` results are materially lower.
- `netpoll` is behind the other three frameworks across all measured categories in this run.
