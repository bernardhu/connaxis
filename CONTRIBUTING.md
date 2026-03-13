# Contributing

Thanks for contributing!

## Start Here

For a first contribution, prefer one of these:

- docs or example corrections
- focused tests for existing behavior
- small validation or benchmark script improvements

Read these before changing public behavior or core runtime code:

- `README.md`
- `ROADMAP.md`
- `docs/API_BOUNDARY.md`
- `design/constraints.en.md`

If you want to change loop ownership, listener behavior, backpressure semantics, or TLS/kTLS data paths, open a design discussion or start with a small preparatory PR.

## Development Setup

- Go 1.24.2 required.
- Module path: `github.com/bernardhu/connaxis`
- Run validation before submitting changes:

```sh
go test ./...
go vet ./...
```

Optional but useful for concurrency-sensitive changes:

```sh
./scripts/run_race.sh
./scripts/run_goleak_pilot.sh
```

## Code Style

- Keep changes focused and small.
- Add tests for bug fixes and new features.
- Prefer ASCII in source files unless required.
- Prefer the smallest correct implementation over extra abstraction.
- Do not widen public APIs unless the caller clearly needs it.
- Keep backpressure and ownership semantics explicit.

## Project Policies

- Follow `CODE_OF_CONDUCT.md` for community interactions.
- Follow `SECURITY.md` for vulnerability reporting (do not file public issues for security bugs).

## Pull Requests

- Describe the change and why it is needed.
- Include benchmark results for performance-sensitive changes.
- Update docs/examples when public behavior or APIs change.
- Keep one clear behavior change per PR when possible.
- Call out any runtime model, ownership, or compatibility tradeoffs explicitly.
