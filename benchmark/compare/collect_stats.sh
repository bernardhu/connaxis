#!/usr/bin/env bash
set -euo pipefail

# Collect CPU/RSS stats for a PID while a test runs.
# Usage: ./collect_stats.sh <pid> <out_file> <interval_seconds>
# Example: ./collect_stats.sh 12345 stats.txt 1

PID="${1:?pid required}"
OUT="${2:?out file required}"
INTERVAL="${3:-1}"

echo "timestamp,cpu_percent,rss_kb" > "$OUT"

while kill -0 "$PID" >/dev/null 2>&1; do
  TS=$(date +%s)
  # ps output: %cpu rss
  LINE=$(ps -p "$PID" -o %cpu=,rss= | awk '{print $1 "," $2}')
  if [[ -n "$LINE" ]]; then
    echo "$TS,$LINE" >> "$OUT"
  fi
  sleep "$INTERVAL"
done
