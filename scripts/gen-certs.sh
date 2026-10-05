#!/bin/sh
# Generate self-signed TLS certificates for vswitch gRPC server.
# Usage: sh scripts/gen-certs.sh [days]
set -e
DAYS="${1:-3650}"
CERT_DIR="$(dirname "$0")/../certs"
mkdir -p "$CERT_DIR"

openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
  -keyout "$CERT_DIR/server.key" -out "$CERT_DIR/server.crt" \
  -days "$DAYS" -nodes \
  -subj "/CN=vswitch" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:0.0.0.0"

echo "Certificates generated in $CERT_DIR:"
echo "  server.crt  (certificate / CA for clients)"
echo "  server.key  (private key — keep secret)"