#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORTS_DIR="$ROOT_DIR/reports"

latest_dir() {
  local pattern="$1"
  find "$REPORTS_DIR" -maxdepth 1 -type d -name "$pattern" -print 2>/dev/null | sort -r | head -n 1 || true
}

WS_DIR="${1:-$(latest_dir 'ws_full_*')}"
WSS_DIR="${2:-$(latest_dir 'wss_full_*')}"
OUT_FILE="${3:-$REPORTS_DIR/WS_WSS_AUTOBAHN_REPORT.md}"

if [[ -z "$WS_DIR" || ! -d "$WS_DIR" ]]; then
  echo "ws report dir not found: $WS_DIR" >&2
  exit 1
fi
if [[ -z "$WSS_DIR" || ! -d "$WSS_DIR" ]]; then
  echo "wss report dir not found: $WSS_DIR" >&2
  exit 1
fi
if [[ ! -f "$WS_DIR/servers/index.json" || ! -f "$WS_DIR/run_meta.json" ]]; then
  echo "invalid ws report: $WS_DIR" >&2
  exit 1
fi
if [[ ! -f "$WSS_DIR/servers/index.json" || ! -f "$WSS_DIR/run_meta.json" ]]; then
  echo "invalid wss report: $WSS_DIR" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq not found in PATH" >&2
  exit 127
fi

ws_total="$(jq -r '.connaxis | length' "$WS_DIR/servers/index.json")"
wss_total="$(jq -r '.connaxis | length' "$WSS_DIR/servers/index.json")"

ws_failed="$(jq -r '[.connaxis[] | select((.behavior // "UNKNOWN") == "FAILED")] | length' "$WS_DIR/servers/index.json")"
wss_failed="$(jq -r '[.connaxis[] | select((.behavior // "UNKNOWN") == "FAILED")] | length' "$WSS_DIR/servers/index.json")"
ws_unimplemented="$(jq -r '[.connaxis[] | select((.behavior // "UNKNOWN") == "UNIMPLEMENTED")] | length' "$WS_DIR/servers/index.json")"
wss_unimplemented="$(jq -r '[.connaxis[] | select((.behavior // "UNKNOWN") == "UNIMPLEMENTED")] | length' "$WSS_DIR/servers/index.json")"

ws_gate="PASS"
wss_gate="PASS"
overall_gate="PASS"
if [[ "$ws_failed" -gt 0 || "$ws_unimplemented" -gt 0 ]]; then
  ws_gate="FAIL"
fi
if [[ "$wss_failed" -gt 0 || "$wss_unimplemented" -gt 0 ]]; then
  wss_gate="FAIL"
fi
if [[ "$ws_gate" != "PASS" || "$wss_gate" != "PASS" ]]; then
  overall_gate="FAIL"
fi

ws_target="$(jq -r '.target_url // ""' "$WS_DIR/run_meta.json")"
wss_target="$(jq -r '.target_url // ""' "$WSS_DIR/run_meta.json")"
ws_started="$(jq -r '.run_started_at // ""' "$WS_DIR/run_meta.json")"
ws_finished="$(jq -r '.run_finished_at // ""' "$WS_DIR/run_meta.json")"
wss_started="$(jq -r '.run_started_at // ""' "$WSS_DIR/run_meta.json")"
wss_finished="$(jq -r '.run_finished_at // ""' "$WSS_DIR/run_meta.json")"

ws_behavior_rows="$(
  jq -r '
    .connaxis
    | to_entries
    | map(.value.behavior // "UNKNOWN")
    | group_by(.)
    | map({k: .[0], v: length})
    | sort_by(.k)
    | .[]
    | "| " + .k + " | " + (.v|tostring) + " |"
  ' "$WS_DIR/servers/index.json"
)"
wss_behavior_rows="$(
  jq -r '
    .connaxis
    | to_entries
    | map(.value.behavior // "UNKNOWN")
    | group_by(.)
    | map({k: .[0], v: length})
    | sort_by(.k)
    | .[]
    | "| " + .k + " | " + (.v|tostring) + " |"
  ' "$WSS_DIR/servers/index.json"
)"

wss_failed_rows="$(
  jq -r '
    .connaxis
    | to_entries
    | map(select((.value.behavior // "UNKNOWN") == "FAILED"))
    | sort_by(.key | split(".") | map(tonumber))
    | .[]
    | "| " + .key + " | " + (.value.behaviorClose // "UNKNOWN") + " | " + ((.value.duration // "")|tostring) + " |"
  ' "$WSS_DIR/servers/index.json"
)"

mkdir -p "$(dirname "$OUT_FILE")"
{
  echo "# WS/WSS Autobahn Report"
  echo
  echo "- Generated at: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "- WS report dir: \`$WS_DIR\`"
  echo "- WSS report dir: \`$WSS_DIR\`"
  echo
  echo "## Gate Result"
  echo
  echo "- WS gate (FAILED=0 and UNIMPLEMENTED=0): \`$ws_gate\`"
  echo "- WSS gate (FAILED=0 and UNIMPLEMENTED=0): \`$wss_gate\`"
  echo "- Overall gate: \`$overall_gate\`"
  echo
  if [[ "$overall_gate" == "PASS" ]]; then
    echo "Conclusion: WS/WSS Autobahn gate passed. You can claim WS/WSS support in docs."
  else
    echo "Conclusion: WS/WSS Autobahn gate not passed yet. Do not claim full WS/WSS support in docs."
  fi
  echo
  echo "## WS Summary"
  echo
  echo "- Target: \`$ws_target\`"
  echo "- Run window: \`$ws_started\` -> \`$ws_finished\`"
  echo "- Total cases: \`$ws_total\`"
  echo
  echo "| Behavior | Count |"
  echo "|---|---:|"
  echo "$ws_behavior_rows"
  echo
  echo "## WSS Summary"
  echo
  echo "- Target: \`$wss_target\`"
  echo "- Run window: \`$wss_started\` -> \`$wss_finished\`"
  echo "- Total cases: \`$wss_total\`"
  echo
  echo "| Behavior | Count |"
  echo "|---|---:|"
  echo "$wss_behavior_rows"
  echo
  echo "## WSS FAILED Cases"
  echo
  if [[ -n "$wss_failed_rows" ]]; then
    echo "| Case | behaviorClose | duration |"
    echo "|---|---|---:|"
    echo "$wss_failed_rows"
  else
    echo "No FAILED case."
  fi
} > "$OUT_FILE"

echo "report generated: $OUT_FILE"
echo "overall_gate: $overall_gate"
