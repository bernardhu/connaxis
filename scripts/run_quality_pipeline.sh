#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

PROFILE="${PROFILE:-quick}" # quick|full
RESULTS_ROOT="${RESULTS_ROOT:-$ROOT_DIR/benchmark/quality-results}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d_%H%M%S)}"
OUT_DIR="$RESULTS_ROOT/$RUN_ID"
LOG_DIR="$OUT_DIR/logs"
SUMMARY_FILE="$OUT_DIR/summary.txt"

FAIL_FAST="${FAIL_FAST:-0}"
STRICT="${STRICT:-0}"

RUN_LINT="${RUN_LINT:-1}"
RUN_TEST="${RUN_TEST:-1}"
RUN_RACE="${RUN_RACE:-1}"
RUN_GOLEAK="${RUN_GOLEAK:-1}"
RUN_RACE_STRESS="${RUN_RACE_STRESS:-0}"
RUN_GOLEAK_STRESS="${RUN_GOLEAK_STRESS:-0}"
RUN_BENCHSTAT="${RUN_BENCHSTAT:-1}"
RUN_WS_AUTOBAHN="${RUN_WS_AUTOBAHN:-0}"
RUN_TLS_SUITE="${RUN_TLS_SUITE:-0}"
RUN_COMPARE="${RUN_COMPARE:-0}"

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  cat <<'EOF'
Usage: scripts/run_quality_pipeline.sh [quick|full]

Profiles:
  quick (default): lint, test, race, goleak, benchstat
  full: quick + ws-autobahn + tls-suite + compare matrix

Key env flags:
  RUN_LINT=0|1
  RUN_TEST=0|1
  RUN_RACE=0|1
  RUN_GOLEAK=0|1
  RUN_RACE_STRESS=0|1
  RUN_GOLEAK_STRESS=0|1
  RUN_BENCHSTAT=0|1
  RUN_WS_AUTOBAHN=0|1
  RUN_TLS_SUITE=0|1
  RUN_COMPARE=0|1
  FAIL_FAST=0|1
  STRICT=0|1
EOF
  exit 0
fi

if [[ -n "${1:-}" ]]; then
  PROFILE="$1"
fi

if [[ "$PROFILE" == "full" ]]; then
  RUN_WS_AUTOBAHN="${RUN_WS_AUTOBAHN:-1}"
  RUN_TLS_SUITE="${RUN_TLS_SUITE:-1}"
  RUN_COMPARE="${RUN_COMPARE:-1}"
fi

mkdir -p "$LOG_DIR"

steps=()
statuses=()
logs=()

record() {
  steps+=("$1")
  statuses+=("$2")
  logs+=("$3")
}

run_cmd_step() {
  local name="$1"
  local cmd="$2"
  local log="$LOG_DIR/${name}.log"

  echo "[quality] >>> ${name}"
  set +e
  bash -lc "$cmd" >"$log" 2>&1
  local rc=$?
  set -e

  local status="PASS"
  if [[ $rc -ne 0 ]]; then
    status="FAIL"
    if [[ "$name" == "race" ]] && grep -q "runtime/race: package testmain: cannot find package" "$log"; then
      status="WARN"
    fi
  fi

  echo "[quality] <<< ${name}: ${status}"
  record "$name" "$status" "$log"

  if [[ "$status" == "FAIL" && "$FAIL_FAST" == "1" ]]; then
    write_summary
    exit 1
  fi
}

run_skip_step() {
  local name="$1"
  local reason="$2"
  local log="$LOG_DIR/${name}.log"
  printf "%s\n" "$reason" >"$log"
  echo "[quality] <<< ${name}: SKIP ($reason)"
  record "$name" "SKIP" "$log"
}

resolve_golangci() {
  if command -v golangci-lint >/dev/null 2>&1; then
    command -v golangci-lint
    return 0
  fi
  if [[ -x "/tmp/gobin/golangci-lint" ]]; then
    echo "/tmp/gobin/golangci-lint"
    return 0
  fi
  if [[ -x "$(go env GOPATH)/bin/golangci-lint" ]]; then
    echo "$(go env GOPATH)/bin/golangci-lint"
    return 0
  fi
  return 1
}

write_summary() {
  mkdir -p "$OUT_DIR"
  {
    echo "run_id=$RUN_ID"
    echo "profile=$PROFILE"
    echo "repo=$ROOT_DIR"
    echo
    local i
    for ((i = 0; i < ${#steps[@]}; i++)); do
      echo "${steps[$i]}=${statuses[$i]} log=${logs[$i]}"
    done
  } | tee "$SUMMARY_FILE"
}

echo "[quality] run_id=$RUN_ID profile=$PROFILE out=$OUT_DIR"

if [[ "$RUN_LINT" == "1" ]]; then
  if golangci_bin="$(resolve_golangci)"; then
    run_cmd_step "lint" "cd '$ROOT_DIR' && HOME=/tmp/codex-home GOPATH='${GOPATH:-$(go env GOPATH)}' GOCACHE=/tmp/codex-home/Library/Caches/go-build GOOS=linux GOARCH=amd64 '$golangci_bin' run --timeout=5m"
  else
    run_skip_step "lint" "golangci-lint not found in PATH (/tmp/gobin and GOPATH/bin also checked)"
  fi
fi

if [[ "$RUN_TEST" == "1" ]]; then
  run_cmd_step "test" "cd '$ROOT_DIR' && go test ./..."
fi

if [[ "$RUN_RACE" == "1" ]]; then
  run_cmd_step "race" "cd '$ROOT_DIR' && ./scripts/run_race.sh"
fi

if [[ "$RUN_GOLEAK" == "1" ]]; then
  run_cmd_step "goleak" "cd '$ROOT_DIR' && ./scripts/run_goleak_pilot.sh"
fi

if [[ "$RUN_RACE_STRESS" == "1" ]]; then
  run_cmd_step "race-stress-ws-wss" "cd '$ROOT_DIR' && ./scripts/run_ws_wss_race_stress.sh"
fi

if [[ "$RUN_GOLEAK_STRESS" == "1" ]]; then
  run_cmd_step "goleak-stress-ws-wss" "cd '$ROOT_DIR' && ./scripts/run_ws_wss_goleak_stress.sh"
fi

if [[ "$RUN_BENCHSTAT" == "1" ]]; then
  BASELINE_REF="${BASELINE_REF:-HEAD}"
  TARGET_REF="${TARGET_REF:-HEAD}"
  BENCH_PACKAGE="${BENCH_PACKAGE:-./websocket}"
  BENCH_REGEX="${BENCH_REGEX:-BenchmarkProcessHandshake}"
  BENCH_COUNT="${BENCH_COUNT:-3}"
  run_cmd_step "benchstat" "cd '$ROOT_DIR' && ./scripts/benchstat_compare.sh '$BASELINE_REF' '$TARGET_REF' '$BENCH_PACKAGE' '$BENCH_REGEX' '$BENCH_COUNT'"
fi

if [[ "$RUN_WS_AUTOBAHN" == "1" ]]; then
  TARGET_URL="${TARGET_URL:-ws://127.0.0.1:30000}"
  AUTOBAHN_REPORTS_DIR="${AUTOBAHN_REPORTS_DIR:-$OUT_DIR/autobahn}"
  run_cmd_step "ws-autobahn" "cd '$ROOT_DIR/benchmark/ws-autobahn' && TARGET_URL='$TARGET_URL' REPORTS_DIR='$AUTOBAHN_REPORTS_DIR' ./run.sh"
fi

if [[ "$RUN_TLS_SUITE" == "1" ]]; then
  TLS_TARGET_HOST="${TLS_TARGET_HOST:-127.0.0.1}"
  TLS_PORT="${TLS_PORT:-30001}"
  TLS_SNI="${TLS_SNI:-$TLS_TARGET_HOST}"
  TLS_RESULTS_ROOT="${TLS_RESULTS_ROOT:-$OUT_DIR/tls-suite}"
  run_cmd_step "tls-suite" "cd '$ROOT_DIR/benchmark/tls-suite' && RESULTS_ROOT='$TLS_RESULTS_ROOT' TARGET_HOST='$TLS_TARGET_HOST' TLS_PORT='$TLS_PORT' SNI='$TLS_SNI' RUN_SMOKE='${TLS_RUN_SMOKE:-1}' RUN_TESTSSL='${TLS_RUN_TESTSSL:-0}' RUN_TLSFUZZER='${TLS_RUN_TLSFUZZER:-0}' RUN_BOGO='${TLS_RUN_BOGO:-0}' RUN_TLSANVIL='${TLS_RUN_TLSANVIL:-0}' TESTSSL_STRICT='${TLS_TESTSSL_STRICT:-0}' bash ./run_tls_suite.sh"
fi

if [[ "$RUN_COMPARE" == "1" ]]; then
  COMPARE_FRAMEWORK="${COMPARE_FRAMEWORK:-connaxis}"
  COMPARE_ADDR="${COMPARE_ADDR:-:5000}"
  COMPARE_RESULTS_DIR="${COMPARE_RESULTS_DIR:-$OUT_DIR/compare}"
  run_cmd_step "compare-matrix" "cd '$ROOT_DIR/benchmark/compare' && RESULTS_DIR='$COMPARE_RESULTS_DIR' ./run_matrix.sh '$COMPARE_FRAMEWORK' '$COMPARE_ADDR'"
fi

write_summary

overall_fail=0
overall_warn=0
for s in "${statuses[@]}"; do
  if [[ "$s" == "FAIL" ]]; then
    overall_fail=1
  fi
  if [[ "$s" == "WARN" ]]; then
    overall_warn=1
  fi
done

if [[ $overall_fail -ne 0 ]]; then
  echo "[quality] DONE: FAIL (summary: $SUMMARY_FILE)"
  exit 1
fi

if [[ $overall_warn -ne 0 && "$STRICT" == "1" ]]; then
  echo "[quality] DONE: WARN treated as FAIL in STRICT=1 (summary: $SUMMARY_FILE)"
  exit 1
fi

echo "[quality] DONE: PASS (summary: $SUMMARY_FILE)"
