#!/usr/bin/env bash
set -euo pipefail

# Run normal + backpressure + soak and generate a combined report.
# Usage: ./run_bundle.sh <addr>
# Example:
#   NET=tcp CONNS_LIST=5000 DURATION=60s INCLUDE_TLS=1 \
#   BP_READ_DELAY=5ms SOAK=1 DURATION_SOAK=5m CONNS_SOAK=5000 PAYLOAD_SOAK=512 \
#   ./run_bundle.sh :5000

ADDR="${1:-:5000}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_RUN_ID="${RUN_ID:-$(date +%Y%m%d_%H%M%S)}"

# Run 1: normal
RUN_ID="${BASE_RUN_ID}_base" INCLUDE_BP=0 SOAK=0 GENERATE_REPORTS=0 "$ROOT_DIR/run_full.sh" "$ADDR"

# Run 2: backpressure only (avoid duplicating base cases)
RUN_ID="${BASE_RUN_ID}_bp" INCLUDE_BP=1 BP_ONLY=1 SOAK=0 GENERATE_REPORTS=0 "$ROOT_DIR/run_full.sh" "$ADDR"

# Run 3: soak (optional)
if [[ "${SOAK:-0}" == "1" ]]; then
  SOAK_DIR="$ROOT_DIR/results/${BASE_RUN_ID}_soak"
  RESULTS_DIR="$SOAK_DIR" DURATION="${DURATION_SOAK:-30m}" CONNS="${CONNS_SOAK:-50}" PAYLOAD="${PAYLOAD_SOAK:-64}" "$ROOT_DIR/run_soak.sh" "$ADDR"
fi

# Combine results
COMBINED_DIR="$ROOT_DIR/results/${BASE_RUN_ID}_combined"
RUN_DIRS=("$ROOT_DIR/results/${BASE_RUN_ID}_base" "$ROOT_DIR/results/${BASE_RUN_ID}_bp")
if [[ "${SOAK:-0}" == "1" ]]; then
  RUN_DIRS+=("$ROOT_DIR/results/${BASE_RUN_ID}_soak")
fi

"$ROOT_DIR/merge_runs.sh" "$COMBINED_DIR" "${RUN_DIRS[@]}"
RESULTS_DIR="$COMBINED_DIR" "$ROOT_DIR/generate_report.sh"
"$ROOT_DIR/generate_charts.sh" "$COMBINED_DIR"

echo "Combined report in $COMBINED_DIR"
