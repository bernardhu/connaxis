#!/usr/bin/env bash
set -euo pipefail

# Runs BoringSSL's BoGo test runner on the remote Linux host (via ssh),
# using the connaxis BoGo shim built from the remote connaxis source tree.
#
# This is meant to be called via benchmark/tls-suite/run_bogo.sh:
#   RUN_BOGO=1 BOGO_CMD='bash ./bogo/run_remote.sh' bash ./run_tls_suite.sh
#
# Inputs (env):
# - OUT_DIR: local output directory (required; provided by run_bogo.sh)
# - BOGO_SSH_HOST: ssh target (default: root@127.0.0.1)
# - BOGO_REMOTE_EVIODIR: remote connaxis repo root (default: /root/go/src/github.com/bernardhu/connaxis)
# - BOGO_REMOTE_BORINGSSL_DIR: remote boringssl checkout (default: /tmp/boringssl)
# - BOGO_REMOTE_GO: remote go binary (default: /usr/local/go/bin/go)
# - BOGO_TLS_ENGINE: connaxis shim engine (default: atls)
# - BOGO_TEST_PATTERN: test glob (default: *-TLS12;*-TLS13;TLS13-*)
# - BOGO_SKIP_PATTERN: skip globs (default: a TLS12/13-only profile + common unsupported features)
# - BOGO_NUM_WORKERS: runner parallelism (default: 4)
# - BOGO_GO_TEST_TIMEOUT: go test timeout (default: 30m)
#
# Outputs (local):
# - $OUT_DIR/bogo.json: JSON output from the runner (if produced)

OUT_DIR="${OUT_DIR:?OUT_DIR is required}"

BOGO_SSH_HOST="${BOGO_SSH_HOST:-root@127.0.0.1}"
BOGO_REMOTE_EVIODIR="${BOGO_REMOTE_EVIODIR:-/root/go/src/github.com/bernardhu/connaxis}"
BOGO_REMOTE_BORINGSSL_DIR="${BOGO_REMOTE_BORINGSSL_DIR:-/tmp/boringssl}"
BOGO_REMOTE_GO="${BOGO_REMOTE_GO:-/usr/local/go/bin/go}"
BOGO_TLS_ENGINE="${BOGO_TLS_ENGINE:-atls}"
BOGO_TEST_PATTERN="${BOGO_TEST_PATTERN:-*-TLS12;*-TLS13;TLS13-*}"
BOGO_SKIP_PATTERN="${BOGO_SKIP_PATTERN:-*-DTLS-*;*-QUIC-*;CertificateSelection-*;ALPS-*;NPN-*;NegotiateALPNAndNPN-*;EarlyData*;TLS13-EarlyData-*;SkipEarlyData-*;CurveTest-*;CustomKeyShares-*;KeyShareWithServerHint-*;DuplicateKeyShares-*;QUICTransportParams-*;*ECH*;CertCompression*;DuplicateCertCompressionExt*;DuplicateExtension*;AllExtensions-*;UnknownExtension-*;UnknownUnencryptedExtension-*;UnofferedExtension-*;ExtraClientEncryptedExtension-*;Send*;ServerNameExtensionServer-TLS-*;TolerateServerNameAck-*;UnsolicitedServerNameAck-*;SupportedVersionSelection-*;TLS13-HRR-*;*HelloRetryRequest*;PointFormat-*;EncryptedExtensionsWithKeyShare-*;EmptyEncryptedExtensions-*;NoNullCompression-*;NoCommonAlgorithms-*;MaxSendFragment-*;Renegotiate-*;RenegotiationInfo-*;Server-InvalidSignature-*;Server-Sign-*;Server-Verify*;Server-DDoS-*;Server-TooLongSessionID-*;Client-Sign*;Client-Verify*;ClientAuth-*;RequireAnyClientCertificate-*;SkipClientCertificate-*;Verify-ClientAuth-*;TLS13-Client-ClientAuth-*;RejectEmptyOCSPResponse-*;SignedCertificateTimestampList-*;RejectPSSKeyType-*;ServerSkipCertificateVerify-*;RSAKeyUsage-*;ECDSAKeyUsage-*;GarbageCertificate-*;RetainOnlySHA256-*;FailCertCallback-*;TLS-HintMismatch-*;TicketCallback-*;TicketSessionIDLength-*;TLS12NoSessionID-*;ResumeTLS12SessionID-*;TLS13-TicketAge*;TLS13-TestValidTicketAge-*;TLS13-HonorServerSessionTicketLifetime-*;TLS13-Client-EmptyTicketFlags;TLS13-Client-NonminimalTicketFlags;TLS13-DuplicateTicketEarlyDataSupport;TLS13-ExpectTicketEarlyDataSupport;TLS13-NoTicket-*;TLS13-Server-ResumptionAcrossNames;TLS13-SendNoKEMModesWithPSK-*;ExtraHandshake-*;TrailingDataWithFinished-*;GREASE-*;CheckECDSACurve-*;ECDSACurveMismatch-*;EMS-*;Resume-Client-CipherMismatch-*;Resume-Server-CipherNotPreferred-*;Resume-Server-DeclineBadCipher-*;Resume-Server-DeclineCrossVersion-*;Resume-Server-UnofferedCipher-*;ALPNClient-RejectUnknown-TLS-TLS12;ALPNClient-RejectUnknown-TLS-TLS13;ALPNServer-Async-TLS-TLS12}"
BOGO_NUM_WORKERS="${BOGO_NUM_WORKERS:-4}"
BOGO_GO_TEST_TIMEOUT="${BOGO_GO_TEST_TIMEOUT:-30m}"

remote_json="/tmp/connaxis_bogo.json"

echo "[bogo] ssh_host=$BOGO_SSH_HOST"
echo "[bogo] remote_connaxis_dir=$BOGO_REMOTE_EVIODIR"
echo "[bogo] remote_boringssl_dir=$BOGO_REMOTE_BORINGSSL_DIR"
echo "[bogo] remote_go=$BOGO_REMOTE_GO"
echo "[bogo] tls_engine=$BOGO_TLS_ENGINE"
echo "[bogo] test_pattern=$BOGO_TEST_PATTERN"
echo "[bogo] skip_pattern=$BOGO_SKIP_PATTERN"
echo "[bogo] num_workers=$BOGO_NUM_WORKERS"
echo "[bogo] go_test_timeout=$BOGO_GO_TEST_TIMEOUT"
echo "[bogo] remote_json=$remote_json"

ssh "$BOGO_SSH_HOST" "set -euo pipefail; test -d '$BOGO_REMOTE_EVIODIR'"

# Ensure BoringSSL checkout exists on the remote host.
ssh "$BOGO_SSH_HOST" "set -euo pipefail; if [[ ! -d '$BOGO_REMOTE_BORINGSSL_DIR/ssl/test/runner' ]]; then rm -rf '$BOGO_REMOTE_BORINGSSL_DIR'; git clone --depth 1 https://boringssl.googlesource.com/boringssl '$BOGO_REMOTE_BORINGSSL_DIR'; fi"

# Build shim on the remote host.
ssh "$BOGO_SSH_HOST" "set -euo pipefail; cd '$BOGO_REMOTE_EVIODIR' && '$BOGO_REMOTE_GO' build -o /tmp/connaxis_bogo_shim ./cmd/bogo_shim"

# Run runner on the remote host.
ssh "$BOGO_SSH_HOST" "set -euo pipefail; cd '$BOGO_REMOTE_BORINGSSL_DIR' && CONNAXIS_BOGO_TLS_ENGINE='$BOGO_TLS_ENGINE' '$BOGO_REMOTE_GO' test -timeout '$BOGO_GO_TEST_TIMEOUT' ./ssl/test/runner -run TestAll -count=1 -args -shim-path /tmp/connaxis_bogo_shim -num-workers '$BOGO_NUM_WORKERS' -allow-unimplemented -loose-errors -test '$BOGO_TEST_PATTERN' -skip '$BOGO_SKIP_PATTERN' -pipe -json-output '$remote_json'"

# Fetch JSON output back to local OUT_DIR.
ssh "$BOGO_SSH_HOST" "set -euo pipefail; test -f '$remote_json'; cat '$remote_json'" >"$OUT_DIR/bogo.json"
echo "[bogo] wrote $OUT_DIR/bogo.json"
