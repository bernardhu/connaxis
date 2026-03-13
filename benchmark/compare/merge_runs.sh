#!/usr/bin/env bash
set -euo pipefail

# Merge multiple run directories into one combined results directory.
# Usage: ./merge_runs.sh <out_dir> <run_dir1> [run_dir2 ...]

OUT_DIR="${1:?out_dir required}"
shift

if [[ "$#" -lt 1 ]]; then
  echo "at least one run_dir required" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

first=1
for d in "$@"; do
  if [[ ! -d "$d" ]]; then
    echo "skip missing dir: $d" >&2
    continue
  fi

  if [[ -f "$d/env.json" && ! -f "$OUT_DIR/env.json" ]]; then
    cp "$d/env.json" "$OUT_DIR/env.json"
  fi

  # merge results_all.csv if present, otherwise merge results_*.csv
  if [[ -f "$d/results_all.csv" ]]; then
    if [[ $first -eq 1 ]]; then
      cat "$d/results_all.csv" > "$OUT_DIR/results_all.csv"
      first=0
    else
      tail -n +2 "$d/results_all.csv" >> "$OUT_DIR/results_all.csv"
    fi
  else
    for f in "$d"/results_*.csv; do
      [[ -f "$f" ]] || continue
      if [[ $first -eq 1 ]]; then
        cat "$f" > "$OUT_DIR/results_all.csv"
        first=0
      else
        tail -n +2 "$f" >> "$OUT_DIR/results_all.csv"
      fi
    done
  fi

  # copy stats and raw logs (best effort)
  cp -f "$d"/*_cpu_rss.csv "$OUT_DIR/" 2>/dev/null || true
  cp -f "$d"/*.txt "$OUT_DIR/" 2>/dev/null || true
done

echo "Wrote $OUT_DIR/results_all.csv"
