#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
OUT_DIR="${OUT_DIR:?OUT_DIR is required}"

BOGO_LOCAL_BORINGSSL_DIR="${BOGO_LOCAL_BORINGSSL_DIR:-/private/tmp/boringssl-src}"
BOGO_LOCAL_GO="${BOGO_LOCAL_GO:-$(command -v go || true)}"
BOGO_TLS_ENGINE="${BOGO_TLS_ENGINE:-atls}"
BOGO_TEST_PATTERN="${BOGO_TEST_PATTERN:-*-TLS12;*-TLS13;TLS13-*}"
BOGO_SKIP_PATTERN="${BOGO_SKIP_PATTERN:-*-DTLS-*;*-QUIC-*;CertificateSelection-*;ALPS-*;NPN-*;NegotiateALPNAndNPN-*;EarlyData*;TLS13-EarlyData-*;SkipEarlyData-*;CurveTest-*;CustomKeyShares-*;KeyShareWithServerHint-*;DuplicateKeyShares-*;QUICTransportParams-*;*ECH*;CertCompression*;DuplicateCertCompressionExt*;DuplicateExtension*;AllExtensions-*;UnknownExtension-*;UnknownUnencryptedExtension-*;UnofferedExtension-*;ExtraClientEncryptedExtension-*;Send*;ServerNameExtensionServer-TLS-*;TolerateServerNameAck-*;UnsolicitedServerNameAck-*;SupportedVersionSelection-*;TLS13-HRR-*;*HelloRetryRequest*;PointFormat-*;EncryptedExtensionsWithKeyShare-*;EmptyEncryptedExtensions-*;NoNullCompression-*;NoCommonAlgorithms-*;MaxSendFragment-*;Renegotiate-*;RenegotiationInfo-*;Server-InvalidSignature-*;Server-Sign-*;Server-Verify*;Server-DDoS-*;Server-TooLongSessionID-*;Client-Sign*;Client-Verify*;ClientAuth-*;RequireAnyClientCertificate-*;SkipClientCertificate-*;Verify-ClientAuth-*;TLS13-Client-ClientAuth-*;RejectEmptyOCSPResponse-*;SignedCertificateTimestampList-*;RejectPSSKeyType-*;ServerSkipCertificateVerify-*;RSAKeyUsage-*;ECDSAKeyUsage-*;GarbageCertificate-*;RetainOnlySHA256-*;FailCertCallback-*;TLS-HintMismatch-*;TicketCallback-*;TicketSessionIDLength-*;TLS12NoSessionID-*;ResumeTLS12SessionID-*;TLS13-TicketAge*;TLS13-TestValidTicketAge-*;TLS13-HonorServerSessionTicketLifetime-*;TLS13-Client-EmptyTicketFlags;TLS13-Client-NonminimalTicketFlags;TLS13-DuplicateTicketEarlyDataSupport;TLS13-ExpectTicketEarlyDataSupport;TLS13-NoTicket-*;TLS13-Server-ResumptionAcrossNames;TLS13-SendNoKEMModesWithPSK-*;ExtraHandshake-*;TrailingDataWithFinished-*;GREASE-*;CheckECDSACurve-*;ECDSACurveMismatch-*;EMS-*;Resume-Client-CipherMismatch-*;Resume-Server-CipherNotPreferred-*;Resume-Server-DeclineBadCipher-*;Resume-Server-DeclineCrossVersion-*;Resume-Server-UnofferedCipher-*;ALPNClient-RejectUnknown-TLS-TLS12;ALPNClient-RejectUnknown-TLS-TLS13;ALPNServer-Async-TLS-TLS12}"
BOGO_NUM_WORKERS="${BOGO_NUM_WORKERS:-4}"
BOGO_GO_TEST_TIMEOUT="${BOGO_GO_TEST_TIMEOUT:-30m}"

if [[ -z "$BOGO_LOCAL_GO" ]]; then
  echo "[bogo] go binary not found" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"

if [[ ! -d "$BOGO_LOCAL_BORINGSSL_DIR/ssl/test/runner" ]]; then
  echo "[bogo] missing boringssl checkout at $BOGO_LOCAL_BORINGSSL_DIR" >&2
  exit 2
fi

shim_path="$OUT_DIR/evio_bogo_shim"
json_path="$OUT_DIR/bogo.json"

echo "[bogo] local_boringssl_dir=$BOGO_LOCAL_BORINGSSL_DIR"
echo "[bogo] go=$BOGO_LOCAL_GO"
echo "[bogo] tls_engine=$BOGO_TLS_ENGINE"
echo "[bogo] test_pattern=$BOGO_TEST_PATTERN"
echo "[bogo] skip_pattern=$BOGO_SKIP_PATTERN"
echo "[bogo] num_workers=$BOGO_NUM_WORKERS"
echo "[bogo] json=$json_path"

"$BOGO_LOCAL_GO" build -o "$shim_path" "$ROOT_DIR/cmd/bogo_shim"
(
  cd "$BOGO_LOCAL_BORINGSSL_DIR"
  CONNAXIS_BOGO_TLS_ENGINE="$BOGO_TLS_ENGINE" \
    "$BOGO_LOCAL_GO" test -timeout "$BOGO_GO_TEST_TIMEOUT" ./ssl/test/runner -run TestAll -count=1 -args \
    -shim-path "$shim_path" \
    -num-workers "$BOGO_NUM_WORKERS" \
    -allow-unimplemented \
    -loose-errors \
    -test "$BOGO_TEST_PATTERN" \
    -skip "$BOGO_SKIP_PATTERN" \
    -pipe \
    -json-output "$json_path"
)

echo "[bogo] wrote $json_path"
