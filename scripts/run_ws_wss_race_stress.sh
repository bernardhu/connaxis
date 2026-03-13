#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPARE_DIR="$ROOT_DIR/benchmark/compare"
BIN_DIR="$COMPARE_DIR/bin"
RUN_ID="${RUN_ID:-$(date +%Y%m%d_%H%M%S)}"
LOG_DIR="${LOG_DIR:-$ROOT_DIR/benchmark/quality-results/race_stress_${RUN_ID}}"

WS_PORT="${WS_PORT:-36100}"
WSS_PORT="${WSS_PORT:-36101}"
CONNS="${CONNS:-300}"
DURATION="${DURATION:-8s}"
PAYLOADS="${PAYLOADS:-128,512}"
NET="${NET:-tcp4}"

if command -v nproc >/dev/null 2>&1; then
  LOOPS_DEFAULT="$(nproc)"
else
  LOOPS_DEFAULT="$(sysctl -n hw.ncpu 2>/dev/null || echo 1)"
fi
LOOPS="${LOOPS:-$LOOPS_DEFAULT}"

mkdir -p "$LOG_DIR"

cleanup_pid=""
cleanup() {
  if [[ -n "$cleanup_pid" ]]; then
    kill "$cleanup_pid" >/dev/null 2>&1 || true
    wait "$cleanup_pid" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo "[race-stress] building race binaries..."
(
  cd "$COMPARE_DIR"
  go build -race -o "$BIN_DIR/client_race" ./client
  go build -race -o "$BIN_DIR/connaxis_server_race" ./connaxis_server
)

IFS=',' read -r -a payload_arr <<<"$PAYLOADS"

run_mode() {
  local mode="$1"
  local port="$2"
  local server_log="$LOG_DIR/server_${mode}.log"

  echo "[race-stress] starting server mode=$mode port=$port loops=$LOOPS"
  if [[ "$mode" == "wss" ]]; then
    "$BIN_DIR/connaxis_server_race" -mode "$mode" -addr ":$port" -net "$NET" -loops "$LOOPS" \
      -cert "$ROOT_DIR/benchmark/certs/local/lima-local-cert.pem" -key "$ROOT_DIR/benchmark/certs/local/lima-local-key.pem" \
      >"$server_log" 2>&1 &
  else
    "$BIN_DIR/connaxis_server_race" -mode "$mode" -addr ":$port" -net "$NET" -loops "$LOOPS" \
      >"$server_log" 2>&1 &
  fi
  cleanup_pid=$!
  sleep 1

  if ! kill -0 "$cleanup_pid" >/dev/null 2>&1; then
    echo "[race-stress] server failed to start mode=$mode"
    sed -n '1,120p' "$server_log" || true
    exit 1
  fi

  for payload in "${payload_arr[@]}"; do
    local client_log="$LOG_DIR/client_${mode}_${payload}.log"
    echo "[race-stress] running client mode=$mode payload=$payload conns=$CONNS duration=$DURATION"
    local out
    out="$("$BIN_DIR/client_race" -mode "$mode" -addr "127.0.0.1:$port" -c "$CONNS" -payload "$payload" -d "$DURATION")"
    echo "$out" | tee "$client_log"

    local total
    total="$(echo "$out" | sed -n 's/.*total=\([0-9][0-9]*\).*/\1/p' | tail -n1)"
    if [[ -z "$total" || "$total" -le 0 ]]; then
      echo "[race-stress] invalid client result mode=$mode payload=$payload total=$total"
      exit 1
    fi
  done

  kill "$cleanup_pid" >/dev/null 2>&1 || true
  wait "$cleanup_pid" >/dev/null 2>&1 || true
  cleanup_pid=""
}

run_mode ws "$WS_PORT"
run_mode wss "$WSS_PORT"

echo "[race-stress] scanning logs for race reports..."
if grep -R "WARNING: DATA RACE" "$LOG_DIR" >/dev/null 2>&1; then
  echo "[race-stress] detected data race"
  grep -R "WARNING: DATA RACE" "$LOG_DIR"
  exit 1
fi

echo "[race-stress] PASS logs=$LOG_DIR"
