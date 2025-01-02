#!/bin/bash

# Create directories for certificates
mkdir -p certs/{ca,server,client}

# Generate CA private key and certificate
openssl genpkey -algorithm RSA -out certs/ca/ca.key
openssl req -new -x509 -key certs/ca/ca.key -out certs/ca/ca.crt -days 365 -subj "/CN=JAX CA"

# Create config file for server certificate
cat > certs/server/server.conf << EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = localhost

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
IP.1 = 127.0.0.1
EOF

# Create config file for client certificate
cat > certs/client/client.conf << EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = jax-client

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = jax-client
EOF

# Generate server private key and CSR
openssl genpkey -algorithm RSA -out certs/server/server.key
openssl req -new -key certs/server/server.key -out certs/server/server.csr -config certs/server/server.conf

# Generate client private key and CSR
openssl genpkey -algorithm RSA -out certs/client/client.key
openssl req -new -key certs/client/client.key -out certs/client/client.csr -config certs/client/client.conf

# Sign server CSR with CA (including the SANs from the CSR)
openssl x509 -req -in certs/server/server.csr \
    -CA certs/ca/ca.crt \
    -CAkey certs/ca/ca.key \
    -CAcreateserial \
    -out certs/server/server.crt \
    -days 365 \
    -extensions v3_req \
    -extfile certs/server/server.conf

# Sign client CSR with CA (including the SANs from the CSR)
openssl x509 -req -in certs/client/client.csr \
    -CA certs/ca/ca.crt \
    -CAkey certs/ca/ca.key \
    -CAcreateserial \
    -out certs/client/client.crt \
    -days 365 \
    -extensions v3_req \
    -extfile certs/client/client.conf

# Clean up CSR files and config files
rm certs/server/server.csr certs/client/client.csr
rm certs/server/server.conf certs/client/client.conf

# Set appropriate permissions
chmod 600 certs/ca/ca.key certs/server/server.key certs/client/client.key

echo "Certificates generated successfully in the certs directory" 