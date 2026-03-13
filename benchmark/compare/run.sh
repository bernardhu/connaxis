#!/usr/bin/env bash
set -euo pipefail

# Simple benchmark runner template
# Usage: ./run.sh <framework> <mode> <addr> <duration> <conns> <payload>
# Example: ./run.sh connaxis tcp :5000 30s 200 64

FRAMEWORK="${1:-connaxis}"
MODE="${2:-tcp}"
ADDR="${3:-:5000}"
DURATION="${4:-30s}"
CONNS="${5:-200}"
PAYLOAD="${6:-64}"
NET="${NET:-tcp}"
LOOPS="${LOOPS:-}"
READ_DELAY="${READ_DELAY:-0}"
CONNAXIS_FASTPATH_TCP="${CONNAXIS_FASTPATH_TCP:-0}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="$ROOT_DIR/bin"
CLIENT_BIN="$BIN_DIR/client"
TLS_PROXY_BIN="$BIN_DIR/tls_proxy"
RESULTS_DIR="${RESULTS_DIR:-$ROOT_DIR/results}"
mkdir -p "$RESULTS_DIR"

function build_all() {
  mkdir -p "$BIN_DIR"
  (cd "$ROOT_DIR" && go mod tidy)
  (cd "$ROOT_DIR" && go build -o "$BIN_DIR/client" ./client)
  (cd "$ROOT_DIR" && go build -o "$BIN_DIR/connaxis_server" ./connaxis_server)
  (cd "$ROOT_DIR" && go build -o "$BIN_DIR/tidwall_server" ./tidwall_server)
  (cd "$ROOT_DIR" && go build -o "$BIN_DIR/gnet_server" ./gnet_server)
  (cd "$ROOT_DIR" && go build -tags netpoll -o "$BIN_DIR/netpoll_server" ./netpoll_server)
  (cd "$ROOT_DIR" && go build -o "$BIN_DIR/tls_proxy" ./tls_proxy)
}

function start_server() {
  local fw="$1"
  local mode="$2"
  local addr="$3"
  local stats_mode="${STATS_MODE_LABEL:-$mode}"
  local backend_addr="$addr"
  local proxy_pid=""
  local tls_proxy="false"

  if [[ "$mode" == "tls" || "$mode" == "wss" ]]; then
    if [[ "$fw" != "connaxis" ]]; then
      tls_proxy="true"
      local backend_offset=1
      if [[ "$mode" == "wss" ]]; then
        backend_offset=2
      fi
      backend_addr="127.0.0.1:$(( ${addr#:} + backend_offset ))"
    fi
  fi

  if [[ -z "$LOOPS" ]]; then
    if command -v nproc >/dev/null 2>&1; then
      LOOPS="$(nproc)"
    else
      LOOPS="$(sysctl -n hw.ncpu 2>/dev/null || echo 1)"
    fi
  fi

  if [[ "$tls_proxy" == "true" ]]; then
    local proxy_listen="127.0.0.1:${addr#:}"
    "$TLS_PROXY_BIN" -listen "$proxy_listen" -target "$backend_addr" &
    proxy_pid=$!
    sleep 1
  fi
  case "$fw" in
    connaxis)
      local connaxis_args=(-mode "$mode" -addr "$addr" -net "$NET" -loops "$LOOPS")
      if [[ "$CONNAXIS_FASTPATH_TCP" == "1" ]]; then
        connaxis_args+=(-fastpath-tcp)
      fi
      GOMAXPROCS="$LOOPS" "$BIN_DIR/connaxis_server" "${connaxis_args[@]}" &
      ;;
    tidwall)
      if [[ "$tls_proxy" == "true" ]]; then
        if [[ "$mode" == "wss" ]]; then
          GOMAXPROCS="$LOOPS" "$BIN_DIR/tidwall_server" -mode "ws" -addr ":${backend_addr#127.0.0.1:}" -loops "$LOOPS" &
        else
          GOMAXPROCS="$LOOPS" "$BIN_DIR/tidwall_server" -mode "tcp" -addr ":${backend_addr#127.0.0.1:}" -loops "$LOOPS" &
        fi
      else
        GOMAXPROCS="$LOOPS" "$BIN_DIR/tidwall_server" -mode "$mode" -addr "$addr" -loops "$LOOPS" &
      fi
      ;;
    gnet)
      if [[ "$tls_proxy" == "true" ]]; then
        if [[ "$mode" == "wss" ]]; then
          GOMAXPROCS="$LOOPS" "$BIN_DIR/gnet_server" -mode "ws" -addr "tcp://$backend_addr" -loops "$LOOPS" &
        else
          GOMAXPROCS="$LOOPS" "$BIN_DIR/gnet_server" -mode "tcp" -addr "tcp://$backend_addr" -loops "$LOOPS" &
        fi
      else
        GOMAXPROCS="$LOOPS" "$BIN_DIR/gnet_server" -mode "$mode" -addr "tcp://$addr" -loops "$LOOPS" &
      fi
      ;;
    netpoll)
      if [[ "$tls_proxy" == "true" ]]; then
        if [[ "$mode" == "wss" ]]; then
          GOMAXPROCS="$LOOPS" "$BIN_DIR/netpoll_server" -mode "ws" -addr "$backend_addr" &
        else
          GOMAXPROCS="$LOOPS" "$BIN_DIR/netpoll_server" -mode "tcp" -addr "$backend_addr" &
        fi
      else
        GOMAXPROCS="$LOOPS" "$BIN_DIR/netpoll_server" -mode "$mode" -addr "$addr" &
      fi
      ;;
    *)
      echo "Unknown framework: $fw" >&2
      exit 1
      ;;
  esac
  SERVER_PID=$!
  PROXY_PID=$proxy_pid
  sleep "${STARTUP_WAIT:-1}"
  if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    echo "Server failed to start: framework=$fw mode=$mode addr=$addr" >&2
    exit 1
  fi
  if [[ -n "$PROXY_PID" ]] && ! kill -0 "$PROXY_PID" >/dev/null 2>&1; then
    echo "TLS proxy failed to start: framework=$fw mode=$mode listen=127.0.0.1:${addr#:}" >&2
    exit 1
  fi
  if [[ "${COLLECT_STATS:-0}" == "1" ]]; then
    "$ROOT_DIR/collect_stats.sh" "$SERVER_PID" "$RESULTS_DIR/${fw}_${stats_mode}_${PAYLOAD}_${CONNS}_cpu_rss.csv" "${STATS_INTERVAL:-1}" &
    STATS_PID=$!
  fi
}

function stop_server() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "${PROXY_PID:-}" ]]; then
    kill "$PROXY_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "${STATS_PID:-}" ]]; then
    kill "$STATS_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "${STATS_PID:-}" ]]; then
    wait "$STATS_PID" >/dev/null 2>&1 || true
    STATS_PID=""
  fi
  if [[ -n "${PROXY_PID:-}" ]]; then
    wait "$PROXY_PID" >/dev/null 2>&1 || true
    PROXY_PID=""
  fi
  if [[ -n "${SERVER_PID:-}" ]]; then
    wait "$SERVER_PID" >/dev/null 2>&1 || true
    SERVER_PID=""
  fi
}

trap stop_server EXIT

build_all
start_server "$FRAMEWORK" "$MODE" "$ADDR"

SERVER_ADDR="127.0.0.1:${ADDR#:}"
CLIENT_ARGS=()
if [[ -n "$READ_DELAY" && "$READ_DELAY" != "0" ]]; then
  CLIENT_ARGS+=("-read-delay" "$READ_DELAY")
fi
"$CLIENT_BIN" -mode "$MODE" -addr "$SERVER_ADDR" -c "$CONNS" -payload "$PAYLOAD" -d "$DURATION" "${CLIENT_ARGS[@]}"
