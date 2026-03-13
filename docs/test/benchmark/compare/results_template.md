# Benchmark Results Template

## Environment

- Date:
- CPU:
- RAM:
- OS / Kernel:
- Go version:
- GOMAXPROCS:

---

## TCP Echo

| Framework | Payload | Conns | Duration | Throughput (msg/s) | p50 | p95 | p99 | CPU% | RSS(MB) | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| connaxis | 64B | 200 | 30s |  |  |  |  |  |  |  |
| gnet | 64B | 200 | 30s |  |  |  |  |  |  |  |
| tidwall | 64B | 200 | 30s |  |  |  |  |  |  |  |
| netpoll | 64B | 200 | 30s |  |  |  |  |  |  |  |

---

## HTTP

| Framework | Conns | Duration | Throughput (req/s) | p50 | p95 | p99 | CPU% | RSS(MB) | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| connaxis | 200 | 30s |  |  |  |  |  |  |  |
| gnet | 200 | 30s |  |  |  |  |  |  |  |
| tidwall | 200 | 30s |  |  |  |  |  |  |  |
| netpoll | 200 | 30s |  |  |  |  |  |  |  |

---

## WS Echo

| Framework | Payload | Conns | Duration | Throughput (msg/s) | p50 | p95 | p99 | CPU% | RSS(MB) | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| connaxis | 64B | 200 | 30s |  |  |  |  |  |  |  |
| gnet | 64B | 200 | 30s |  |  |  |  |  |  |  |
| tidwall | 64B | 200 | 30s |  |  |  |  |  |  |  |
| netpoll | 64B | 200 | 30s |  |  |  |  |  |  |  |

---

## TLS Echo

| Framework | Payload | Conns | Duration | Throughput (msg/s) | p50 | p95 | p99 | CPU% | RSS(MB) | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| connaxis | 64B | 200 | 30s |  |  |  |  |  |  |  |
| gnet | 64B | 200 | 30s |  |  |  |  |  |  |  |

---

## WSS Echo

| Framework | Payload | Conns | Duration | Throughput (msg/s) | p50 | p95 | p99 | CPU% | RSS(MB) | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| connaxis | 64B | 200 | 30s |  |  |  |  |  |  |  |
| gnet | 64B | 200 | 30s |  |  |  |  |  |  |  |
