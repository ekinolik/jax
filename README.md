# JAX - Options Delta and Gamma Exposure Service

JAX is a gRPC service that calculates delta exposure (DEX) and gamma exposure (GEX) for options using data from Polygon.io.

## Available Methods

### OptionService
1. `GetDex`: Returns DEX data for a range of strike prices
   - Input: `underlyingAsset` (required), `startStrikePrice` (optional), `endStrikePrice` (optional)
   - Returns DEX data for all strikes within the specified range

2. `GetDexByStrikes`: Returns DEX data for a specified number of strikes around the spot price
   - Input: `underlyingAsset` (required), `numStrikes` (required)
   - Returns DEX data for N strikes centered around the spot price
   - For even numbers, returns one more strike above spot price than below
   - Adjusts if not enough strikes are available in either direction

3. `GetGex`: Returns GEX data for a range of strike prices
   - Input: `underlyingAsset` (required), `startStrikePrice` (optional), `endStrikePrice` (optional)
   - Returns GEX data for all strikes within the specified range

4. `GetGexByStrikes`: Returns GEX data for a specified number of strikes around the spot price
   - Input: `underlyingAsset` (required), `numStrikes` (required)
   - Returns GEX data for N strikes centered around the spot price
   - For even numbers, returns one more strike above spot price than below
   - Adjusts if not enough strikes are available in either direction

### MarketService
1. `GetLastTrade`: Returns the most recent trade data for a ticker
   - Input: `ticker` (required)
   - Returns price, size, timestamp, and exchange information

## Prerequisites

- Go 1.21 or later
- Protocol Buffers compiler
- Polygon.io API key

## Installation

1. Install the Protocol Buffers compiler:
```bash
brew install protobuf
```

2. Install Go Protocol Buffers plugins:
```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

3. Install dependencies:
```bash
go mod tidy
```

## Setup

1. Set your Polygon.io API key:
```bash
export POLYGON_API_KEY=your_api_key_here
```

2. Generate Protocol Buffer code:
```bash
make proto
```

3. Build the service:
```bash
make build
```

## Running the Service

Start the server:
```bash
make run
```

The service will start on port 50051.

## Usage

Use grpcurl to query the service. Example:

```bash
# Get DEX for AAPL options between strike prices 170 and 180
grpcurl -plaintext -d '{"underlyingAsset": "AAPL", "startStrikePrice": 170, "endStrikePrice": 180}' localhost:50051 jax.v1.OptionService/GetDex

# Get DEX for all AAPL options (no strike price filter)
grpcurl -plaintext -d '{"underlyingAsset": "AAPL"}' localhost:50051 jax.v1.OptionService/GetDex

# Get GEX for AAPL options between strike prices 170 and 180
grpcurl -plaintext -d '{"underlyingAsset": "AAPL", "startStrikePrice": 170, "endStrikePrice": 180}' localhost:50051 jax.v1.OptionService/GetGex

# Get GEX for 5 strikes around spot price for AAPL
grpcurl -plaintext -d '{"underlyingAsset": "AAPL", "numStrikes": 5}' localhost:50051 jax.v1.OptionService/GetGexByStrikes
```

The response includes:
- `spotPrice`: Current price of the underlying asset
- `strikePrices`: Map of strike prices to expiration dates and option types, with DEX or GEX values

## Response Structure

```json
{
  "spotPrice": 123.45,
  "strikePrices": {
    "170": {
      "expirationDates": {
        "2024-03-15": {
          "optionTypes": {
            "call": {
              "value": 12345.67
            },
            "put": {
              "value": -1234.56
            }
          }
        }
      }
    }
  }
}
```

## Caching Strategy

The service implements an efficient caching mechanism to minimize API calls to Polygon.io:

### Cache Implementation
- In-memory cache with 15-minute TTL
- Caches full option chain data per underlying asset
- Thread-safe implementation using mutex locks
- Cache expiration time is communicated to clients

### Cache Behavior
- First request for an asset fetches all strike prices
- Subsequent requests for the same asset use cached data
- Strike price filtering is performed post-cache
- Cache is cleared on server restart

### Benefits
- Reduced Polygon.io API calls
- Lower latency for cached responses
- Efficient memory usage
- Support for multiple strike ranges from single cache entry

## Authentication

The server uses mutual TLS (mTLS) authentication. This means both the server and clients need certificates signed by a trusted Certificate Authority (CA).

### Generating Certificates

1. Run the certificate generation script:
```bash
chmod +x scripts/generate-certs.sh
./scripts/generate-certs.sh
```

This will create the following files in the `certs` directory:
- `ca/ca.key`: CA private key
- `ca/ca.crt`: CA certificate
- `server/server.key`: Server private key
- `server/server.crt`: Server certificate
- `client/client.key`: Client private key
- `client/client.crt`: Client certificate

### Server Configuration

The server is automatically configured to use mTLS. It will look for the certificates in the following locations:
- `certs/ca/ca.crt`: CA certificate to verify client certificates
- `certs/server/server.key`: Server private key
- `certs/server/server.crt`: Server certificate

### Client Configuration

When creating a new JaxClient instance, provide the certificate paths:

```typescript
const client = new JaxClient({
  host: 'localhost:50051',
  useTLS: true,
  certPaths: {
    ca: '../certs/ca/ca.crt',
    cert: '../certs/client/client.crt',
    key: '../certs/client/client.key'
  }
});
```

### Security Notes

1. Keep private keys secure and never commit them to version control
2. In production, use a proper Certificate Authority
3. Regularly rotate certificates
4. Set appropriate file permissions (the script sets 600 for private keys)

### Generating Client Certificates

For clients that need to connect to the server, you can generate client certificates using the provided script:

```bash
# Generate certificates for a client named "alice"
./scripts/generate-client-cert.sh alice

# Generate certificates for a client named "bob"
./scripts/generate-client-cert.sh bob
```

This will create a directory `client-certs/<client-name>` containing:
- `ca.crt`: The CA certificate (public)
- `client.crt`: The client's certificate (public)
- `client.key`: The client's private key (keep secure)

These files should be securely transferred to the client and used to initialize their connection. The client should keep their private key secure and never share it.

For React clients using the `@ekinolik/jax-react-client` package, copy these files to their project and initialize the client with:

```typescript
const client = new JaxClient({
  host: 'your-server:50051',
  useTLS: true,
  certPaths: {
    ca: './path/to/ca.crt',
    cert: './path/to/client.crt',
    key: './path/to/client.key'
  }
});
```

### Security Best Practices for Client Certificates

1. Generate separate certificates for each client
2. Never share private keys between clients
3. Use secure methods to transfer certificates to clients
4. Regularly rotate client certificates
5. Maintain a list of valid client certificates and revoke them when necessary
6. Store client private keys securely, preferably in a secure secret management system
