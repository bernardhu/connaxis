# TLS Test Matrix

## Goal

Provide a release-facing TLS validation matrix that is separate from throughput benchmarks and clearly split into default baseline vs extended checks.

## Matrix

| Tier | Tool | What it validates | Default | Gate |
|---|---|---|---|---|
| P0 | `openssl s_client` + HTTP Upgrade probe | handshake availability, negotiated version/cipher, basic WSS upgrade | yes | must pass |
| P0 | `testssl.sh` | protocol/cipher exposure and common misconfiguration | yes | pass or explicitly accepted warning |
| P1 | `tlsanvil` | stronger protocol behavior validation for a running TLS service | yes | must pass |
| P2 | `tlsfuzzer` | negative-input, alert, and edge-case probing | no | tracked separately |
| P2 | BoGo | structured interoperability and negative-behavior checks | no | tracked separately |
| P0 (kTLS claims) | `cmd/ktlscheck` | Linux kTLS availability + data path activation | separate | must pass on kTLS target |

## Implemented Entry Points

- `benchmark/tls-suite/run_tls_suite.sh`
- `benchmark/tls-suite/run_tls_smoke.sh`
- `benchmark/tls-suite/run_testssl.sh`
- `benchmark/tls-suite/run_tlsanvil.sh`
- `benchmark/tls-suite/run_tlsfuzzer.sh`
- `benchmark/tls-suite/run_bogo.sh`
- `cmd/ktlscheck` (Linux only)
- `scripts/lima/run_tls_matrix.sh`

## Default Lima Matrix

Default scenarios:

- `atls-tls12`
- `atls-tls13`
- `ktls-tls12-tx`
- `ktls-tls12-rxtx`
- `ktls-tls13-tx`
- `ktls-tls13-rxtx`

Default slots:

- `smoke`
- `testssl`
- `tlsanvil`

## Extended Checks

- `tlsfuzzer` and BoGo remain available through `run_tls_suite.sh`
- they are not interpreted as part of the default delivery baseline
- use them when comparing TLS-only behavior, negative input handling, or historical extended reports

## Notes

- TLS suite scripts are shell wrappers around command hooks
- WS protocol correctness remains gated by Autobahn in `benchmark/ws-autobahn`
- `ktls` claims still require Linux-side verification with `cmd/ktlscheck` and Linux lab runs
