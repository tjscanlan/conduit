#!/usr/bin/env bash
# gen-certs.sh — generates a local CA and mTLS certs for development.
# In production, use your PKI (Vault, cert-manager, etc.) instead.
#
# Usage:
#   ./scripts/gen-certs.sh               # CA, gateway server cert, example agent cert
#   ./scripts/gen-certs.sh agent NAME    # issue a client cert for agent NAME from the existing CA
#
# The agent's CommonName (NAME) is its identity: the gateway rate-limits by
# it and stamps it on every span as agent_name.
#
# Env: CERTS_DIR (default: <repo>/certs). FORCE=1 to overwrite an existing CA,
# which invalidates every certificate it issued.
set -euo pipefail

CERTS_DIR="${CERTS_DIR:-$(cd "$(dirname "$0")/.." && pwd)/certs}"
mkdir -p "$CERTS_DIR"
cd "$CERTS_DIR"

issue_agent() {
  local name="$1"
  if [[ ! "$name" =~ ^[A-Za-z0-9._-]{1,64}$ ]]; then
    echo "agent name must be 1-64 chars of [A-Za-z0-9._-]" >&2
    exit 1
  fi
  if [[ ! -f ca.crt || ! -f ca.key ]]; then
    echo "no CA in $CERTS_DIR; run ./scripts/gen-certs.sh first" >&2
    exit 1
  fi
  if [[ -f "$name.crt" && "${FORCE:-}" != 1 ]]; then
    echo "$CERTS_DIR/$name.crt already exists (FORCE=1 to reissue)" >&2
    exit 1
  fi
  echo "==> Issuing client cert for agent \"$name\"..."
  openssl genrsa -out "$name.key" 2048 2>/dev/null
  chmod 600 "$name.key"
  openssl req -new -key "$name.key" -out "$name.csr" -subj "/CN=$name/O=AgentObs"
  openssl x509 -req -days 365 -in "$name.csr" -CA ca.crt -CAkey ca.key \
    -CAcreateserial -out "$name.crt" \
    -extfile <(printf "extendedKeyUsage=clientAuth")
  rm -f "$name.csr" ca.srl
  echo "    $CERTS_DIR/$name.crt"
  echo "    $CERTS_DIR/$name.key"
}

if [[ "${1:-}" == "agent" ]]; then
  [[ $# -eq 2 ]] || { echo "usage: $0 agent NAME" >&2; exit 1; }
  issue_agent "$2"
  exit 0
fi
[[ $# -eq 0 ]] || { echo "usage: $0 [agent NAME]" >&2; exit 1; }

if [[ -f ca.key && "${FORCE:-}" != 1 ]]; then
  echo "A CA already exists in $CERTS_DIR. Regenerating it invalidates every issued cert." >&2
  echo "Use '$0 agent NAME' to issue an agent cert, or FORCE=1 $0 to start over." >&2
  exit 1
fi

echo "==> Generating CA key and certificate..."
openssl genrsa -out ca.key 4096 2>/dev/null
chmod 600 ca.key
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt \
  -subj "/CN=agent-observability-ca/O=AgentObs"

echo "==> Generating gateway server cert..."
openssl genrsa -out server.key 2048 2>/dev/null
chmod 600 server.key
openssl req -new -key server.key -out server.csr \
  -subj "/CN=gateway/O=AgentObs"
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out server.crt \
  -extfile <(printf "subjectAltName=DNS:gateway,DNS:localhost\nextendedKeyUsage=serverAuth")
rm -f server.csr ca.srl

issue_agent agent-example

echo ""
echo "Certs written to $CERTS_DIR:"
ls -1 "$CERTS_DIR"
echo ""
echo "To issue a cert for a new agent:"
echo "  ./scripts/gen-certs.sh agent my-agent"
