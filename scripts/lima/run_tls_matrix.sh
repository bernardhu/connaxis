#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LIMACTL="${LIMACTL:-limactl}"
VM_NAME="${VM_NAME:-ub2404-k61plus}"
RESULTS_ROOT="${RESULTS_ROOT:-$ROOT_DIR/benchmark/tls-suite/results}"
TLS_SUITE_DIR="$ROOT_DIR/benchmark/tls-suite"
SERVER_BIN="${SERVER_BIN:-$ROOT_DIR/benchmark/linux-lab/bin/connaxis_ws_server}"
CERT_FILE="${CERT_FILE:-$ROOT_DIR/benchmark/certs/local/lima-local-cert.pem}"
KEY_FILE="${KEY_FILE:-$ROOT_DIR/benchmark/certs/local/lima-local-key.pem}"
REMOTE_PIDFILE="${REMOTE_PIDFILE:-/tmp/evio_tls_matrix.pid}"
REMOTE_LOG_DIR="${REMOTE_LOG_DIR:-/tmp}"
SCENARIOS="${SCENARIOS:-atls-tls12,atls-tls13,ktls-tls12-tx,ktls-tls12-rxtx,ktls-tls13-tx,ktls-tls13-rxtx}"
MATRIX_RUN_ID="${MATRIX_RUN_ID:-lima_tls_matrix_${VM_NAME}_$(date +%Y%m%d_%H%M%S)}"
MATRIX_OUT_DIR="${MATRIX_OUT_DIR:-$RESULTS_ROOT/$MATRIX_RUN_ID}"

RUN_SMOKE="${RUN_SMOKE:-1}"
RUN_TESTSSL="${RUN_TESTSSL:-1}"
RUN_TLSFUZZER="${RUN_TLSFUZZER:-0}"
RUN_BOGO="${RUN_BOGO:-0}"
RUN_TLSANVIL="${RUN_TLSANVIL:-1}"
TESTSSL_STRICT="${TESTSSL_STRICT:-0}"
ENABLE_WSS_UPGRADE_CHECK="${ENABLE_WSS_UPGRADE_CHECK:-1}"
TLSFUZZER_TIMEOUT_SEC="${TLSFUZZER_TIMEOUT_SEC:-1800}"
TLSFUZZER_CMD="${TLSFUZZER_CMD:-}"
BOGO_TIMEOUT_SEC="${BOGO_TIMEOUT_SEC:-1800}"
BOGO_CMD="${BOGO_CMD:-}"
TLSANVIL_TIMEOUT_SEC="${TLSANVIL_TIMEOUT_SEC:-1800}"
TLSANVIL_CMD="${TLSANVIL_CMD:-}"
TLSFUZZER_ROOT="${TLSFUZZER_ROOT:-/private/tmp/tlsfuzzer-src}"
BOGO_LOCAL_BORINGSSL_DIR="${BOGO_LOCAL_BORINGSSL_DIR:-/private/tmp/boringssl-src}"
if [[ -x /opt/anaconda3/bin/python3 ]]; then
  PYTHON_BIN="${PYTHON_BIN:-/opt/anaconda3/bin/python3}"
else
  PYTHON_BIN="${PYTHON_BIN:-python3}"
fi
HOST_OS="${HOST_OS:-$(uname -s)}"

case "$VM_NAME" in
  ub2204-k515)
    HOST_TLS_PORT="${HOST_TLS_PORT:-32001}"
    ;;
  ub2404-k61plus)
    HOST_TLS_PORT="${HOST_TLS_PORT:-34001}"
    ;;
  *)
    echo "unsupported VM_NAME=$VM_NAME (expected ub2204-k515 or ub2404-k61plus)" >&2
    exit 2
    ;;
esac

start_server() {
  local scenario="$1"
  local remote_log="$REMOTE_LOG_DIR/evio_tls_${scenario}_$(date +%Y%m%d_%H%M%S).log"
  local extra_args=()

  case "$scenario" in
    atls-tls12)
      extra_args=(-tls-engine atls -tls-min-version tls1.2 -tls-max-version tls1.2)
      ;;
    atls-tls13)
      extra_args=(-tls-engine atls -tls-min-version tls1.3 -tls-max-version tls1.3)
      ;;
    ktls-tls12-tx)
      extra_args=(-tls-engine ktls -ktls-policy tls12-tx)
      ;;
    ktls-tls12-rxtx)
      extra_args=(-tls-engine ktls -ktls-policy tls12-rxtx)
      ;;
    ktls-tls13-tx)
      extra_args=(-tls-engine ktls -ktls-policy tls13-tx)
      ;;
    ktls-tls13-rxtx)
      extra_args=(-tls-engine ktls -ktls-policy tls13-rxtx)
      ;;
    *)
      echo "unsupported scenario=$scenario" >&2
      exit 2
      ;;
  esac

  "$LIMACTL" shell --start "$VM_NAME" bash -lc "
    if [[ -f '$REMOTE_PIDFILE' ]]; then
      pid=\$(cat '$REMOTE_PIDFILE' 2>/dev/null || true)
      if [[ -n \"\$pid\" ]]; then kill \"\$pid\" 2>/dev/null || true; fi
      rm -f '$REMOTE_PIDFILE'
    fi
    nohup '$SERVER_BIN' \
      -tls \
      -cert '$CERT_FILE' \
      -key '$KEY_FILE' \
      -addr :30001 \
      -pprof-addr= \
      -log-level info \
      ${extra_args[*]} \
      >'$remote_log' 2>&1 < /dev/null &
    echo \$! > '$REMOTE_PIDFILE'
    echo \"REMOTE_LOG=$remote_log\"
    echo \"PID=\$(cat '$REMOTE_PIDFILE')\"
  "
}

stop_server() {
  "$LIMACTL" shell --start "$VM_NAME" bash -lc "
    if [[ -f '$REMOTE_PIDFILE' ]]; then
      pid=\$(cat '$REMOTE_PIDFILE' 2>/dev/null || true)
      if [[ -n \"\$pid\" ]]; then kill \"\$pid\" 2>/dev/null || true; fi
      rm -f '$REMOTE_PIDFILE'
    fi
  "
}

run_scenario() {
  local scenario="$1"
  local run_id="lima_tls_${VM_NAME}_${scenario}_$(date +%Y%m%d_%H%M%S)"
  local scenario_tlsfuzzer_cmd="$TLSFUZZER_CMD"
  local scenario_bogo_cmd="$BOGO_CMD"
  local scenario_tlsanvil_cmd="$TLSANVIL_CMD"

  echo "=== scenario=$scenario vm=$VM_NAME port=$HOST_TLS_PORT ==="
  start_server "$scenario"
  sleep 3

  if [[ -z "$scenario_tlsfuzzer_cmd" ]]; then
    case "$scenario" in
      *tls12*)
        scenario_tlsfuzzer_cmd="printf 'tlsfuzzer is tls13-only in this matrix\\n'; exit 3"
        ;;
      *tls13*)
        scenario_tlsfuzzer_cmd="$PYTHON_BIN '$ROOT_DIR/benchmark/tls-suite/tlsfuzzer/run_local.py' --host \"\$TARGET_HOST\" --port \"\$TLS_PORT\" --out-dir \"\$OUT_DIR\" --tlsfuzzer-root '$TLSFUZZER_ROOT' --script-glob 'test_tls13_*.py'"
        ;;
    esac
  fi

  if [[ -z "$scenario_bogo_cmd" ]]; then
    local bogo_engine="atls"
    if [[ "$scenario" == ktls-* ]]; then
      bogo_engine="ktls"
    fi
    if [[ "$bogo_engine" == "ktls" && "$HOST_OS" != "Linux" ]]; then
      scenario_bogo_cmd="printf 'bogo ktls runner requires a Linux host\\n'; exit 3"
    else
      scenario_bogo_cmd="BOGO_TLS_ENGINE='$bogo_engine' BOGO_LOCAL_BORINGSSL_DIR='$BOGO_LOCAL_BORINGSSL_DIR' bash '$ROOT_DIR/benchmark/tls-suite/bogo/run_local.sh'"
    fi
  fi

  if [[ -z "$scenario_tlsanvil_cmd" ]]; then
    scenario_tlsanvil_cmd="docker run --rm -e JAVA_TOOL_OPTIONS='-XX:+UseSerialGC -Xms256m -Xmx1024m' -v \"\$OUT_DIR:/output/\" ghcr.io/tls-attacker/tlsanvil:latest -zip -parallelHandshakes 1 -connectionTimeout 200 -strength 1 -identifier connaxis server -connect \"\${TARGET_HOST}:\${TLS_PORT}\""
  fi

  (
    cd "$TLS_SUITE_DIR"
    RUN_ID="$run_id" \
    TARGET_HOST="host.docker.internal" \
    SMOKE_CONNECT_HOST="127.0.0.1" \
    SNI="localhost" \
    TLS_PORT="$HOST_TLS_PORT" \
    ENABLE_WSS_UPGRADE_CHECK="$ENABLE_WSS_UPGRADE_CHECK" \
    RUN_SMOKE="$RUN_SMOKE" \
    RUN_TESTSSL="$RUN_TESTSSL" \
    RUN_TLSFUZZER="$RUN_TLSFUZZER" \
    RUN_BOGO="$RUN_BOGO" \
    RUN_TLSANVIL="$RUN_TLSANVIL" \
    TESTSSL_STRICT="$TESTSSL_STRICT" \
    TLSFUZZER_TIMEOUT_SEC="$TLSFUZZER_TIMEOUT_SEC" \
    TLSFUZZER_CMD="$scenario_tlsfuzzer_cmd" \
    BOGO_TIMEOUT_SEC="$BOGO_TIMEOUT_SEC" \
    BOGO_CMD="$scenario_bogo_cmd" \
    TLSANVIL_TIMEOUT_SEC="$TLSANVIL_TIMEOUT_SEC" \
    TLSANVIL_CMD="$scenario_tlsanvil_cmd" \
    bash ./run_tls_suite.sh
  )

  local summary_file="$RESULTS_ROOT/$run_id/summary.txt"
  local smoke_status="UNKNOWN"
  local testssl_status="UNKNOWN"
  local tlsanvil_status="UNKNOWN"
  if [[ -f "$summary_file" ]]; then
    smoke_status="$(sed -n 's/^smoke=//p' "$summary_file" | head -n1)"
    testssl_status="$(sed -n 's/^testssl=//p' "$summary_file" | head -n1)"
    tlsanvil_status="$(sed -n 's/^tlsanvil=//p' "$summary_file" | head -n1)"
  fi
  printf '%s\t%s\t%s\t%s\t%s\n' \
    "$scenario" \
    "$run_id" \
    "$smoke_status" \
    "$testssl_status" \
    "$tlsanvil_status" >>"$MATRIX_OUT_DIR/matrix_summary.tsv"

  echo "RESULTS=$RESULTS_ROOT/$run_id"
}

bash "$ROOT_DIR/scripts/lima/build_linux_artifacts.sh"
if [[ "$RUN_TLSFUZZER" == "1" || "$RUN_BOGO" == "1" ]]; then
  bash "$ROOT_DIR/scripts/lima/ensure_tls_suite_tools.sh"
fi
mkdir -p "$MATRIX_OUT_DIR"
printf 'scenario\trun_id\tsmoke\ttestssl\ttlsanvil\n' >"$MATRIX_OUT_DIR/matrix_summary.tsv"

IFS=',' read -r -a scenario_list <<<"$SCENARIOS"
for scenario in "${scenario_list[@]}"; do
  run_scenario "$scenario"
done

stop_server
echo "MATRIX_SUMMARY=$MATRIX_OUT_DIR/matrix_summary.tsv"
