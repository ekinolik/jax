# JAX Deployment (Linux)

JAX ships pre-built binaries for AWS EC2 Linux instances. Choose the package that matches your instance architecture:

| Instance type | Architecture | Package / binary suffix |
|---------------|--------------|-------------------------|
| **t4g.nano** (Graviton2) | ARM64 / aarch64 | `linux-arm64` |
| **t3.nano** (x86) | AMD64 / x86_64 | `linux-amd64` |

## Build from source (cross-compile)

From a Mac or Linux dev machine (pure Go — no CGO required):

```bash
# t4g.nano (ARM64 Graviton2) — recommended for new deployments
make build-linux-arm64
make package-linux-arm64   # versioned tarball in package/

# t3.nano (x86_64)
make build-linux-amd64
make package-linux-amd64

# All platforms (amd64 + arm64 Linux, darwin arm64)
make package-all
```

Quick one-off dev binaries land in `bin/` with platform suffixes (for local multi-arch builds):

- `bin/jax-linux-arm64`, `bin/confluence-test-linux-arm64`
- `bin/jax-linux-amd64`, `bin/confluence-test-linux-amd64`

Versioned release tarballs land in `package/` (e.g. `jax-0.1.00001-linux-arm64.tar.gz`).

Each tarball extracts flat to the current directory (no wrapper folder) with architecture-specific binaries only:

| Path | Contents |
|------|----------|
| `bin/` | `jax` and `confluence-test` (no platform suffix; tarball is already arch-specific) |
| `confluence-configs/` | `settings.yaml`, `sic_sectors.yaml` (Confluence processor defaults) |
| `cache-configs/` | `example.yaml` (copy to `cache_tasks.yaml` on deploy) |
| `scripts/` | Helper shell scripts (e.g. cert generation) |
| `certs/` | Empty placeholder; add TLS certs before production use |
| `cache/` | Empty placeholder for runtime cache data |
| `.env.example` | Environment variable template |
| `README.md` | This deployment guide |
| `VERSION` | Build version string |

`make package-all` produces **separate** versioned tarballs per platform (linux-amd64, linux-arm64, darwin-arm64), not one combined archive.

## Deploy to t4g.nano (ARM64)

1. Build on your dev machine: `make package-linux-arm64` (or `make build-linux-arm64` for dev binaries only).
2. Copy the tarball to the instance (e.g. `scp package/jax-*-linux-arm64.tar.gz ec2-user@host:~/`).
3. On the instance, extract and run the server binary:

   ```bash
   tar -xzf jax-*-linux-arm64.tar.gz
   cp .env.example .env   # edit POLYGON_API_KEY, ports, cache paths
   ./bin/jax
   ```

4. Optional CLI smoke test (no server required):

   ```bash
   export POLYGON_API_KEY=your_key
   ./bin/confluence-test --ticker NVDA
   ```

## Deploy to t3.nano (AMD64)

Same steps as t4g.nano, but use `make package-linux-amd64`.

## Prerequisites

Before starting the application, ensure you have:

1. The required certificates and keys in the `certs` directory:
   - `certs/server/server.crt`
   - `certs/server/server.key`
   - `certs/ca/ca.crt`
   - `certs/ca/ca.key`
   - `certs/ca/ca.srl`
2. If you do not have the server certs and keys you can generate them:
   - `./scripts/generate-certs.sh`
3. You will need a `cache-configs/cache_tasks.yaml` that you can create from `example.yaml`

## Setup

1. Copy `.env.example` to `.env` and adjust the values for your environment
2. Place your certificates in the `certs` directory
3. Run the server binary (see architecture-specific paths above)

## Configuration

The following environment variables can be set in `.env`:

- `JAX_PORT`: The GRPC Port
- `JAX_GRPC_HOST`: The host and port of the GRPC server `:50051` for all interfaces
- `POLYGON_API_KEY`: API key for Massive.com (formerly Polygon.io)
- `JAX_DEX_CACHE_TTL`: Cache TTL for DEX data (supported formats: Ns, Nm, Nh)
- `JAX_MARKET_CACHE_TTL`: Cache TTL for Market data (supported formats: Ns, Nm, Nh)
- `JAX_AGGREGATE_CACHE_TTL`: Cache TTL for Aggregate trades (supported formats: Ns, Nm, Nh)
- `JAX_NUM_EXECUTORS`: Number of background scheduler tasks to run

## Resource notes (t3.nano / t4g.nano)

Both instance types provide 2 vCPUs and 512 MiB RAM (often shared with the Node.js frontend). Defaults in `confluence-configs/settings.yaml` are tuned for this footprint. On t4g.nano, use the same tuning knobs; only the CPU architecture differs.
