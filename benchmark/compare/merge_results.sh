#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${1:-$ROOT_DIR/results}"
OUT="$RESULTS_DIR/results_all.csv"

first=1
for f in "$RESULTS_DIR"/results_*.csv; do
  [[ -f "$f" ]] || continue
  if [[ $first -eq 1 ]]; then
    cat "$f" > "$OUT"
    first=0
  else
    tail -n +2 "$f" >> "$OUT"
  fi
 done

echo "Wrote $OUT"
