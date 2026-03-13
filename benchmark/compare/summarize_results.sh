#!/usr/bin/env bash
set -euo pipefail

# Summarize run outputs from results/ into a CSV.
# Usage: ./summarize_results.sh <framework> <conns> [results_dir]
# Example: ./summarize_results.sh connaxis 50 results

FRAMEWORK="${1:?framework required}"
CONN="${2:-NA}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${3:-$ROOT_DIR/results}"
OUT="$RESULTS_DIR/results_${FRAMEWORK}_${CONN}.csv"

echo "framework,mode,payload,conns,duration,throughput,p50,p95,p99" > "$OUT"

for f in "$RESULTS_DIR"/${FRAMEWORK}_*.txt; do
  [[ -f "$f" ]] || continue

  # Extract from filename: framework_mode_payload_conn_tag.txt
  base=$(basename "$f")
  IFS="_" read -r fw mode payload conn tag <<<"${base%.txt}"

  # Filter by requested conns when provided
  if [[ "$CONN" != "NA" && "$conn" != "$CONN" ]]; then
    continue
  fi

  # Extract the final line with metrics
  line=$(grep -E "^duration=" "$f" | tail -n1 || true)
  if [[ -z "$line" ]]; then
    continue
  fi

  # Parse fields
  duration=$(echo "$line" | sed -n 's/.*duration=\([^ ]*\).*/\1/p')
  total=$(echo "$line" | sed -n 's/.*total=\([^ ]*\).*/\1/p')
  qps=$(echo "$line" | sed -n 's/.*qps=\([^ ]*\).*/\1/p')
  p50=$(echo "$line" | sed -n 's/.*p50=\([^ ]*\).*/\1/p')
  p95=$(echo "$line" | sed -n 's/.*p95=\([^ ]*\).*/\1/p')
  p99=$(echo "$line" | sed -n 's/.*p99=\([^ ]*\).*/\1/p')

  # Conns and payload from filename, duration from line
  echo "$fw,$mode,$payload,$conn,$duration,$qps,$p50,$p95,$p99" >> "$OUT"

done

echo "Wrote $OUT"
