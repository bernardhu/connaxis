#!/usr/bin/env bash
set -euo pipefail

# Run matrix for all frameworks and aggregate CSVs
# Usage: ./run_all.sh <addr>
# Example: NET=tcp CONNS_LIST=5000 PAYLOADS_LIST=128,512 DURATION=15s ./run_all.sh :5600

ADDR="${1:-:5000}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${RESULTS_DIR:-$ROOT_DIR/results}"

FRAMEWORKS=(connaxis tidwall gnet netpoll)
CONNS_LIST="${CONNS_LIST:-5000}"
INCLUDE_TLS="${INCLUDE_TLS:-0}"
INCLUDE_BP="${INCLUDE_BP:-0}"
BP_READ_DELAY="${BP_READ_DELAY:-5ms}"
PAYLOADS_LIST="${PAYLOADS_LIST:-}"
IFS=',' read -r -a CONNS_ARR <<< "$CONNS_LIST"

for conn in "${CONNS_ARR[@]}"; do
  export CONNS="$conn"
  for fw in "${FRAMEWORKS[@]}"; do
    echo "=== Running $fw (conns=$conn) ==="
    INCLUDE_TLS="$INCLUDE_TLS" INCLUDE_BP="$INCLUDE_BP" BP_READ_DELAY="$BP_READ_DELAY" PAYLOADS_LIST="$PAYLOADS_LIST" RESULTS_DIR="$RESULTS_DIR" "$ROOT_DIR/run_matrix.sh" "$fw" "$ADDR"
    sleep 1
    # increment port to avoid conflicts
    if [[ "$ADDR" =~ ^:([0-9]+)$ ]]; then
      base=${BASH_REMATCH[1]}
      base=$((base+10))
      ADDR=":$base"
    fi
  done
done
