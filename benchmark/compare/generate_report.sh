#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${RESULTS_DIR:-$ROOT_DIR/results}"
REPORT="$ROOT_DIR/../../docs/test/benchmark/PERF_REPORT.md"
ENV_FILE="$RESULTS_DIR/env.json"
ALL_CSV="$RESULTS_DIR/results_all.csv"

resolve_python() {
  if [[ -n "${PYTHON_BIN:-}" ]]; then
    if command -v "$PYTHON_BIN" >/dev/null 2>&1; then
      echo "$PYTHON_BIN"
      return 0
    fi
    echo "PYTHON_BIN '$PYTHON_BIN' not found in PATH" >&2
    exit 1
  fi
  if command -v python >/dev/null 2>&1; then
    echo "python"
    return 0
  fi
  if command -v python3 >/dev/null 2>&1; then
    echo "python3"
    return 0
  fi
  echo "No python interpreter found (checked: python, python3). Set PYTHON_BIN." >&2
  exit 1
}

PYTHON_BIN="$(resolve_python)"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env.json not found, run collect_env.sh or run_matrix.sh" >&2
  exit 1
fi

cpu_model=$(jq -r '.cpu_model' "$ENV_FILE")
cores=$(jq -r '.cores' "$ENV_FILE")
mem_bytes=$(jq -r '.mem_bytes' "$ENV_FILE")
env_date=$(jq -r '.date' "$ENV_FILE")
ram_gb=$(MEM_BYTES="$mem_bytes" "$PYTHON_BIN" - <<'PY'
import os

mem = os.environ.get("MEM_BYTES", "")
try:
    mb = int(mem) // (1024 * 1024)
    print(round(mb / 1024, 1))
except Exception:
    print("unknown")
PY
)
os_kernel=$(jq -r '.os_kernel' "$ENV_FILE")
go_version=$(jq -r '.go_version' "$ENV_FILE")
gomaxprocs=$(jq -r '.gomaxprocs' "$ENV_FILE")
sysctl_ostype=$(jq -r '.sysctl_ostype' "$ENV_FILE")
sysctl_osrelease=$(jq -r '.sysctl_osrelease' "$ENV_FILE")
sysctl_version=$(jq -r '.sysctl_version' "$ENV_FILE")
sysctl_pagesize=$(jq -r '.sysctl_pagesize' "$ENV_FILE")
sysctl_cachelinesize=$(jq -r '.sysctl_cachelinesize' "$ENV_FILE")
sysctl_l1dcachesize=$(jq -r '.sysctl_l1dcachesize' "$ENV_FILE")
sysctl_l2cachesize=$(jq -r '.sysctl_l2cachesize' "$ENV_FILE")
sysctl_l3cachesize=$(jq -r '.sysctl_l3cachesize' "$ENV_FILE")
net_core_somaxconn=$(jq -r '.net_core_somaxconn' "$ENV_FILE")
net_core_netdev_max_backlog=$(jq -r '.net_core_netdev_max_backlog' "$ENV_FILE")
net_ipv4_tcp_max_syn_backlog=$(jq -r '.net_ipv4_tcp_max_syn_backlog' "$ENV_FILE")
net_ipv4_tcp_fin_timeout=$(jq -r '.net_ipv4_tcp_fin_timeout' "$ENV_FILE")
net_ipv4_tcp_tw_reuse=$(jq -r '.net_ipv4_tcp_tw_reuse' "$ENV_FILE")
net_ipv4_tcp_timestamps=$(jq -r '.net_ipv4_tcp_timestamps' "$ENV_FILE")
net_ipv4_tcp_keepalive_time=$(jq -r '.net_ipv4_tcp_keepalive_time' "$ENV_FILE")
net_ipv4_tcp_keepalive_intvl=$(jq -r '.net_ipv4_tcp_keepalive_intvl' "$ENV_FILE")
net_ipv4_tcp_keepalive_probes=$(jq -r '.net_ipv4_tcp_keepalive_probes' "$ENV_FILE")
net_sendbuf=$(jq -r '.net_sendbuf' "$ENV_FILE")
net_recvbuf=$(jq -r '.net_recvbuf' "$ENV_FILE")

cat > "$REPORT" <<EOF2
# Performance Report (Generated)

## Environment
- Date: ${env_date}
- CPU: ${cpu_model} (${cores} cores)
- RAM: ${ram_gb} GB
- OS / Kernel: ${os_kernel}
- Go version: ${go_version}
- GOMAXPROCS: ${gomaxprocs}

| sysctl | value |
|---|---|
| kern.ostype | ${sysctl_ostype} |
| kern.osrelease | ${sysctl_osrelease} |
| kern.version | ${sysctl_version} |
| hw.pagesize | ${sysctl_pagesize} |
| hw.cachelinesize | ${sysctl_cachelinesize} |
| hw.l1dcachesize | ${sysctl_l1dcachesize} |
| hw.l2cachesize | ${sysctl_l2cachesize} |
| hw.l3cachesize | ${sysctl_l3cachesize} |
| net.sendbuf | ${net_sendbuf} |
| net.recvbuf | ${net_recvbuf} |
| net.core.somaxconn | ${net_core_somaxconn} |
| net.core.netdev_max_backlog | ${net_core_netdev_max_backlog} |
| net.ipv4.tcp_max_syn_backlog | ${net_ipv4_tcp_max_syn_backlog} |
| net.ipv4.tcp_fin_timeout | ${net_ipv4_tcp_fin_timeout} |
| net.ipv4.tcp_tw_reuse | ${net_ipv4_tcp_tw_reuse} |
| net.ipv4.tcp_timestamps | ${net_ipv4_tcp_timestamps} |
| net.ipv4.tcp_keepalive_time | ${net_ipv4_tcp_keepalive_time} |
| net.ipv4.tcp_keepalive_intvl | ${net_ipv4_tcp_keepalive_intvl} |
| net.ipv4.tcp_keepalive_probes | ${net_ipv4_tcp_keepalive_probes} |

## Results CSV
- ${RESULTS_DIR}/results_all.csv
- ${RESULTS_DIR}/results_*.csv

## Raw Outputs
- ${RESULTS_DIR}/*.txt
- ${RESULTS_DIR}/*_cpu_rss.csv

## Notes
- TLS/WSS for non-connaxis frameworks are proxied via 'benchmark/compare/tls_proxy' and are **not apples-to-apples** with native TLS.
- In this harness, non-connaxis TLS/WSS is: client -> tls_proxy (userspace crypto/tls) -> plain TCP -> server. connaxis TLS/WSS is native and may enable kTLS (kernel TLS) when the negotiated version/cipher is supported. Treat TLS/WSS numbers as showing the effect of kTLS + avoiding proxy overhead, not a pure framework-only comparison.
- For a fair TLS comparison: (a) disable kTLS in connaxis or (b) implement native userspace TLS in each framework (no proxy), and ensure TLS version/cipher suites are identical across all runs.
- CPU/RSS are sampled during each run and summarized per row.
- Echo-path bias: gnet writes responses directly within 'OnTraffic' (event-loop), which shortens the hot path for simple echo and can improve throughput/latency in this synthetic workload. connaxis typically queues writes after parsing, which adds a small path length in echo-only scenarios but can offer better behavior under backpressure and busy FDs.
- 'tcpbp' is a “busy FD / backpressure” scenario (client read throttling) to validate behavior under real-world congestion.
- 'soak' is a long-duration TCP run for stability/long-tail behavior.

EOF2

if [[ -f "$ALL_CSV" ]]; then
REPORT_PATH="$REPORT" ALL_CSV="$ALL_CSV" "$PYTHON_BIN" - <<'PY'
import csv
from collections import defaultdict
import os

report = os.environ.get("REPORT_PATH")
csv_path = os.environ.get("ALL_CSV")

data = defaultdict(list)
with open(csv_path, newline="") as f:
    r = csv.DictReader(f)
    for row in r:
        data[row["mode"]].append(row)

sections = ["tcp", "tcpbp", "soak", "http", "ws", "tls", "wss"]
section_titles = {
    "tcp": "TCP",
    "http": "HTTP",
    "ws": "WS",
    "tls": "TLS",
    "wss": "WSS",
    "tcpbp": "TCP Backpressure",
    "soak": "TCP Soak",
}

def to_int(s, default=0):
    try:
        return int(s)
    except Exception:
        return default

framework_order = {"connaxis": 0, "tidwall": 1, "gnet": 2, "netpoll": 3}

def summarize_stats_csv(stats_path):
    try:
        with open(stats_path, newline="") as f:
            r = csv.DictReader(f)
            cpus = []
            rss_kb = []
            for row in r:
                cpus.append(float(row["cpu_percent"]))
                rss_kb.append(float(row["rss_kb"]))
        if not cpus:
            return ("NA", "NA", "NA", "NA")
        cpu_avg = sum(cpus) / len(cpus)
        cpu_max = max(cpus)
        rss_avg_kb = sum(rss_kb) / len(rss_kb)
        rss_max_kb = max(rss_kb)
        return (f"{cpu_avg:.2f}", f"{cpu_max:.2f}", f"{rss_avg_kb/1024:.2f}", f"{rss_max_kb/1024:.2f}")
    except Exception:
        return ("NA", "NA", "NA", "NA")

with open(report, "a") as out:
    out.write("## Results (From results_all.csv)\n\n")
    for mode in sections:
        rows = data.get(mode, [])
        if not rows:
            continue
        out.write(f"### {section_titles.get(mode, mode.upper())}\n\n")
        out.write("| framework | payload | conns | duration | throughput | p50 | p95 | p99 | cpu_avg | cpu_max | rss_avg_mb | rss_max_mb |\n")
        out.write("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
        rows.sort(key=lambda r: (framework_order.get(r.get("framework", ""), 99), to_int(r.get("conns")), to_int(r.get("payload"))))
        results_dir = os.path.dirname(csv_path)
        for row in rows:
            # stats file naming: <framework>_<mode>_<payload>_<conns>_cpu_rss.csv
            stats_path = f"{results_dir}/{row['framework']}_{row['mode']}_{row['payload']}_{row['conns']}_cpu_rss.csv"
            cpu_avg, cpu_max, rss_avg_mb, rss_max_mb = summarize_stats_csv(stats_path) if os.path.exists(stats_path) else ("NA", "NA", "NA", "NA")
            out.write(
                f"| {row['framework']} | {row['payload']} | {row['conns']} | {row['duration']} | {row['throughput']} | {row['p50']} | {row['p95']} | {row['p99']} | {cpu_avg} | {cpu_max} | {rss_avg_mb} | {rss_max_mb} |\n"
            )
        out.write("\n")
PY
fi

echo "Wrote $REPORT"
