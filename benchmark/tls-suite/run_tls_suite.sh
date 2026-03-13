#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ -f "$ROOT_DIR/targets.env" ]]; then
  # shellcheck source=/dev/null
  source "$ROOT_DIR/targets.env"
fi

RUN_ID="${RUN_ID:-$(date +%Y%m%d_%H%M%S)}"
RESULTS_ROOT="${RESULTS_ROOT:-$ROOT_DIR/results/$RUN_ID}"

TARGET_HOST="${TARGET_HOST:-127.0.0.1}"
TLS_PORT="${TLS_PORT:-30001}"
SNI="${SNI:-$TARGET_HOST}"
WSS_PATH="${WSS_PATH:-/}"
ENABLE_WSS_UPGRADE_CHECK="${ENABLE_WSS_UPGRADE_CHECK:-1}"
SMOKE_CONNECT_HOST="${SMOKE_CONNECT_HOST:-$TARGET_HOST}"

RUN_SMOKE="${RUN_SMOKE:-1}"
RUN_TESTSSL="${RUN_TESTSSL:-1}"
RUN_TLSFUZZER="${RUN_TLSFUZZER:-0}"
RUN_BOGO="${RUN_BOGO:-0}"
RUN_TLSANVIL="${RUN_TLSANVIL:-0}"
TESTSSL_STRICT="${TESTSSL_STRICT:-0}"
TESTSSL_ADD_HOST="${TESTSSL_ADD_HOST:-}"
DOCKER_TESTSSL_IMAGE="${DOCKER_TESTSSL_IMAGE:-drwetter/testssl.sh:latest}"
TESTSSL_OPTS="${TESTSSL_OPTS:---fast --sneaky}"
TESTSSL_TIMEOUT_SEC="${TESTSSL_TIMEOUT_SEC:-900}"
DOCKER_NETWORK_MODE="${DOCKER_NETWORK_MODE:-}"
TLSFUZZER_TIMEOUT_SEC="${TLSFUZZER_TIMEOUT_SEC:-1800}"
TLSFUZZER_CMD="${TLSFUZZER_CMD:-}"
BOGO_TIMEOUT_SEC="${BOGO_TIMEOUT_SEC:-1800}"
BOGO_CMD="${BOGO_CMD:-}"
TLSANVIL_TIMEOUT_SEC="${TLSANVIL_TIMEOUT_SEC:-1800}"
TLSANVIL_CMD="${TLSANVIL_CMD:-}"

mkdir -p "$RESULTS_ROOT"
SUMMARY_FILE="$RESULTS_ROOT/summary.txt"

echo "[tls-suite] run_id=$RUN_ID results=$RESULTS_ROOT"
echo "[tls-suite] target=${TARGET_HOST}:${TLS_PORT} sni=$SNI"

SMOKE_STATUS="SKIPPED"
TESTSSL_STATUS="SKIPPED"
TLSFUZZER_STATUS="SKIPPED"
BOGO_STATUS="SKIPPED"
TLSANVIL_STATUS="SKIPPED"

if [[ "$RUN_SMOKE" == "1" ]]; then
  if OUT_DIR="$RESULTS_ROOT/smoke" \
    TARGET_HOST="$TARGET_HOST" \
    TLS_PORT="$TLS_PORT" \
    SNI="$SNI" \
    WSS_PATH="$WSS_PATH" \
    ENABLE_WSS_UPGRADE_CHECK="$ENABLE_WSS_UPGRADE_CHECK" \
    SMOKE_CONNECT_HOST="$SMOKE_CONNECT_HOST" \
    "$ROOT_DIR/run_tls_smoke.sh"; then
    SMOKE_STATUS="PASS"
  else
    SMOKE_STATUS="FAIL"
  fi
fi

if [[ "$RUN_TESTSSL" == "1" ]]; then
  if OUT_DIR="$RESULTS_ROOT/testssl" \
    TARGET_HOST="$TARGET_HOST" \
    TLS_PORT="$TLS_PORT" \
    TESTSSL_STRICT="$TESTSSL_STRICT" \
    TESTSSL_ADD_HOST="$TESTSSL_ADD_HOST" \
    DOCKER_TESTSSL_IMAGE="$DOCKER_TESTSSL_IMAGE" \
    TESTSSL_OPTS="$TESTSSL_OPTS" \
    TESTSSL_TIMEOUT_SEC="$TESTSSL_TIMEOUT_SEC" \
    DOCKER_NETWORK_MODE="$DOCKER_NETWORK_MODE" \
    "$ROOT_DIR/run_testssl.sh"; then
    rc_file="$RESULTS_ROOT/testssl/testssl_exit_code.txt"
    if [[ -f "$rc_file" ]]; then
      rc="$(cat "$rc_file")"
      if [[ "$rc" == "0" ]]; then
        TESTSSL_STATUS="PASS"
      else
        if [[ "$TESTSSL_STRICT" == "1" ]]; then
          TESTSSL_STATUS="FAIL"
        else
          TESTSSL_STATUS="WARN(rc=${rc})"
        fi
      fi
    else
      TESTSSL_STATUS="PASS"
    fi
  else
    TESTSSL_STATUS="FAIL"
  fi
fi

if [[ "$RUN_TLSFUZZER" == "1" ]]; then
  set +e
  OUT_DIR="$RESULTS_ROOT/tlsfuzzer" \
    TARGET_HOST="$TARGET_HOST" \
    TLS_PORT="$TLS_PORT" \
    SNI="$SNI" \
    TLSFUZZER_TIMEOUT_SEC="$TLSFUZZER_TIMEOUT_SEC" \
    TLSFUZZER_CMD="$TLSFUZZER_CMD" \
    "$ROOT_DIR/run_tlsfuzzer.sh"
  rc=$?
  set -e
  if [[ "$rc" == "0" ]]; then
    TLSFUZZER_STATUS="PASS"
  elif [[ "$rc" == "3" ]]; then
    TLSFUZZER_STATUS="N/A"
  elif [[ "$rc" == "2" ]]; then
    TLSFUZZER_STATUS="SKIPPED"
  else
    TLSFUZZER_STATUS="FAIL"
  fi
fi

if [[ "$RUN_BOGO" == "1" ]]; then
  set +e
  OUT_DIR="$RESULTS_ROOT/bogo" \
    TARGET_HOST="$TARGET_HOST" \
    TLS_PORT="$TLS_PORT" \
    SNI="$SNI" \
    BOGO_TIMEOUT_SEC="$BOGO_TIMEOUT_SEC" \
    BOGO_CMD="$BOGO_CMD" \
    "$ROOT_DIR/run_bogo.sh"
  rc=$?
  set -e
  if [[ "$rc" == "0" ]]; then
    BOGO_STATUS="PASS"
  elif [[ "$rc" == "3" ]]; then
    BOGO_STATUS="N/A"
  elif [[ "$rc" == "2" ]]; then
    BOGO_STATUS="SKIPPED"
  else
    BOGO_STATUS="FAIL"
  fi
fi

if [[ "$RUN_TLSANVIL" == "1" ]]; then
  set +e
  OUT_DIR="$RESULTS_ROOT/tlsanvil" \
    TARGET_HOST="$TARGET_HOST" \
    TLS_PORT="$TLS_PORT" \
    SNI="$SNI" \
    TLSANVIL_TIMEOUT_SEC="$TLSANVIL_TIMEOUT_SEC" \
    TLSANVIL_CMD="$TLSANVIL_CMD" \
    "$ROOT_DIR/run_tlsanvil.sh"
  rc=$?
  set -e
  if [[ "$rc" == "0" ]]; then
    TLSANVIL_STATUS="PASS"
  elif [[ "$rc" == "3" ]]; then
    TLSANVIL_STATUS="N/A"
  elif [[ "$rc" == "2" ]]; then
    TLSANVIL_STATUS="SKIPPED"
  else
    TLSANVIL_STATUS="FAIL"
  fi
fi

{
  echo "run_id=$RUN_ID"
  echo "target=${TARGET_HOST}:${TLS_PORT}"
  echo "sni=$SNI"
  echo "wss_path=$WSS_PATH"
  echo "smoke=$SMOKE_STATUS"
  echo "testssl=$TESTSSL_STATUS"
  echo "tlsfuzzer=$TLSFUZZER_STATUS"
  echo "bogo=$BOGO_STATUS"
  echo "tlsanvil=$TLSANVIL_STATUS"
} | tee "$SUMMARY_FILE"

echo "summary: $SUMMARY_FILE"
