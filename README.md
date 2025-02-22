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

The service implements a sophisticated caching mechanism to minimize API calls to Polygon.io and optimize performance:

### Cache Types

1. Frequent Cache (Real-time)
   - Used for rapidly changing data like last trade prices
   - Stored in memory
   - Very short TTL (configurable, default 1 second)
   - Updated on-demand when data is requested

2. Regular Interval Cache
   - Used for data that needs periodic updates
   - Can be configured to run during specific hours
   - Supports both memory and disk storage
   - Example: Option volume data updated every 30 minutes during market hours

3. Timed Update Cache
   - Used for data that updates at specific times
   - Can be configured to run at exact times daily
   - Typically uses disk storage for persistence
   - Example: Option open interest updated at 8am and 2pm

### Cache Implementation
- In-memory cache with configurable limit (default 50MB)
- Disk cache with configurable limit (default 2GB)
- Thread-safe implementation using mutex locks
- Compression support for disk cache
- Automatic cache expiration
- Cache size monitoring and limits enforcement

### Cache Configuration
The following environment variables can be used to configure the cache:

```bash
# Cache TTLs
JAX_DEX_CACHE_TTL=15m        # Default 15 minutes
JAX_MARKET_CACHE_TTL=1s      # Default 1 second

# Cache Limits
JAX_MEMORY_CACHE_LIMIT=52428800     # Default 50MB (in bytes)
JAX_DISK_CACHE_LIMIT=2147483648     # Default 2GB (in bytes)
JAX_CACHE_DIR=cache                 # Default cache directory
```

### Cache Behavior
- Small data (<10KB) is automatically stored in memory
- Larger data is stored on disk with optional compression
- Cache entries include size tracking and expiration time
- Cache is cleared on service restart
- Failed cache updates are retried with exponential backoff

### Benefits
- Reduced Polygon.io API calls
- Lower latency for cached responses
- Efficient memory and disk usage
- Support for different data update patterns
- Automatic recovery from failures

### Cache Monitoring
The service logs the following cache-related events:
- Cache hits/misses
- Cache size warnings
- Failed cache updates
- Cache cleanup operations

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
    ca: '../certs/ca.crt',
    cert: '../certs/client.crt',
    key: '../certs/client.key'
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
