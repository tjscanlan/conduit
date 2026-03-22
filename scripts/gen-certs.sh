#!/usr/bin/env bash
# gen-certs.sh — generates a local CA and mTLS certs for development
# In production, use your PKI (Vault, cert-manager, etc.) instead.
set -euo pipefail

CERTS_DIR="$(git rev-parse --show-toplevel)/certs"
mkdir -p "$CERTS_DIR"
cd "$CERTS_DIR"

echo "==> Generating CA key and certificate..."
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt \
  -subj "/CN=agent-observability-ca/O=AgentObs"

echo "==> Generating gateway server cert..."
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr \
  -subj "/CN=gateway/O=AgentObs"
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out server.crt \
  -extfile <(printf "subjectAltName=DNS:gateway,DNS:localhost")

echo "==> Generating example agent client cert..."
openssl genrsa -out agent-example.key 2048
openssl req -new -key agent-example.key -out agent-example.csr \
  -subj "/CN=example-agent/O=AgentObs"
openssl x509 -req -days 365 -in agent-example.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out agent-example.crt

# Clean up CSRs
rm -f *.csr *.srl

echo ""
echo "Certs written to $CERTS_DIR:"
ls -1 "$CERTS_DIR"
echo ""
echo "To issue a cert for a new agent:"
echo "  CN=my-agent ./scripts/gen-certs.sh agent"
