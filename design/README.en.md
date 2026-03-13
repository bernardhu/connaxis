# Connaxis Design Docs Overview (English)

This document set explains `connaxis` core architecture, engineering invariants, performance/validation methodology, kTLS roadmap, and comparisons with similar frameworks. It is intended for:

- architects/platform engineers evaluating `connaxis` for gateway/proxy workloads
- backend engineers planning secondary development on top of `connaxis`
- maintainers who need to understand performance tradeoffs, invariants, and runtime protection behavior

Boundary note:

- public package surface is documented in `docs/API_BOUNDARY.md`
- listener/dial/server wiring and packages under `internal/` should be treated as implementation detail, not stable architecture modules for external consumers

## Recommended Reading Order

1. [`design/architecture_diagram.en.md`](./architecture_diagram.en.md)
   - Start with the component layout, main/sub reactors, and memory pool placement
2. [`design/design.en.md`](./design.en.md)
   - Understand implementation strategies behind performance goals (zero-copy, write queue, flow control, overload protection)
3. [`design/atls_runtime.en.md`](./atls_runtime.en.md)
   - Understand how the userspace async TLS path bridges `crypto/tls` into the non-blocking event loop
4. [`design/constraints.en.md`](./constraints.en.md)
   - Learn the caller contracts and high-performance-path invariants (especially memory ownership and `AddCmd`)
5. [`design/comparison.en.md`](./comparison.en.md)
   - See positioning differences and engineering tradeoffs vs `tidwall/evio`, `gnet`, and `netpoll`
6. [`design/performance_methodology.en.md`](./performance_methodology.en.md)
   - Review publishing rules, comparison methodology, and report templates for performance/validation results
7. [`design/quality_and_benchmark_pipeline.en.md`](./quality_and_benchmark_pipeline.en.md)
   - Review CI/local quality gates, goleak package coverage, and benchstat workflow
8. [`design/ktls_status_and_roadmap.en.md`](./ktls_status_and_roadmap.en.md)
   - Review kTLS current status, risks, and future evolution directions

## Document Map

- **Architecture Diagram**: [`design/architecture_diagram.en.md`](./architecture_diagram.en.md)
- **Core Design Details**: [`design/design.en.md`](./design.en.md)
- **aTLS Runtime Design**: [`design/atls_runtime.en.md`](./atls_runtime.en.md)
- **Runtime and Caller Invariants**: [`design/constraints.en.md`](./constraints.en.md)
- **Framework Comparison**: [`design/comparison.en.md`](./comparison.en.md)
- **External Pitch Comparison Summary**: [`design/comparison_pitch.en.md`](./comparison_pitch.en.md)
- **Performance and Validation Methodology**: [`design/performance_methodology.en.md`](./performance_methodology.en.md)
- **Quality and Benchmark Pipeline**: [`design/quality_and_benchmark_pipeline.en.md`](./quality_and_benchmark_pipeline.en.md)
- **kTLS Status and Roadmap**: [`design/ktls_status_and_roadmap.en.md`](./ktls_status_and_roadmap.en.md)

## Reading Notes

- The docs focus on engineering capabilities, invariants, and boundary conditions rather than single benchmark numbers.
- kTLS-related sections include Linux-specific paths and should be interpreted together with kernel version, cipher suites, and runtime environment.
- Code locations (file/line references) may drift over time; the current repository source is authoritative.
