#!/usr/bin/env bash
set -euo pipefail

TLSFUZZER_ROOT="${TLSFUZZER_ROOT:-/private/tmp/tlsfuzzer-src}"
BOGO_LOCAL_BORINGSSL_DIR="${BOGO_LOCAL_BORINGSSL_DIR:-/private/tmp/boringssl-src}"

if [[ -x /opt/anaconda3/bin/python3 ]]; then
  PYTHON_BIN_DEFAULT="/opt/anaconda3/bin/python3"
else
  PYTHON_BIN_DEFAULT="$(command -v python3 || true)"
fi
PYTHON_BIN="${PYTHON_BIN:-$PYTHON_BIN_DEFAULT}"

if [[ -z "$PYTHON_BIN" ]]; then
  echo "python3 not found" >&2
  exit 2
fi

if [[ ! -d "$TLSFUZZER_ROOT/.git" ]]; then
  git clone --depth 1 https://github.com/tlsfuzzer/tlsfuzzer.git "$TLSFUZZER_ROOT"
fi

if [[ ! -d "$BOGO_LOCAL_BORINGSSL_DIR/.git" ]]; then
  git clone --depth 1 https://boringssl.googlesource.com/boringssl "$BOGO_LOCAL_BORINGSSL_DIR"
fi

echo "TLSFUZZER_ROOT=$TLSFUZZER_ROOT"
echo "BOGO_LOCAL_BORINGSSL_DIR=$BOGO_LOCAL_BORINGSSL_DIR"
echo "PYTHON_BIN=$PYTHON_BIN"
