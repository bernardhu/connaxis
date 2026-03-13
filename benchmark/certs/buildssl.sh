#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl not found" >&2
  exit 127
fi

CERT_FILE="${CERT_FILE:-cert.pem}"
KEY_FILE="${KEY_FILE:-key.pem}"
DAYS="${DAYS:-365}"
CERT_MODE="${CERT_MODE:-selfsigned}"
CA_CERT_FILE="${CA_CERT_FILE:-ca.pem}"
CA_KEY_FILE="${CA_KEY_FILE:-ca.key.pem}"
CA_DAYS="${CA_DAYS:-3650}"

COUNTRY="${COUNTRY:-CN}"
STATE="${STATE:-Guangdong}"
LOCALITY="${LOCALITY:-Guangzhou}"
ORG="${ORG:-connaxis}"
ORG_UNIT="${ORG_UNIT:-benchmark}"

SAN_DNS="${SAN_DNS:-localhost}"
SAN_IPS="${SAN_IPS:-127.0.0.1,::1}"

# If CERT_CN not provided, use first SAN DNS or first SAN IP.
if [[ -n "${CERT_CN:-}" ]]; then
  CERT_CN_VALUE="$CERT_CN"
else
  if [[ -n "$SAN_DNS" ]]; then
    CERT_CN_VALUE="${SAN_DNS%%,*}"
  else
    CERT_CN_VALUE="${SAN_IPS%%,*}"
  fi
fi

trim() {
  local v="$1"
  v="${v#"${v%%[![:space:]]*}"}"
  v="${v%"${v##*[![:space:]]}"}"
  printf '%s' "$v"
}

TMP_CONF="$(mktemp "${TMPDIR:-/tmp}/connaxis-openssl.XXXXXX.cnf")"
trap 'rm -f "$TMP_CONF"' EXIT

{
  cat <<EOF
[ req ]
default_bits       = 2048
default_md         = sha256
distinguished_name = req_dn
x509_extensions    = v3_req
prompt             = no

[ req_dn ]
C  = ${COUNTRY}
ST = ${STATE}
L  = ${LOCALITY}
O  = ${ORG}
OU = ${ORG_UNIT}
CN = ${CERT_CN_VALUE}

[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[ v3_ca ]
basicConstraints = critical, CA:TRUE, pathlen:1
keyUsage = critical, keyCertSign, cRLSign
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer

[ alt_names ]
EOF
} >"$TMP_CONF"

dns_idx=1
if [[ -n "$SAN_DNS" ]]; then
  IFS=',' read -r -a dns_values <<<"$SAN_DNS"
  for raw in "${dns_values[@]}"; do
    item="$(trim "$raw")"
    if [[ -n "$item" ]]; then
      printf 'DNS.%d = %s\n' "$dns_idx" "$item" >>"$TMP_CONF"
      dns_idx=$((dns_idx + 1))
    fi
  done
fi

ip_idx=1
if [[ -n "$SAN_IPS" ]]; then
  IFS=',' read -r -a ip_values <<<"$SAN_IPS"
  for raw in "${ip_values[@]}"; do
    item="$(trim "$raw")"
    if [[ -n "$item" ]]; then
      printf 'IP.%d = %s\n' "$ip_idx" "$item" >>"$TMP_CONF"
      ip_idx=$((ip_idx + 1))
    fi
  done
fi

if (( dns_idx == 1 && ip_idx == 1 )); then
  echo "at least one SAN entry is required (set SAN_DNS and/or SAN_IPS)" >&2
  exit 2
fi

case "$CERT_MODE" in
  selfsigned)
    openssl req -x509 -newkey rsa:2048 -sha256 -nodes \
      -keyout "$KEY_FILE" \
      -out "$CERT_FILE" \
      -days "$DAYS" \
      -config "$TMP_CONF" \
      -extensions v3_req
    ;;
  ca|chain)
    openssl req -x509 -newkey rsa:2048 -sha256 -nodes \
      -keyout "$CA_KEY_FILE" \
      -out "$CA_CERT_FILE" \
      -days "$CA_DAYS" \
      -config "$TMP_CONF" \
      -extensions v3_ca

    CSR_FILE="$(mktemp "${TMPDIR:-/tmp}/connaxis-openssl.XXXXXX.csr")"
    trap 'rm -f "$TMP_CONF" "$CSR_FILE"' EXIT

    openssl req -new -newkey rsa:2048 -sha256 -nodes \
      -keyout "$KEY_FILE" \
      -out "$CSR_FILE" \
      -config "$TMP_CONF"

    LEAF_CERT_FILE="$(mktemp "${TMPDIR:-/tmp}/connaxis-openssl.XXXXXX.leaf.pem")"
    trap 'rm -f "$TMP_CONF" "$CSR_FILE" "$LEAF_CERT_FILE"' EXIT

    openssl x509 -req -sha256 \
      -in "$CSR_FILE" \
      -CA "$CA_CERT_FILE" \
      -CAkey "$CA_KEY_FILE" \
      -CAcreateserial \
      -out "$LEAF_CERT_FILE" \
      -days "$DAYS" \
      -extfile "$TMP_CONF" \
      -extensions v3_req

    cat "$LEAF_CERT_FILE" "$CA_CERT_FILE" >"$CERT_FILE"
    ;;
  *)
    echo "unsupported CERT_MODE: $CERT_MODE (use selfsigned|ca)" >&2
    exit 2
    ;;
esac

if [[ "$CERT_FILE" = /* ]]; then
  cert_path="$CERT_FILE"
else
  cert_path="$ROOT_DIR/$CERT_FILE"
fi
if [[ "$KEY_FILE" = /* ]]; then
  key_path="$KEY_FILE"
else
  key_path="$ROOT_DIR/$KEY_FILE"
fi

echo "wrote certificate: $cert_path"
echo "wrote private key: $key_path"
if [[ "$CERT_MODE" != "selfsigned" ]]; then
  if [[ "$CA_CERT_FILE" = /* ]]; then
    ca_cert_path="$CA_CERT_FILE"
  else
    ca_cert_path="$ROOT_DIR/$CA_CERT_FILE"
  fi
  if [[ "$CA_KEY_FILE" = /* ]]; then
    ca_key_path="$CA_KEY_FILE"
  else
    ca_key_path="$ROOT_DIR/$CA_KEY_FILE"
  fi
  echo "wrote CA certificate: $ca_cert_path"
  echo "wrote CA private key: $ca_key_path"
fi
echo "subject: $(openssl x509 -noout -subject -in "$CERT_FILE" | sed 's/^subject= //')"
echo "san:"
openssl x509 -noout -text -in "$CERT_FILE" | sed -n '/Subject Alternative Name/,+1p'
