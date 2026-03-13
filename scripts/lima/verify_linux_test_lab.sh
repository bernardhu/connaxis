#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LIMACTL="${LIMACTL:-limactl}"
JAMMY_VM_NAME="${JAMMY_VM_NAME:-ub2204-k515}"
NOBLE_VM_NAME="${NOBLE_VM_NAME:-ub2404-k61plus}"

verify_vm() {
  local name="$1"

  echo "[verify] $name kernel"
  "$LIMACTL" shell --start "$name" bash -lc 'uname -a'
  echo "[verify] $name repo mount"
  "$LIMACTL" shell --start "$name" bash -lc "test -d '$ROOT_DIR' && echo '$ROOT_DIR mounted'"
  echo "[verify] $name ktlscheck"
  "$LIMACTL" shell --start "$name" bash -lc "'$ROOT_DIR/benchmark/linux-lab/bin/ktlscheck' -bench=false"
}

bash "$ROOT_DIR/scripts/lima/build_linux_artifacts.sh"
verify_vm "$JAMMY_VM_NAME"
verify_vm "$NOBLE_VM_NAME"
