# Performance Report Template

**Project:** connaxis (comparison with tidwall/evio, gnet, netpoll)

## 1. Summary

- Key takeaways (3–5 bullets)
- Overall winner per scenario (if applicable)
- Notable regressions / anomalies

## 2. Environment

- Date:
- CPU:
- RAM:
- OS / Kernel:
- Go version:
- GOMAXPROCS:
- TLS config (if applicable):
- Notes (CPU pinning, frequency scaling, etc.)

## 3. Methodology

- Harness: `benchmark/compare`
- Scenarios: TCP / HTTP / WS / TLS / WSS
- Payload sizes:
- Warm-up duration:
- Measurement duration:
- Repetitions:
- Client machine (if separate):

## 4. Results (Tables)

### 4.1 TCP Echo

| Framework | Payload | Conns | Duration | Throughput (msg/s) | p50 | p95 | p99 | CPU% | RSS(MB) | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| connaxis |  |  |  |  |  |  |  |  |  |  |
| gnet |  |  |  |  |  |  |  |  |  |  |
| tidwall |  |  |  |  |  |  |  |  |  |  |
| netpoll |  |  |  |  |  |  |  |  |  |  |

### 4.1b TCP Echo (Backpressure)

| Framework | Payload | Conns | Duration | Throughput (msg/s) | p50 | p95 | p99 | CPU% | RSS(MB) | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| connaxis |  |  |  |  |  |  |  |  |  |  |
| gnet |  |  |  |  |  |  |  |  |  |  |
| tidwall |  |  |  |  |  |  |  |  |  |  |
| netpoll |  |  |  |  |  |  |  |  |  |  |

### 4.2 HTTP

| Framework | Conns | Duration | Throughput (req/s) | p50 | p95 | p99 | CPU% | RSS(MB) | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| connaxis |  |  |  |  |  |  |  |  |  |
| gnet |  |  |  |  |  |  |  |  |  |
| tidwall |  |  |  |  |  |  |  |  |  |
| netpoll |  |  |  |  |  |  |  |  |  |

### 4.3 WS Echo

| Framework | Payload | Conns | Duration | Throughput (msg/s) | p50 | p95 | p99 | CPU% | RSS(MB) | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| connaxis |  |  |  |  |  |  |  |  |  |  |
| gnet |  |  |  |  |  |  |  |  |  |  |
| tidwall |  |  |  |  |  |  |  |  |  |  |
| netpoll |  |  |  |  |  |  |  |  |  |  |

### 4.4 TLS Echo

| Framework | Payload | Conns | Duration | Throughput (msg/s) | p50 | p95 | p99 | CPU% | RSS(MB) | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| connaxis |  |  |  |  |  |  |  |  |  |  |
| gnet |  |  |  |  |  |  |  |  |  |  |

### 4.5 WSS Echo

| Framework | Payload | Conns | Duration | Throughput (msg/s) | p50 | p95 | p99 | CPU% | RSS(MB) | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| connaxis |  |  |  |  |  |  |  |  |  |  |
| gnet |  |  |  |  |  |  |  |  |  |  |

## 5. Interpretation

- Explain wins/losses per scenario
- Any anomalies (e.g. CPU spikes, GC pressure)
- Notes on TLS/WSS fairness (same TLS stack?)
- Echo-path bias: gnet writes responses directly within `OnTraffic` (event-loop), which shortens the hot path for simple echo and can improve throughput/latency in this synthetic workload. connaxis typically queues writes after parsing, which adds a small path length in echo-only scenarios but can offer better behavior under backpressure and busy FDs.
- Consider adding a “busy FD / backpressure” scenario (e.g., client read throttling or reduced send buffer) to validate behavior under real-world congestion.

## 6. Appendix

- Raw logs / result CSV paths
- Command lines used
