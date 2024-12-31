# JAX - Options Delta Exposure Service

JAX is a gRPC service that calculates delta exposure (DEX) for options using data from Polygon.io.

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
```

The response includes:
- `spotPrice`: Current price of the underlying asset
- `strikePrices`: Map of strike prices to expiration dates and option types, with DEX values

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
