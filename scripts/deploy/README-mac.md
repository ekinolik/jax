# JAX UI Deployment (Linux x64)

## Prerequisites

Before starting the application, ensure you have:

1. The required certificates certs and keys in the `certs` directory:
   - `certs/server/server.crt`
   - `certs/server/server.key`
   - `certs/ca/ca.crt`
   - `certs/ca/ca.key`
   - `certs/ca/ca.srl`
2. If you do not have the server certs and keys you can generate them
   - `./certs/generate-certs.sh
3. You will need a `cache-configs/cache_tasks.yaml` that you can create from the example.yaml

## Setup

1. Copy `.env.example` to `.env` and adjust the values for your environment
2. Place your certificates in the `certs` directory
3. Run `./start.sh` to start the application

## Configuration

The following environment variables can be set in `.env`:
- `JAX_PORT`: The GRPC Port
- `JAX_GRPC_HOST`: The host and port of the GRPC server `:50051` for all interfaces
- `POLYGON_API_KEY`: API key for Polygon.io server
- `JAX_DEX_CACHE_TTL`: Cache TTL for DEX data (supported formats: Ns, Nm, Nh)
- `JAX_MARKET_CACHE_TTL`: Cache TTL for Market data (supported formats: Ns, Nm, Nh)
- `JAX_AGGREGATE_CACHE_TTL`: Cache TTL for Aggregate trades (supported formats: Ns, Nm, Nh)
- `JAX_NUM_EXECUTORS`: Number of background scheduler tasks to run
