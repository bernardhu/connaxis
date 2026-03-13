# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Changed
- Module path updated to `github.com/bernardhu/connaxis` for public GitHub release preparation.
- Benchmark example programs now use a repository-local example logger instead of private internal logger dependencies.
- Benchmark/TLS suite docs use public-safe placeholder hosts/IPs (`example.test`, `127.0.0.1`, `192.0.2.10`).

### Fixed
- Round-robin load balancer no longer returns nil loops.
- Config loading returns proper errors on invalid JSON.

### Added
- CI workflow for `go test` and `go vet`.
- Examples for echo/TLS/HTTP/WebSocket.
- Documentation for configuration, constraints, metrics, and benchmarks.
- `SECURITY.md` and `CODE_OF_CONDUCT.md` for public repository governance.
