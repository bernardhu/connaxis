#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/benchmark/linux-lab/bin}"
GOOS_TARGET="${GOOS_TARGET:-linux}"
GOARCH_TARGET="${GOARCH_TARGET:-arm64}"

mkdir -p "$OUT_DIR"

echo "[linux-artifacts] building connaxis_ws_server for ${GOOS_TARGET}/${GOARCH_TARGET}"
CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
  go build -o "$OUT_DIR/connaxis_ws_server" ./benchmark/ws-autobahn/connaxis_ws_server

echo "[linux-artifacts] building ktlscheck for ${GOOS_TARGET}/${GOARCH_TARGET}"
CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
  go build -o "$OUT_DIR/ktlscheck" ./cmd/ktlscheck

echo "[linux-artifacts] wrote $OUT_DIR/connaxis_ws_server"
echo "[linux-artifacts] wrote $OUT_DIR/ktlscheck"
