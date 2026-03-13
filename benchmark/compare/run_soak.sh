#!/usr/bin/env bash
set -euo pipefail

# Run TCP soak tests for all frameworks.
# Usage: ./run_soak.sh <addr>
# Example: NET=tcp CONNS=50 DURATION=30m PAYLOAD=64 ./run_soak.sh :6000

ADDR="${1:-:6000}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${RESULTS_DIR:-$ROOT_DIR/results}"

FRAMEWORKS=(connaxis tidwall gnet netpoll)
DURATION="${DURATION:-30m}"
CONNS="${CONNS:-50}"
PAYLOAD="${PAYLOAD:-64}"

mkdir -p "$RESULTS_DIR"

if [[ -x "$ROOT_DIR/collect_env.sh" && ! -f "$RESULTS_DIR/env.json" ]]; then
  "$ROOT_DIR/collect_env.sh" "$RESULTS_DIR/env.json"
fi

for fw in "${FRAMEWORKS[@]}"; do
  out="$RESULTS_DIR/${fw}_soak_${PAYLOAD}_${CONNS}_soak.txt"
  echo "=== Soak $fw duration=$DURATION conns=$CONNS payload=$PAYLOAD ===" | tee "$out"
  STATS_MODE_LABEL="soak" COLLECT_STATS=1 RESULTS_DIR="$RESULTS_DIR" "$ROOT_DIR/run.sh" "$fw" "tcp" "$ADDR" "$DURATION" "$CONNS" "$PAYLOAD" | tee -a "$out"
  sleep 1
  if [[ "$ADDR" =~ ^:([0-9]+)$ ]]; then
    base=${BASH_REMATCH[1]}
    base=$((base+10))
    ADDR=":$base"
  fi
done

for fw in "${FRAMEWORKS[@]}"; do
  "$ROOT_DIR/summarize_results.sh" "$fw" "$CONNS" "$RESULTS_DIR"
done

"$ROOT_DIR/merge_results.sh" "$RESULTS_DIR"
