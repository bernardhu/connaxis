#!/usr/bin/env bash
set -euo pipefail

# Run a full matrix of scenarios and payload sizes.
# Usage: ./run_matrix.sh <framework> <addr>
# Example: ./run_matrix.sh connaxis :5000

FRAMEWORK="${1:-connaxis}"
ADDR="${2:-:5000}"
DURATION="${DURATION:-30s}"
CONNS="${CONNS:-5000}"
PAYLOADS_DEFAULT=(128 512)
PAYLOADS_LIST="${PAYLOADS_LIST:-}"
if [[ -n "$PAYLOADS_LIST" ]]; then
  IFS=',' read -r -a PAYLOADS <<< "$PAYLOADS_LIST"
else
  PAYLOADS=("${PAYLOADS_DEFAULT[@]}")
fi
INCLUDE_TLS="${INCLUDE_TLS:-0}"
INCLUDE_BP="${INCLUDE_BP:-0}"
BP_ONLY="${BP_ONLY:-0}"
BP_READ_DELAY="${BP_READ_DELAY:-5ms}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${RESULTS_DIR:-$ROOT_DIR/results}"
mkdir -p "$RESULTS_DIR"

if [[ -x "$ROOT_DIR/collect_env.sh" && ! -f "$RESULTS_DIR/env.json" ]]; then
  "$ROOT_DIR/collect_env.sh" "$RESULTS_DIR/env.json"
fi

function run_case() {
  local mode_label="$1"
  local payload="$2"
  local tag="$3"
  local run_mode="${4:-$mode_label}"
  local read_delay="${5:-}"
  local out="$RESULTS_DIR/${FRAMEWORK}_${mode_label}_${payload}_${CONNS}_${tag}.txt"
  echo "Running $FRAMEWORK $mode_label payload=$payload conn=$CONNS" | tee "$out"
  if [[ -n "$read_delay" ]]; then
    READ_DELAY="$read_delay" STATS_MODE_LABEL="$mode_label" COLLECT_STATS=1 RESULTS_DIR="$RESULTS_DIR" "$ROOT_DIR/run.sh" "$FRAMEWORK" "$run_mode" "$ADDR" "$DURATION" "$CONNS" "$payload" | tee -a "$out"
  else
    STATS_MODE_LABEL="$mode_label" COLLECT_STATS=1 RESULTS_DIR="$RESULTS_DIR" "$ROOT_DIR/run.sh" "$FRAMEWORK" "$run_mode" "$ADDR" "$DURATION" "$CONNS" "$payload" | tee -a "$out"
  fi
}

# Backpressure-only suite (TCP only, client read delay)
if [[ "$BP_ONLY" == "1" ]]; then
  if [[ "$INCLUDE_BP" != "1" ]]; then
    echo "BP_ONLY=1 requires INCLUDE_BP=1" >&2
    exit 2
  fi
  for p in "${PAYLOADS[@]}"; do
    run_case "tcpbp" "$p" "tcpbp" "tcp" "$BP_READ_DELAY"
  done
  if [[ -x "$ROOT_DIR/summarize_results.sh" ]]; then
    "$ROOT_DIR/summarize_results.sh" "$FRAMEWORK" "$CONNS" "$RESULTS_DIR"
  fi
  exit 0
fi

# TCP/TLS/WS/WSS
if [[ "$FRAMEWORK" == "connaxis" ]]; then
  MODES=(tcp tls ws wss)
else
  MODES=(tcp ws)
  if [[ "$INCLUDE_TLS" == "1" ]]; then
    MODES+=(tls wss)
  fi
fi
for p in "${PAYLOADS[@]}"; do
  for m in "${MODES[@]}"; do
    run_case "$m" "$p" "$m"
  done
done

# Backpressure scenario (TCP only, client read delay)
if [[ "$INCLUDE_BP" == "1" ]]; then
  for p in "${PAYLOADS[@]}"; do
    run_case "tcpbp" "$p" "tcpbp" "tcp" "$BP_READ_DELAY"
  done
fi

# HTTP (fixed payload, run once)
run_case http 0 "http"

# Summarize results
if [[ -x "$ROOT_DIR/summarize_results.sh" ]]; then
  "$ROOT_DIR/summarize_results.sh" "$FRAMEWORK" "$CONNS" "$RESULTS_DIR"
fi
