#!/bin/bash
set -e
# Generate ECDSA P-384 certificates for microchat.ai

echo "Generating ECDSA P-384 certificates..."

# Get script directory to ensure files are created in certs/
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Generate CA private key with ECDSA P-384
openssl ecparam -name secp384r1 -genkey -out ca.key

# Generate CA certificate
openssl req -new -x509 -days 90 -key ca.key -out ca.crt -subj "/CN=MicroChat CA" -sha384

# Generate server private key with ECDSA P-384
openssl ecparam -name secp384r1 -genkey -out server.key

# Generate server certificate signing request with Subject Alternative Names
openssl req -new -key server.key -out server.csr -subj "/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,DNS:microchat.ai,IP:127.0.0.1"

# Sign server certificate with CA using SHA-384
openssl x509 -req -days 90 -in server.csr -CA ca.crt -CAkey ca.key \
    -CAcreateserial -out server.crt -sha384 -copy_extensions copy

# Clean up temporary files
rm server.csr ca.srl

echo "ECDSA P-384 certificates generated successfully!"
echo "Files created in certs/: ca.crt, ca.key, server.crt, server.key"
echo "Certificate validity: 90 days (renewal recommended at 60 days)"