#!/bin/bash

# Check if client name is provided
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <client-name>"
    echo "Example: $0 alice"
    exit 1
fi

CLIENT_NAME=$1
OUTPUT_DIR="client-certs/$CLIENT_NAME"

# Check if CA exists
if [ ! -f "certs/ca/ca.crt" ] || [ ! -f "certs/ca/ca.key" ]; then
    echo "Error: CA certificate and key not found in certs/ca/"
    echo "Please run generate-certs.sh first to set up the server and CA"
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Create config file for client certificate
cat > "$OUTPUT_DIR/client.conf" << EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = $CLIENT_NAME

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = $CLIENT_NAME
EOF

# Generate client private key and CSR
openssl genpkey -algorithm RSA -out "$OUTPUT_DIR/client.key"
openssl req -new -key "$OUTPUT_DIR/client.key" -out "$OUTPUT_DIR/client.csr" -config "$OUTPUT_DIR/client.conf"

# Sign client CSR with CA
openssl x509 -req -in "$OUTPUT_DIR/client.csr" \
    -CA certs/ca/ca.crt \
    -CAkey certs/ca/ca.key \
    -CAcreateserial \
    -out "$OUTPUT_DIR/client.crt" \
    -days 365 \
    -extensions v3_req \
    -extfile "$OUTPUT_DIR/client.conf"

# Copy CA certificate (needed by client)
cp certs/ca/ca.crt "$OUTPUT_DIR/ca.crt"

# Clean up CSR and config files
rm "$OUTPUT_DIR/client.csr" "$OUTPUT_DIR/client.conf"

# Set appropriate permissions
chmod 600 "$OUTPUT_DIR/client.key"
chmod 644 "$OUTPUT_DIR/client.crt" "$OUTPUT_DIR/ca.crt"

echo "Client certificates generated successfully in $OUTPUT_DIR/"
echo "Files generated:"
echo "  - $OUTPUT_DIR/ca.crt     (CA certificate - public)"
echo "  - $OUTPUT_DIR/client.crt (Client certificate - public)"
echo "  - $OUTPUT_DIR/client.key (Client private key - keep secure)"
echo ""
echo "To use these certificates with the React client:"
echo "1. Copy these files to your React project"
echo "2. Initialize the client with:"
echo ""
echo "const client = new JaxClient({"
echo "  host: 'your-server:50051',"
echo "  useTLS: true,"
echo "  certPaths: {"
echo "    ca: './path/to/ca.crt',"
echo "    cert: './path/to/client.crt',"
echo "    key: './path/to/client.key'"
echo "  }"
echo "});" 