#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/results/testssl_$(date +%Y%m%d_%H%M%S)}"

TARGET_HOST="${TARGET_HOST:-127.0.0.1}"
TLS_PORT="${TLS_PORT:-30001}"
TARGET="${TARGET_HOST}:${TLS_PORT}"

DOCKER_TESTSSL_IMAGE="${DOCKER_TESTSSL_IMAGE:-drwetter/testssl.sh:latest}"
TESTSSL_OPTS="${TESTSSL_OPTS:---fast --sneaky}"
TESTSSL_STRICT="${TESTSSL_STRICT:-0}"
TESTSSL_TIMEOUT_SEC="${TESTSSL_TIMEOUT_SEC:-900}"
TESTSSL_ADD_HOST="${TESTSSL_ADD_HOST:-}"
DOCKER_PLATFORM="${DOCKER_PLATFORM:-}"
default_network_mode="host"
if [[ "$(uname -s)" == "Darwin" ]]; then
  default_network_mode="bridge"
fi
DOCKER_NETWORK_MODE="${DOCKER_NETWORK_MODE:-$default_network_mode}"

mkdir -p "$OUT_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not found" >&2
  exit 127
fi

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

REPORT_TXT="$OUT_DIR/testssl_report.txt"
META_TXT="$OUT_DIR/testssl_meta.txt"
RC_TXT="$OUT_DIR/testssl_exit_code.txt"

echo "[testssl] image=$DOCKER_TESTSSL_IMAGE target=$TARGET out=$OUT_DIR"

{
  echo "image=$DOCKER_TESTSSL_IMAGE"
  echo "target=$TARGET"
  echo "opts=$TESTSSL_OPTS"
  echo "strict=$TESTSSL_STRICT"
  echo "timeout_sec=$TESTSSL_TIMEOUT_SEC"
  echo "add_host=$TESTSSL_ADD_HOST"
  echo "docker_network_mode=$DOCKER_NETWORK_MODE"
  echo "docker_platform=$DOCKER_PLATFORM"
} >"$META_TXT"

DOCKER_CMD=(docker run --rm --network "$DOCKER_NETWORK_MODE" -v "$OUT_DIR:/out")
if [[ -n "$DOCKER_PLATFORM" ]]; then
  DOCKER_CMD+=(--platform "$DOCKER_PLATFORM")
fi
if [[ -n "$TESTSSL_ADD_HOST" ]]; then
  DOCKER_CMD+=(--add-host "$TESTSSL_ADD_HOST")
fi
DOCKER_CMD+=("$DOCKER_TESTSSL_IMAGE")

# shellcheck disable=SC2206
TESTSSL_OPTS_ARR=($TESTSSL_OPTS)
DOCKER_CMD+=("${TESTSSL_OPTS_ARR[@]}")
DOCKER_CMD+=(--warnings batch "$TARGET")

set +e
run_with_timeout "$TESTSSL_TIMEOUT_SEC" "${DOCKER_CMD[@]}" >"$REPORT_TXT" 2>&1
rc=$?
set -e

echo "$rc" >"$RC_TXT"

if [[ "$rc" != "0" ]]; then
  echo "testssl exited non-zero: $rc (see $REPORT_TXT)"
fi

echo "testssl report: $REPORT_TXT"

if [[ "$TESTSSL_STRICT" == "1" && "$rc" != "0" ]]; then
  exit "$rc"
fi
