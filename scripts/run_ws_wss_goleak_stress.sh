#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Stress ws/wss path and let goleak verify package-level goroutine cleanup.
go test -tags='goleak,stress' -count=1 -run '^TestConnaxis(WS|WSS)StressEcho$' ./websocket
