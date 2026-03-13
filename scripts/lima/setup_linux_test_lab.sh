#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LIMACTL="${LIMACTL:-limactl}"
LIMA_HOME="${LIMA_HOME:-$HOME/.lima}"

VM_CPUS="${VM_CPUS:-4}"
VM_MEMORY="${VM_MEMORY:-4}"
VM_DISK="${VM_DISK:-40}"

JAMMY_VM_NAME="${JAMMY_VM_NAME:-ub2204-k515}"
NOBLE_VM_NAME="${NOBLE_VM_NAME:-ub2404-k61plus}"

JAMMY_WS_PORT="${JAMMY_WS_PORT:-32000}"
JAMMY_WSS_PORT="${JAMMY_WSS_PORT:-32001}"
JAMMY_PPROF_PORT="${JAMMY_PPROF_PORT:-32002}"

NOBLE_WS_PORT="${NOBLE_WS_PORT:-34000}"
NOBLE_WSS_PORT="${NOBLE_WSS_PORT:-34001}"
NOBLE_PPROF_PORT="${NOBLE_PPROF_PORT:-34002}"

LOCAL_CERT_DIR="${LOCAL_CERT_DIR:-$ROOT_DIR/benchmark/certs/local}"
LOCAL_CERT_FILE="${LOCAL_CERT_FILE:-$LOCAL_CERT_DIR/lima-local-cert.pem}"
LOCAL_KEY_FILE="${LOCAL_KEY_FILE:-$LOCAL_CERT_DIR/lima-local-key.pem}"

ensure_prereqs() {
  if ! command -v "$LIMACTL" >/dev/null 2>&1; then
    echo "limactl not found; install Lima first (brew install lima)" >&2
    exit 127
  fi
}

ensure_local_cert() {
  mkdir -p "$LOCAL_CERT_DIR"
  if [[ -f "$LOCAL_CERT_FILE" && -f "$LOCAL_KEY_FILE" ]]; then
    return
  fi

  CERT_FILE="$LOCAL_CERT_FILE" \
  KEY_FILE="$LOCAL_KEY_FILE" \
  SAN_DNS="localhost,host.docker.internal" \
  SAN_IPS="127.0.0.1,::1" \
  CERT_CN="localhost" \
    bash "$ROOT_DIR/benchmark/certs/buildssl.sh"
}

start_vm() {
  local name="$1"
  local config="$2"
  local ws_port="$3"
  local wss_port="$4"
  local pprof_port="$5"

  if [[ -f "$LIMA_HOME/$name/lima.yaml" ]]; then
    "$LIMACTL" start "$name"
    return
  fi

  "$LIMACTL" start \
    --tty=false \
    --name "$name" \
    --cpus "$VM_CPUS" \
    --memory "$VM_MEMORY" \
    --disk "$VM_DISK" \
    --mount "$ROOT_DIR:w" \
    --port-forward "${ws_port}:30000,static=true" \
    --port-forward "${wss_port}:30001,static=true" \
    --port-forward "${pprof_port}:30002,static=true" \
    "$config"
}

print_vm_summary() {
  local name="$1"
  local ws_port="$2"
  local wss_port="$3"
  local kernel

  kernel=$("$LIMACTL" shell --start "$name" bash -lc "uname -r")
  echo "[$name] kernel=$kernel ws=127.0.0.1:${ws_port} wss=127.0.0.1:${wss_port}"
}

ensure_prereqs
ensure_local_cert
bash "$ROOT_DIR/scripts/lima/build_linux_artifacts.sh"

start_vm "$JAMMY_VM_NAME" "$ROOT_DIR/scripts/lima/ubuntu-2204.yaml" "$JAMMY_WS_PORT" "$JAMMY_WSS_PORT" "$JAMMY_PPROF_PORT"
start_vm "$NOBLE_VM_NAME" "$ROOT_DIR/scripts/lima/ubuntu-2404.yaml" "$NOBLE_WS_PORT" "$NOBLE_WSS_PORT" "$NOBLE_PPROF_PORT"

print_vm_summary "$JAMMY_VM_NAME" "$JAMMY_WS_PORT" "$JAMMY_WSS_PORT"
print_vm_summary "$NOBLE_VM_NAME" "$NOBLE_WS_PORT" "$NOBLE_WSS_PORT"

cat <<EOF
local cert: $LOCAL_CERT_FILE
local key:  $LOCAL_KEY_FILE
linux bin:  $ROOT_DIR/benchmark/linux-lab/bin
EOF
