#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF' >&2
Usage:
  merge_servers.sh --out <servers_dir> [--agent <name>] <servers_dir_1> [servers_dir_2 ...]
EOF
}

AGENT="connaxis"
OUT_DIR=""
INPUT_DIRS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --agent)
      AGENT="${2:-}"
      shift 2
      ;;
    --out)
      OUT_DIR="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      INPUT_DIRS+=("$1")
      shift
      ;;
  esac
done

if [[ -z "$OUT_DIR" || ${#INPUT_DIRS[@]} -eq 0 ]]; then
  usage
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq not found in PATH" >&2
  exit 127
fi

mkdir -p "$OUT_DIR"

for src in "${INPUT_DIRS[@]}"; do
  shopt -s nullglob
  for path in "$src"/"${AGENT}"_case_*.json "$src"/"${AGENT}"_case_*.html; do
    cp -f "$path" "$OUT_DIR"/
  done
  shopt -u nullglob
done

index_files=()
for src in "${INPUT_DIRS[@]}"; do
  if [[ -f "$src/index.json" ]]; then
    index_files+=("$src/index.json")
  fi
done

if [[ ${#index_files[@]} -eq 0 ]]; then
  echo "no index.json found in input report directories" >&2
  exit 1
fi

jq -s --arg agent "$AGENT" '
  reduce .[] as $doc ({($agent): {}}; .[$agent] += ($doc[$agent] // {}))
' "${index_files[@]}" > "$OUT_DIR/index.json"

total_cases="$(jq -r --arg agent "$AGENT" '.[$agent] | length' "$OUT_DIR/index.json")"
behavior_summary="$(jq -r --arg agent "$AGENT" '.[$agent] | to_entries | map(.value.behavior // "UNKNOWN") | group_by(.) | map({k: .[0], v: length}) | sort_by(-.v) | map("\(.k)=\(.v)") | join(", ")' "$OUT_DIR/index.json")"
behavior_close_summary="$(jq -r --arg agent "$AGENT" '.[$agent] | to_entries | map(.value.behaviorClose // "UNKNOWN") | group_by(.) | map({k: .[0], v: length}) | sort_by(-.v) | map("\(.k)=\(.v)") | join(", ")' "$OUT_DIR/index.json")"

cat > "$OUT_DIR/index.html" <<EOF
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Autobahn Servers Report (Combined)</title>
  <style>
    body { font-family: Segoe UI, Tahoma, Arial, Verdana, sans-serif; background: #f4f4f4; color: #333; }
    h1 { margin: 16px 20px; }
    .block { background: #e0e0e0; padding: 16px; margin: 20px; }
    table { border-collapse: collapse; margin: 20px; width: calc(100% - 40px); background: #fff; }
    th, td { border: 1px solid #aaa; padding: 6px 8px; font-size: 13px; text-align: left; }
    th { background: #f0f0f0; }
    tr.OK { background: #e7f6e7; }
    tr.FAILED { background: #ffe3e3; }
    tr.NON-STRICT { background: #fff2cc; }
    tr.INFORMATIONAL { background: #e6f2ff; }
  </style>
</head>
<body>
  <h1>Autobahn Servers Report (Combined)</h1>
  <div class="block">
    <b>Agent:</b> ${AGENT}<br/>
    <b>Total cases:</b> ${total_cases}<br/>
    <b>Behavior:</b> ${behavior_summary}<br/>
    <b>BehaviorClose:</b> ${behavior_close_summary}
  </div>
  <table>
    <thead>
      <tr>
        <th>case</th>
        <th>behavior</th>
        <th>behaviorClose</th>
        <th>duration(ms)</th>
        <th>remoteCloseCode</th>
        <th>details</th>
      </tr>
    </thead>
    <tbody>
EOF

jq -r --arg agent "$AGENT" '
  .[$agent]
  | to_entries
  | sort_by(.key | split(".") | map(try tonumber catch 0))
  | .[]
  | . as $row
  | ($row.value.reportfile | sub("\\.json$"; ".html")) as $html
  | "<tr class=\"\(($row.value.behavior // "UNKNOWN") | @html)\"><td>\(($row.key // "") | @html)</td><td>\(($row.value.behavior // "") | @html)</td><td>\(($row.value.behaviorClose // "") | @html)</td><td>\(($row.value.duration // "") | tostring | @html)</td><td>\(($row.value.remoteCloseCode // "") | tostring | @html)</td><td><a href=\"\($html | @html)\">\($html | @html)</a></td></tr>"
' "$OUT_DIR/index.json" >> "$OUT_DIR/index.html"

cat >> "$OUT_DIR/index.html" <<'EOF'
    </tbody>
  </table>
</body>
</html>
EOF
