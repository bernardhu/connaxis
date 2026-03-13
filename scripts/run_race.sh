#!/usr/bin/env bash
set -euo pipefail

# Run race detector on the main module.
# Use GOOS/GOARCH from environment if provided by CI.
#
# NOTE:
# A few command-only packages under benchmark/examples can fail under
# `go test -race` with "cannot find package" in some toolchain setups.
# Exclude them and focus race checks on core library/server paths.
exclude_regex='/(benchmark|examples)/|/cmd/'
race_pkgs="$(go list ./... | grep -Ev "${exclude_regex}" | tr '\n' ' ')"

go test -race ${race_pkgs}
