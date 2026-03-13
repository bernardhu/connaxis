#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 5 ]]; then
  cat <<'EOF'
Usage: scripts/benchstat_compare.sh <baseline-ref> <target-ref> [package] [bench-regex] [count]

Examples:
  scripts/benchstat_compare.sh main HEAD ./websocket . 5
  scripts/benchstat_compare.sh v1.2.0 my-branch ./websocket 'BenchmarkProcess.*' 10
EOF
  exit 2
fi

baseline_ref="$1"
target_ref="$2"
pkg="${3:-./websocket}"
bench_regex="${4:-.}"
count="${5:-5}"

if ! command -v benchstat >/dev/null 2>&1; then
  echo "benchstat not found, installing golang.org/x/perf/cmd/benchstat@latest..." >&2
  GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}" go install golang.org/x/perf/cmd/benchstat@latest
  gobin="$(go env GOBIN)"
  if [[ -z "${gobin}" ]]; then
    gobin="$(go env GOPATH)/bin"
  fi
  export PATH="${gobin}:${PATH}"
fi

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/connaxis-benchstat.XXXXXX")"
export GOCACHE="${GOCACHE:-${tmpdir}/gocache}"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

git clone --quiet . "${tmpdir}/baseline"
git clone --quiet . "${tmpdir}/target"
git -C "${tmpdir}/baseline" checkout --quiet --detach "${baseline_ref}"
git -C "${tmpdir}/target" checkout --quiet --detach "${target_ref}"

run_bench() {
  local dir="$1"
  local out="$2"
  (
    cd "${dir}"
    go test -run '^$' -bench "${bench_regex}" -benchmem -count "${count}" "${pkg}"
  ) | tee "${out}"
}

run_bench "${tmpdir}/baseline" "${tmpdir}/baseline.bench"
run_bench "${tmpdir}/target" "${tmpdir}/target.bench"

echo
benchstat "${tmpdir}/baseline.bench" "${tmpdir}/target.bench"
