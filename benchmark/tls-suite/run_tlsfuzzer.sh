#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/results/tlsfuzzer_$(date +%Y%m%d_%H%M%S)}"

TARGET_HOST="${TARGET_HOST:-127.0.0.1}"
TLS_PORT="${TLS_PORT:-30001}"
SNI="${SNI:-$TARGET_HOST}"
TLSFUZZER_TIMEOUT_SEC="${TLSFUZZER_TIMEOUT_SEC:-1800}"
TLSFUZZER_CMD="${TLSFUZZER_CMD:-}"

mkdir -p "$OUT_DIR"

SUMMARY_TXT="$OUT_DIR/tlsfuzzer_summary.txt"
LOG_TXT="$OUT_DIR/tlsfuzzer.log"
META_TXT="$OUT_DIR/tlsfuzzer_meta.txt"
RC_TXT="$OUT_DIR/tlsfuzzer_exit_code.txt"

{
  echo "target=${TARGET_HOST}:${TLS_PORT}"
  echo "sni=${SNI}"
  echo "timeout_sec=${TLSFUZZER_TIMEOUT_SEC}"
  echo "cmd=${TLSFUZZER_CMD}"
} >"$META_TXT"

TIMEOUT_BIN=""
if command -v timeout >/dev/null 2>&1; then
  TIMEOUT_BIN="timeout"
elif command -v gtimeout >/dev/null 2>&1; then
  TIMEOUT_BIN="gtimeout"
fi

run_with_timeout() {
  local sec="$1"
  shift
  if [[ -n "$TIMEOUT_BIN" ]]; then
    "$TIMEOUT_BIN" "${sec}s" "$@"
  else
    "$@" &
    local cmd_pid=$!
    (
      sleep "$sec"
      kill -TERM "$cmd_pid" >/dev/null 2>&1 || true
      sleep 1
      kill -KILL "$cmd_pid" >/dev/null 2>&1 || true
    ) &
    local killer_pid=$!
    local rc=0
    wait "$cmd_pid" || rc=$?
    kill -TERM "$killer_pid" >/dev/null 2>&1 || true
    wait "$killer_pid" >/dev/null 2>&1 || true
    return "$rc"
  fi
}

if [[ -z "$TLSFUZZER_CMD" ]]; then
  {
    echo "result=SKIPPED"
    echo "reason=TLSFUZZER_CMD is empty"
  } | tee "$SUMMARY_TXT"
  exit 2
fi

set +e
run_with_timeout "$TLSFUZZER_TIMEOUT_SEC" env \
  TARGET_HOST="$TARGET_HOST" \
  TLS_PORT="$TLS_PORT" \
  SNI="$SNI" \
  OUT_DIR="$OUT_DIR" \
  bash -lc "$TLSFUZZER_CMD" >"$LOG_TXT" 2>&1
rc=$?
set -e
echo "$rc" >"$RC_TXT"

if [[ "$rc" == "0" ]]; then
  {
    echo "result=PASS"
    echo "exit_code=0"
  } | tee "$SUMMARY_TXT"
  exit 0
fi

if [[ "$rc" == "3" ]]; then
  {
    echo "result=N/A"
    echo "exit_code=3"
  } | tee "$SUMMARY_TXT"
  exit 3
fi

{
  echo "result=FAIL"
  echo "exit_code=$rc"
} | tee "$SUMMARY_TXT"
exit "$rc"
