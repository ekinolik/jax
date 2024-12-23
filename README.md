# JAX - Options Delta Exposure Service

JAX is a gRPC service that calculates delta exposure (DEX) for options using data from Polygon.io.

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
grpcurl -plaintext -d '{"underlyingAsset": "AAPL", "startStrikePrice": 170, "endStrikePrice": 180}' localhost:50051 dex.v1.DexService/GetDex

# Get DEX for all AAPL options (no strike price filter)
grpcurl -plaintext -d '{"underlyingAsset": "AAPL"}' localhost:50051 dex.v1.DexService/GetDex
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