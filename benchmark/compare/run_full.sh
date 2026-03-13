#!/usr/bin/env bash
set -euo pipefail

# One-command full benchmark runner.
# Usage: ./run_full.sh <addr>
# Example: NET=tcp CONNS_LIST=5000 PAYLOADS_LIST=128,512 DURATION=15s INCLUDE_TLS=1 ./run_full.sh :5000
# Optional: SOAK=1 DURATION_SOAK=30m CONNS_SOAK=50 PAYLOAD_SOAK=64
# Optional: INCLUDE_BP=1 BP_READ_DELAY=5ms

ADDR="${1:-:5000}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUN_ID="${RUN_ID:-$(date +%Y%m%d_%H%M%S)}"
RESULTS_DIR="${RESULTS_DIR:-$ROOT_DIR/results/$RUN_ID}"
GENERATE_REPORTS="${GENERATE_REPORTS:-1}"

mkdir -p "$RESULTS_DIR"

if [[ -x "$ROOT_DIR/collect_env.sh" && ! -f "$RESULTS_DIR/env.json" ]]; then
  "$ROOT_DIR/collect_env.sh" "$RESULTS_DIR/env.json"
fi

RESULTS_DIR="$RESULTS_DIR" "$ROOT_DIR/run_all.sh" "$ADDR"

if [[ "${SOAK:-0}" == "1" ]]; then
  DURATION="${DURATION_SOAK:-30m}" \
  CONNS="${CONNS_SOAK:-50}" \
  PAYLOAD="${PAYLOAD_SOAK:-64}" \
  RESULTS_DIR="$RESULTS_DIR" "$ROOT_DIR/run_soak.sh" "$ADDR"
fi

"$ROOT_DIR/merge_results.sh" "$RESULTS_DIR"
if [[ "$GENERATE_REPORTS" == "1" ]]; then
  RESULTS_DIR="$RESULTS_DIR" "$ROOT_DIR/generate_report.sh"
  "$ROOT_DIR/generate_charts.sh" "$RESULTS_DIR"
fi
