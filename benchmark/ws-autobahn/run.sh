#!/usr/bin/env bash
set -euo pipefail

# Run Autobahn fuzzingclient against an connaxis WS/WSS endpoint.
#
# Example (WS):
#   TARGET_URL=ws://127.0.0.1:30000 ./run.sh
#
# Example (WSS):
#   TARGET_URL=wss://127.0.0.1:30000 ./run.sh

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODE="${MODE:-fuzzingclient}"
TARGET_URL="${TARGET_URL:-ws://127.0.0.1:30000}"
AGENT="${AGENT:-connaxis}"
AUTOBANH_IMAGE="${AUTOBANH_IMAGE:-crossbario/autobahn-testsuite}"
REPORTS_DIR="${REPORTS_DIR:-$ROOT_DIR/reports/$(date +%Y%m%d_%H%M%S)}"
CASES_JSON="${CASES_JSON:-[\"*\"]}"
EXCLUDE_CASES_JSON="${EXCLUDE_CASES_JSON:-[]}"
SERVER_TLS_ENGINE="${SERVER_TLS_ENGINE:-}"
SERVER_OPTIONS_JSON="${SERVER_OPTIONS_JSON-}"
if [[ -z "$SERVER_OPTIONS_JSON" ]]; then
  SERVER_OPTIONS_JSON='{"version":18}'
fi
DOCKER_USE_HOST_NETWORK="${DOCKER_USE_HOST_NETWORK:-0}"
DOCKER_ADD_HOSTS="${DOCKER_ADD_HOSTS:-}"

if [[ "$MODE" != "fuzzingclient" ]]; then
  echo "only MODE=fuzzingclient is supported by this script" >&2
  exit 2
fi

if [[ ! "$TARGET_URL" =~ ^wss?:// ]]; then
  echo "TARGET_URL must start with ws:// or wss:// (got: $TARGET_URL)" >&2
  exit 2
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not found in PATH" >&2
  exit 127
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq not found in PATH" >&2
  exit 127
fi

mkdir -p "$REPORTS_DIR"
RUN_META_FILE="$REPORTS_DIR/run_meta.json"
RUN_STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/autobahn.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT
CONFIG_FILE="$TMP_DIR/fuzzingclient.json"

DOCKER_TARGET_URL="$TARGET_URL"
DOCKER_EXTRA_ARGS=()
DOCKER_NETWORK_MODE="bridge"
if [[ "$DOCKER_USE_HOST_NETWORK" == "1" ]]; then
  DOCKER_EXTRA_ARGS+=(--network host)
  DOCKER_NETWORK_MODE="host"
else
  url_scheme="${TARGET_URL%%://*}"
  url_rest="${TARGET_URL#*://}"
  url_authority="${url_rest%%/*}"
  if [[ "$url_rest" == "$url_authority" ]]; then
    url_suffix=""
  else
    url_suffix="/${url_rest#*/}"
  fi

  url_host=""
  url_port=""
  if [[ "$url_authority" == \[*\]* ]]; then
    url_host="${url_authority#\[}"
    url_host="${url_host%%\]*}"
    authority_tail="${url_authority#*\]}"
    if [[ "$authority_tail" == :* ]]; then
      url_port="${authority_tail#:}"
    fi
  else
    if [[ "$url_authority" == *:* ]]; then
      url_host="${url_authority%%:*}"
      url_port="${url_authority##*:}"
    else
      url_host="$url_authority"
    fi
  fi

  if [[ -z "$url_port" ]]; then
    if [[ "$url_scheme" == "wss" ]]; then
      url_port="443"
    else
      url_port="80"
    fi
  fi

  if [[ "$url_host" == "127.0.0.1" || "$url_host" == "localhost" || "$url_host" == "::1" ]]; then
    DOCKER_TARGET_URL="${url_scheme}://host.docker.internal:${url_port}${url_suffix}"
    if [[ "$(uname -s)" == "Linux" ]]; then
      DOCKER_EXTRA_ARGS+=(--add-host=host.docker.internal:host-gateway)
    fi
  fi
fi

if [[ -n "$DOCKER_ADD_HOSTS" ]]; then
  IFS=',' read -r -a add_host_items <<<"$DOCKER_ADD_HOSTS"
  for raw in "${add_host_items[@]}"; do
    item="${raw#"${raw%%[![:space:]]*}"}"
    item="${item%"${item##*[![:space:]]}"}"
    if [[ -n "$item" ]]; then
      DOCKER_EXTRA_ARGS+=(--add-host "$item")
    fi
  done
fi

if ! jq -e . >/dev/null 2>&1 <<<"$CASES_JSON"; then
  echo "invalid CASES_JSON: $CASES_JSON" >&2
  exit 2
fi
if ! jq -e . >/dev/null 2>&1 <<<"$EXCLUDE_CASES_JSON"; then
  echo "invalid EXCLUDE_CASES_JSON: $EXCLUDE_CASES_JSON" >&2
  exit 2
fi
if ! jq -e . >/dev/null 2>&1 <<<"$SERVER_OPTIONS_JSON"; then
  echo "invalid SERVER_OPTIONS_JSON: $SERVER_OPTIONS_JSON" >&2
  exit 2
fi

jq -n \
  --arg agent "$AGENT" \
  --arg target_url "$DOCKER_TARGET_URL" \
  --argjson cases "$CASES_JSON" \
  --argjson exclude_cases "$EXCLUDE_CASES_JSON" \
  --argjson server_options "$SERVER_OPTIONS_JSON" \
  '
  {
    options: {failByDrop: false},
    outdir: "./reports/servers",
    servers: [
      {
        agent: $agent,
        url: $target_url,
        options: $server_options
      }
    ],
    cases: $cases,
    "exclude-cases": $exclude_cases,
    "exclude-agent-cases": {}
  }
  ' > "$CONFIG_FILE"

CONTAINER_NAME="autobahn_$(date +%s)"
docker_cmd=(
  docker run --rm
  -v "$CONFIG_FILE:/config/fuzzingclient.json:ro"
  -v "$REPORTS_DIR:/reports"
  --name "$CONTAINER_NAME"
)
if [[ "${#DOCKER_EXTRA_ARGS[@]}" -gt 0 ]]; then
  docker_cmd+=("${DOCKER_EXTRA_ARGS[@]}")
fi
docker_cmd+=(
  "$AUTOBANH_IMAGE"
  wstest -s /config/fuzzingclient.json -m "$MODE"
)
"${docker_cmd[@]}"

jq -n \
  --arg run_started_at "$RUN_STARTED_AT" \
  --arg run_finished_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg mode "$MODE" \
  --arg agent "$AGENT" \
  --arg autobahn_image "$AUTOBANH_IMAGE" \
  --arg target_url "$TARGET_URL" \
  --arg docker_target_url "$DOCKER_TARGET_URL" \
  --arg docker_network_mode "$DOCKER_NETWORK_MODE" \
  --argjson docker_use_host_network "$DOCKER_USE_HOST_NETWORK" \
  --arg docker_add_hosts "$DOCKER_ADD_HOSTS" \
  --arg container_name "$CONTAINER_NAME" \
  --arg reports_dir "$REPORTS_DIR" \
  --arg run_meta_file "$RUN_META_FILE" \
  --arg server_tls_engine "$SERVER_TLS_ENGINE" \
  --argjson cases "$CASES_JSON" \
  --argjson exclude_cases "$EXCLUDE_CASES_JSON" \
  --argjson server_options "$SERVER_OPTIONS_JSON" \
  '
  {
    run_started_at: $run_started_at,
    run_finished_at: $run_finished_at,
    mode: $mode,
    agent: $agent,
    autobahn_image: $autobahn_image,
    target_url: $target_url,
    docker_target_url: $docker_target_url,
    docker_target_rewritten: ($target_url != $docker_target_url),
    docker_network_mode: $docker_network_mode,
    docker_use_host_network: $docker_use_host_network,
    docker_add_hosts: (if $docker_add_hosts == "" then [] else ($docker_add_hosts | split(",")) end),
    container_name: $container_name,
    cases: $cases,
    exclude_cases: $exclude_cases,
    server_options: $server_options,
    server_tls_engine: (if $server_tls_engine == "" then null else $server_tls_engine end),
    reports_dir: $reports_dir,
    run_meta_file: $run_meta_file
  }
  ' > "$RUN_META_FILE"

echo "Autobahn report: $REPORTS_DIR/servers/index.html"
echo "Run metadata: $RUN_META_FILE"
