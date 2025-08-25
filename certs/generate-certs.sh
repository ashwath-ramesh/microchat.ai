#!/bin/bash
set -e
# Generate development certificates for microchat.ai

echo "Generating development certificates..."

# Get script directory to ensure files are created in certs/
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Generate CA private key
openssl genrsa -out ca.key 4096

# Generate CA certificate
openssl req -new -x509 -days 365 -key ca.key -out ca.crt -subj "/CN=MicroChat CA"

# Generate server private key
openssl genrsa -out server.key 4096

# Generate server certificate signing request with Subject Alternative Names
openssl req -new -key server.key -out server.csr -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

# Sign server certificate with CA
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -copy_extensions copy

# Clean up temporary files
rm server.csr ca.srl

echo "Development certificates generated successfully!"
echo "Files created in certs/: ca.crt, ca.key, server.crt, server.key"