# Quality and Benchmark Pipeline

This document describes the repository-level quality gates and benchmark comparison flow for `connaxis`, covering both CI and local execution paths.

## 1. Goals
- Keep a sustainable Linux-focused baseline quality gate (lint / test / race / goleak).
- Standardize script entry points to reduce local reproduction and CI debugging cost.
- Provide a repeatable benchmark comparison workflow for performance regressions (benchstat).

## 2. Entry Points and Responsibilities
- Main CI workflow: `.github/workflows/go.yml`
  - Runs `lint`, `go test`, `go test -race`, and `goleak`.
- Benchmark comparison workflow: `.github/workflows/benchstat.yml`
  - Manually triggered via `workflow_dispatch` for baseline vs target comparisons.
- Quality aggregator script: `scripts/run_quality_pipeline.sh`
  - Chains lint/test/race/goleak/benchstat steps locally and emits a summary.
- Single-purpose scripts:
  - race: `scripts/run_race.sh`
  - goleak: `scripts/run_goleak_pilot.sh`
  - benchstat: `scripts/benchstat_compare.sh`

## 3. Current Coverage
- [x] Baseline `golangci-lint` + `go test` checks (Linux target)
- [x] Dedicated `go test -race` CI job (`race`)
- [x] `goleak` coverage for `connection` / `evhandler` / `eventloop` / `websocket`
- [x] Manual `benchstat` workflow (`workflow_dispatch`)
- [x] One-shot quality pipeline script: `scripts/run_quality_pipeline.sh`

## 4. Local Execution
- Quick run (default profile):
  - `./scripts/run_quality_pipeline.sh`
- Explicit profile selection:
  - `./scripts/run_quality_pipeline.sh quick`
  - `./scripts/run_quality_pipeline.sh full`
- Run selected steps only (example):
  - `RUN_LINT=0 RUN_BENCHSTAT=0 ./scripts/run_quality_pipeline.sh`
- Run goleak only (explicit `GOCACHE` recommended):
  - `GOCACHE="${TMPDIR:-/tmp}/connaxis-gocache" ./scripts/run_goleak_pilot.sh`
- Run benchstat only (example):
  - `./scripts/benchstat_compare.sh main HEAD ./websocket . 5`

## 5. Output Artifacts and Triage
- The aggregator script outputs to: `benchmark/quality-results/<run_id>/`
  - Key files: `summary.txt`, `logs/*.log`
- Suggested triage order:
  - Check failed steps in `summary.txt`
  - Inspect the corresponding step log
  - Re-run the failing step script in isolation for deeper diagnosis

## 6. Next Steps
- [ ] Add higher-value scenario tests for `connection/eventloop` (beyond baseline unit tests)
- [ ] Stabilize broader `race` package coverage incrementally (avoid all-at-once CI flakiness)
- [ ] Formalize the `full` profile contract and add regression checks
- [ ] Move WS/WSS stress scenarios into optional nightly automation
