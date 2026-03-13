#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/results/smoke_$(date +%Y%m%d_%H%M%S)}"

TARGET_HOST="${TARGET_HOST:-127.0.0.1}"
TLS_PORT="${TLS_PORT:-30001}"
SNI="${SNI:-$TARGET_HOST}"
WSS_PATH="${WSS_PATH:-/}"
ENABLE_WSS_UPGRADE_CHECK="${ENABLE_WSS_UPGRADE_CHECK:-1}"
SMOKE_CONNECT_HOST="${SMOKE_CONNECT_HOST:-$TARGET_HOST}"

mkdir -p "$OUT_DIR"

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl not found" >&2
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
    # Portable timeout fallback for environments without timeout/gtimeout.
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

TARGET="${TARGET_HOST}:${TLS_PORT}"
CONNECT_TARGET="${SMOKE_CONNECT_HOST}:${TLS_PORT}"
HANDSHAKE_OUT="$OUT_DIR/handshake.txt"
WSS_UPGRADE_OUT="$OUT_DIR/wss_upgrade.txt"
SUMMARY_OUT="$OUT_DIR/smoke_summary.txt"

echo "[tls-smoke] target=$TARGET connect=$CONNECT_TARGET sni=$SNI out=$OUT_DIR"

run_with_timeout 15 bash -lc "echo | openssl s_client -connect '$CONNECT_TARGET' -servername '$SNI'" >"$HANDSHAKE_OUT" 2>&1 || true

HANDSHAKE_OK=0
protocol_line="$(awk -F: '/^[[:space:]]*Protocol[[:space:]]*:/{gsub(/^[[:space:]]+/, "", $2); print $2; exit}' "$HANDSHAKE_OUT" || true)"
cipher_line="$(awk -F: '/^[[:space:]]*Cipher[[:space:]]*:/{gsub(/^[[:space:]]+/, "", $2); print $2; exit}' "$HANDSHAKE_OUT" || true)"
if [[ -z "$protocol_line" ]]; then
  protocol_line="$(sed -n 's/^New, \(TLSv[^,]*\), Cipher is .*/\1/p' "$HANDSHAKE_OUT" | head -n 1)"
fi
if [[ -z "$cipher_line" ]]; then
  cipher_line="$(sed -n 's/^New, TLSv[^,]*, Cipher is \(.*\)$/\1/p' "$HANDSHAKE_OUT" | head -n 1)"
fi

if [[ "$protocol_line" == TLS* ]] && [[ -n "$cipher_line" ]] && [[ "$cipher_line" != "0000" ]] && [[ "$cipher_line" != "(NONE)" ]]; then
  if ! grep -q "no peer certificate available" "$HANDSHAKE_OUT"; then
    HANDSHAKE_OK=1
  fi
fi

WSS_OK="SKIPPED"
if [[ "$ENABLE_WSS_UPGRADE_CHECK" == "1" ]]; then
  req_file="$OUT_DIR/wss_upgrade.req"
  cat >"$req_file" <<EOF
GET ${WSS_PATH} HTTP/1.1
Host: ${TARGET}
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13

EOF
  # Convert LF to CRLF for strict servers.
  sed -i.bak 's/$/\r/' "$req_file"
  run_with_timeout 15 bash -lc "cat '$req_file' | openssl s_client -connect '$CONNECT_TARGET' -servername '$SNI' -quiet" >"$WSS_UPGRADE_OUT" 2>&1 || true
  rm -f "$req_file.bak"

  if grep -Eq "HTTP/1\\.[01] 101" "$WSS_UPGRADE_OUT"; then
    WSS_OK="PASS"
  else
    WSS_OK="FAIL"
  fi
fi

{
  echo "target=${TARGET}"
  echo "connect_target=${CONNECT_TARGET}"
  echo "sni=${SNI}"
  echo "handshake_ok=${HANDSHAKE_OK}"
  echo "wss_upgrade=${WSS_OK}"
  if [[ "$HANDSHAKE_OK" == "1" ]]; then
    echo "result=PASS"
  else
    echo "result=FAIL"
  fi
} | tee "$SUMMARY_OUT"

if [[ "$HANDSHAKE_OK" != "1" ]]; then
  exit 1
fi
