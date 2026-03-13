#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${1:-$ROOT_DIR/results}"
OUT="${2:-$ROOT_DIR/../../docs/test/benchmark/PERF_REPORT_CHARTS.md}"
CSV="$RESULTS_DIR/results_all.csv"

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

if [[ ! -f "$CSV" ]]; then
  echo "results_all.csv not found" >&2
  exit 1
fi

CSV_PATH="$CSV" OUT_PATH="$OUT" "$PYTHON_BIN" - <<'PY'
import csv
import os
import math
from collections import defaultdict

csv_path = os.environ["CSV_PATH"]
out_path = os.environ["OUT_PATH"]

modes = ["tcp", "http", "ws", "tls", "wss", "tcpbp", "soak"]
mode_titles = {
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

def parse_latency_to_us(s):
    if not s or s == "NA":
        return None
    s = s.strip()
    try:
        if s.endswith("ns"):
            return float(s[:-2]) / 1000.0
        if s.endswith("us"):
            return float(s[:-2])
        if s.endswith("µs"):
            return float(s[:-2])
        if s.endswith("ms"):
            return float(s[:-2]) * 1000.0
        if s.endswith("s"):
            return float(s[:-1]) * 1_000_000.0
        return float(s)
    except Exception:
        return None

rows = []
with open(csv_path, newline="") as f:
    r = csv.DictReader(f)
    for row in r:
        if row.get("mode") in modes:
            rows.append(row)

by_mode = defaultdict(list)
for r in rows:
    by_mode[r["mode"]].append(r)

lines = ["# Performance Charts\n"]

def write_bar_chart(title, x_labels, y_label, values):
    lines.append(f"### {title}\n")
    lines.append("```mermaid")
    lines.append("xychart-beta")
    lines.append(f"    title \"{title}\"")
    lines.append(f"    x-axis [{', '.join(x_labels)}]")
    maxy = max(values) if values else 1
    maxy = int(math.ceil(maxy * 1.1))
    lines.append(f"    y-axis \"{y_label}\" 0 --> {maxy}")
    lines.append(f"    bar [{', '.join(str(int(v)) for v in values)}]")
    lines.append("```\n")

def gen_by_conns(mode, metric_key, y_label, title_suffix):
    rows = by_mode.get(mode, [])
    if not rows:
        return
    conns = sorted({r["conns"] for r in rows}, key=lambda x: to_int(x))
    payloads = sorted({r["payload"] for r in rows}, key=lambda x: to_int(x))
    if not conns or not payloads:
        return
    frameworks = sorted({r["framework"] for r in rows})
    for conn in conns:
        for payload in payloads:
            mode_rows = [r for r in rows if r["conns"] == conn and r["payload"] == payload]
            if not mode_rows:
                continue
            values = []
            for fw in frameworks:
                match = next((r for r in mode_rows if r["framework"] == fw), None)
                if not match:
                    values.append(0)
                    continue
                if metric_key == "p99":
                    v = parse_latency_to_us(match.get("p99"))
                    values.append(v if v is not None else 0)
                else:
                    values.append(float(match.get(metric_key, 0)))
            title = f"{mode_titles.get(mode, mode.upper())} {title_suffix} (conns={conn}, payload={payload})"
            write_bar_chart(title, frameworks, y_label, values)

for m in modes:
    gen_by_conns(m, "throughput", "msg/s", "Throughput")
    gen_by_conns(m, "p99", "p99 (us)", "P99")

with open(out_path, "w") as f:
    f.write("\n".join(lines))

print(f"Wrote {out_path}")
PY
