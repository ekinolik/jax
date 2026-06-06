# JAX - Options Delta and Gamma Exposure Service

JAX is a gRPC service that calculates delta exposure (DEX) and gamma exposure (GEX) for options using data from [Massive.com](https://massive.com) (formerly Polygon.io). JAX also includes foundational components for a real-time **Confluence Data API** (multi-signal day-trading scores).

## Data Provider

JAX uses the official [`github.com/massive-com/client-go/v2`](https://github.com/massive-com/client-go) client for REST and WebSocket access. Set `POLYGON_API_KEY` with your Massive.com API key (the environment variable name is unchanged for backward compatibility).

## Confluence Data API (Phase 0–2)

Phase 0 adds the data layer; **Phase 1** adds multi-level GEX/DEX support/resistance, signal calculators, and composite scoring; **Phase 2** wires the event-driven processor into `cmd/server` with stream hub spot ticks, greeks timers, daily OI prefetch, and in-memory snapshot cache.

| Component | Location | Purpose |
|-----------|----------|---------|
| Types & expiration logic | `pkg/confluence/` | `StrikeProfile`, `OptionSlice`, `ConfluenceSnapshot`, `ScoreInput` |
| Signal calculators | `pkg/confluence/signals/` | GEX/DEX levels, gamma/delta/RSI/market/sector/range/entry signals |
| Composite scoring | `pkg/confluence/score.go` | Weighted 0–100 score, readiness bands, haptic/background levels |
| Snapshot builder | `pkg/confluence/signals/score.go` | `BuildSnapshot` — assembles full `ConfluenceSnapshot` |
| Massive REST extensions | `internal/polygon/` | `GetRSI`, `GetTickerOverview`, `GetOptionSlice`, expiration resolver |
| Stream hub | `internal/stream/` | Real-time stock trade WebSocket (`T.*`) spot state |
| Processor | `internal/confluence/` | Event loop, registry, OI cache, RTH gate, `Watch`/`GetSnapshot`, `RecomputeSnapshot` |
| Fetch helpers | `internal/confluence/fetch.go` | Shared OI/greeks/day-stats fetch logic (server + CLI) |
| Config | `confluence-configs/` | Prefetch watchlist, dual-expiration tickers, SIC→ETF map |

**Environment**

```bash
export POLYGON_API_KEY=your_massive_api_key
export CONFLUENCE_CACHE_DIR=./cache/confluence   # same-day OI cache (default)
export JAX_CONFLUENCE_DEBUG_PORT=8081          # optional: GET /confluence/debug?ticker=NVDA
```

### Phase 1 — Signals & scoring

**Multi-level support/resistance** (`signals/levels.go`)

- Per-strike `netGEX` / `netDEX` from `OptionSlice` × live spot (same sign convention as `OptionService`)
- Peak detection: local maxima, keep peaks >20% of per-source max, merge same-source peaks within 0.5% of spot
- Dual-expiration merge (e.g. SPY): levels from multiple slices combined with `expiry_weight`
- Output: `gamma_flip`, ranked `support[]` / `resistance[]`, `nearest_support`, `nearest_resistance`

**Signal axes** (weights sum to 100%)

| Signal | Weight | Icon | Aligned heuristic |
|--------|--------|------|-------------------|
| Gamma support | 25% | G | Spot near rank-1 GEX support; stacked-zone bonus (+5 score) |
| Delta support | 15% | D | Rank-1 DEX support below spot |
| RSI | 20% | R | RSI-14 ≤ 35 (minute timespan, fresh on every recompute) |
| Market | 20% | M | SPY + QQQ day % change vs open |
| Sector | 20% | S | Target vs resolved sector ETF day change (SIC → `sic_sectors.yaml`) |

**Readiness bands**

| Score | Band | Background / haptic level |
|-------|------|---------------------------|
| 0–34 | `no_trade` | 0 |
| 35–54 | `caution` | 1 |
| 55–74 | `possible_entry` | 2 |
| 75–100 | `high_conviction` | 3 |

**Programmatic usage**

```go
snap := signals.BuildSnapshot(confluence.ScoreInput{
    Ticker: "NVDA", Spot: 120.0, RSI: 32,
    OIStatus: confluence.OIStatusReady,
    Slices:   slices,
    SPYOpen: 500, SPYSpot: 502, QQQOpen: 400, QQQSpot: 401,
    TargetOpen: 118, ETFSpot: 201, ETFOpen: 200, SectorETF: "SMH",
})
// snap.Score, snap.ReadinessBand, snap.Signals, snap.Levels
```

When `OIStatus` is `loading`, GEX/DEX signals are suppressed; RSI, market, sector, and spot-based range still compute.

**Design notes**

- RSI is fetched fresh on every recompute — never cached
- Open interest is cached on disk for the current trading day only (`oi/{TICKER}_{DATE}_{EXPIRATION}.json`)
- Confluence processing runs during regular trading hours (9:30–16:00 ET, configurable in `confluence-configs/settings.yaml`)
- SPY uses dual expiration (soonest + soonest weekly Friday) for OI levels

### CLI test tool

One-shot confluence snapshot for a single ticker (no server or stream hub):

```bash
export POLYGON_API_KEY=your_massive_api_key

# Run directly
go run ./cmd/confluence-test --ticker NVDA

# Or build first
make confluence-test
./bin/confluence-test --ticker NVDA
```

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--ticker` | *(required)* | Ticker symbol to score |
| `--config-dir` | `confluence-configs/` | Settings and SIC sector mappings |
| `--format` | `json` | Output format (`json` only in v1) |
| `--json` | `true` | Pretty-print JSON to stdout |

Progress messages go to stderr; the full `ConfluenceSnapshot` JSON goes to stdout. Requires `POLYGON_API_KEY`. Outside regular trading hours, day open/range fall back to the latest daily bar when minute aggregates are unavailable.

### Phase 2 — Event-driven processor

`make run` (or `go run cmd/server/main.go`) now starts alongside the gRPC server:

- **Stream hub** — Massive stock trades WebSocket (`T.*`) for active watches plus SPY/QQQ/sector ETF benchmarks
- **Processor loop** — spot ticks debounce full recompute (≤5s); greeks refresh every 60–90s (max 5 min) and on spot move >0.5%
- **RTH only** — processing during 9:30–16:00 ET weekdays (`settings.yaml`); pre-market subscriptions get `market_status: closed` until open
- **OI lifecycle** — lazy fetch on first `Watch` per day; 08:00 ET prefetch for `prefetch_watchlist`; same-day disk cache reused on restart
- **Active ticker registry** — ref-counted `Watch` API; max 5 active tickers; 5 min idle grace after last unsubscribe
- **Snapshot cache** — latest `ConfluenceSnapshot` per ticker in memory; push to `Watch` channels when score/signal status changes (30s duplicate debounce)
- **Debug HTTP** (optional) — `GET http://127.0.0.1:PORT/confluence/debug?ticker=NVDA` when `JAX_CONFLUENCE_DEBUG_PORT` is set (loopback only)

**Verify Phase 2**

```bash
export POLYGON_API_KEY=your_massive_api_key
export JAX_CONFLUENCE_DEBUG_PORT=8081   # optional

make run
# Logs should include: [confluence] processor started (max_active_tickers=5)

# One-shot scoring still works without the server:
make confluence-test
./bin/confluence-test --ticker NVDA

# With debug port, after a Watch subscription (Phase 3 gRPC) or programmatic Watch:
curl -s 'http://localhost:8081/confluence/debug?ticker=NVDA' | head
```

Graceful shutdown: `SIGINT`/`SIGTERM` stops gRPC, stream hub, and confluence processor.

**Later phases:** gRPC `ConfluenceService` (Phase 3), jax-ov WebSocket gateway (Phase 4).

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

# Task Execution
JAX_NUM_EXECUTORS=3                 # Default 3 executors (min: 1)
```

The number of executors determines how many cache tasks can be processed concurrently. More executors can improve throughput but will consume more system resources. Choose a value based on:
- Available system resources (CPU, memory)
- Expected task load
- Task execution patterns
- API rate limits

For most use cases, the default of 3 executors provides a good balance between performance and resource usage.

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
