# Roadmap

This roadmap is meant to make the project's priorities legible to contributors.
It separates committed near-term work from experiments that may or may not become part of the mainline runtime.

## Mainline Priorities

These are the areas the project expects to keep moving forward in the near term.

### 1. Public API Boundary and Smaller Interface Surface

Keep the supported public surface centered on `connaxis`, `connection`, `eventloop`, `evhandler`, and `websocket`, while shrinking the interface set to the smallest surface that still covers the required use cases.

Done looks like:

- README, examples, and package docs point to the same supported entry points
- public behavior is documented without asking users to understand internal wiring
- smaller interfaces cover the required handler and protocol integration paths without widening the API unnecessarily
- examples and config docs stay aligned with current behavior

Good contribution shapes:

- docs and example corrections
- package doc improvements
- narrow API simplifications that reduce surface area without dropping supported behavior

### 2. BoringSSL / TLS Fuzzer Validation and Compatibility

Improve confidence in protocol handling through reproducible validation against suites such as `bogo` and `tlsfuzzer`, along with targeted regression coverage where needed.

Done looks like:

- `bogo`, `tlsfuzzer`, and related TLS validation workflows remain runnable and relevant
- known handshake and close-path edge cases are covered by focused tests where suites do not already cover them
- compatibility gaps are tracked by tests or explicit notes

Good contribution shapes:

- validation script improvements around `bogo`, `tlsfuzzer`, and related TLS workflows
- compatibility notes and reproducibility fixes
- focused tests for protocol edge cases that fall outside external suite coverage

### 3. TLS / kTLS Validation Across Linux Kernels

The main support target is Linux. For macOS and BSD, the goal is basic end-to-end path validation rather than feature parity with Linux.

Done looks like:

- TLS / kTLS behavior is tested on a wider set of Linux kernel versions
- kernel-specific compatibility gaps are tracked by tests or explicit notes
- non-Linux paths remain runnable for basic validation without expanding the main support scope

Good contribution shapes:

- Linux kernel matrix validation
- docs for kernel constraints, limits, and expected behavior
- reproducible notes for what is and is not expected to work on non-Linux platforms

### 4. Validation and Benchmark Reproducibility

Make it easier to reproduce the project's quality and performance claims on a clean environment.

Done looks like:

- benchmark and validation scripts are documented with realistic prerequisites
- Linux-specific test lab and TLS workflows remain reproducible
- performance reports are generated from understandable, repeatable inputs

Good contribution shapes:

- benchmark script cleanup
- reproducibility fixes
- report and methodology doc improvements

## Experimental Directions

These are worth exploring, but they are not current mainline commitments.
Changes in these areas should start as experiments, prototypes, or isolated branches before they are proposed as production runtime changes.

### `io_uring` backend exploration

Goal:

- evaluate whether a Linux `io_uring` path can provide a clear benefit over the current poll-based runtime

Exit criteria:

- measurable gain on target workloads
- complexity remains understandable and reviewable
- failure handling and ownership stay explicit

### Arena-style allocation experiments

Goal:

- test whether narrow, short-lived allocation paths benefit from arena-style memory management

Exit criteria:

- reduced allocation overhead on measured hot paths
- object ownership and reclamation remain obvious by inspection
- no broad lifetime coupling is introduced into the runtime

### Rust prototypes or comparison implementations

Goal:

- use Rust as a prototype or comparison tool for high-risk experiments, not as a default rewrite direction

Exit criteria:

- the prototype answers a concrete runtime or performance question
- build and debugging complexity stay isolated from the main Go codebase

## Out of Scope for Near-Term Mainline Work

These are not good default contribution targets right now:

- large abstraction-heavy refactors without a measured correctness or performance benefit
- widening the public API when an internal change is sufficient
- broad runtime rewrites that mix experimentation with production changes
- cross-language rewrites of core paths without a narrow experimental goal

## How to Contribute Effectively

Strong contributions usually have these properties:

- one clear behavior change per PR
- tests for correctness changes
- benchmark notes for performance-sensitive changes
- doc updates when public behavior, config, or examples change

For large changes involving loop ownership, listener behavior, TLS / kTLS data paths, or API expansion, start with a design discussion or a small preparatory PR.
