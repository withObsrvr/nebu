# nebu

**A toolkit for building Stellar indexers.** Nebu packages Stellar's modern indexing primitives — especially RPC-backed ledger access, ingest SDK processing, and XDR-native extraction — into a stable Go contract, standalone processors, and Unix-composable pipelines.

nebu (pronounced "neh-boo") is built on the supported building blocks Stellar provides for modern indexing. It turns those primitives into a practical toolkit for developers, operators, and agents.

nebu is two things in one repo:

1. A small, **stable Go contract** ([`pkg/processor`](./pkg/processor) and [`pkg/source`](./pkg/source)) that anyone can implement to write a Stellar processor — origin, transform, or sink. External processors live in their own Go modules and depend only on this contract; they get a CLI, a JSON-schema-emitting `--describe-json` protocol, and runtime observability hooks for free.
2. A **CLI** for discovering, installing, and composing standalone processors with `jq`, DuckDB, and other Unix tools.

Reference and community processors live in the [nebu processor registry](https://github.com/withObsrvr/nebu-processor-registry). The small processors under [`examples/processors/`](./examples/processors/) are educational examples that are not published by the external registry. Build your own processor against `pkg/processor`, ship it as its own binary, register it via [`description.yml`](./docs/REGISTRY_SPEC.md), and `nebu install your-processor` works.

Named after the Nebuchadnezzar from The Matrix, nebu is the vessel that carries data from the on-chain truth to your applications.

## Two ways in

**I want to build a custom processor →**
The contract is in [`pkg/processor`](./pkg/processor) (just `Processor`, `Origin`, `Transform`, `Sink`, `Emitter[T]`, `Reporter`) and [`pkg/source`](./pkg/source) (`LedgerSource`). Both are committed-stable per [docs/STABILITY.md](./docs/STABILITY.md), enforced by a CI check against committed API snapshots in [`.api/`](./.api/). The full proto-first walkthrough lives in the registry repo: [BUILDING_PROTO_PROCESSORS.md](https://github.com/withObsrvr/nebu-processor-registry/blob/main/BUILDING_PROTO_PROCESSORS.md). For runtime extensibility (metrics, tracing, progress bars, agent gating), see [docs/HOOKS.md](./docs/HOOKS.md).

**I want to query Stellar data →** Jump to [Quick Start](#quick-start) below. You'll have JSON events streaming in two minutes.

## Website and release notes

- **Website / quickstart:** [nebu.withobsrvr.com](https://nebu.withobsrvr.com)
- **Latest release:** [v0.6.8](https://github.com/withObsrvr/nebu/releases/tag/v0.6.8)
- **Changelog:** [CHANGELOG.md](./CHANGELOG.md)

**Processors:**
- **Published origins, transforms, and sinks:** [nebu processor registry](https://github.com/withObsrvr/nebu-processor-registry/tree/main/processors)
- **In-repo educational origins:** [`transaction-stats`](./examples/processors/transaction-stats), [`ledger-change-stats`](./examples/processors/ledger-change-stats)

**Coming next:**
- Serializable pipeline descriptors (IndexDescriptor) as the canonical hand-off format between nebu and AI agents
- Presets catalog of well-known ledger ranges, contract IDs, and pipeline templates
- Contract module extraction (`pkg/processor` → `nebu-api`) — deferred until external demand justifies the migration cost

## Quick Start

The canonical quickstart now lives on the website:

- **Query data:** https://nebu.withobsrvr.com/quickstart.html
- **Build processors:** https://nebu.withobsrvr.com/build-processors.html

For GitHub readers, here's the shortest successful local path today:

```bash
go install github.com/withObsrvr/nebu/cmd/nebu@latest
export PATH="$HOME/go/bin:$PATH"

nebu install token-transfer
token-transfer --start-ledger 60200000 --end-ledger 60200001
```

If you only want to preview output before setting up Go, you can also run the published Docker image:

```bash
docker run --rm withobsrvr/nebu:latest \
  token-transfer --start-ledger 60200000 --end-ledger 60200001
```

For development inside this repo:

```bash
git clone https://github.com/withObsrvr/nebu && cd nebu
make install
export PATH="$HOME/go/bin:$PATH"

nebu install token-transfer
token-transfer --start-ledger 60200000 --end-ledger 60200001
```

**Output:** You'll see newline-delimited JSON events streaming to stdout, like:
```json
{"_schema":"nebu.token_transfer.v1","_nebu_version":"v0.6.2","meta":{"ledgerSequence":60200000,"closedAtUnix":"1765158311","txHash":"abc...","transactionIndex":1,"contractAddress":"CA..."},"transfer":{"from":"GA...","to":"GB...","assetCode":"USDC","assetIssuer":"GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN","amount":"1000000"}}
```

**Next steps - Build pipelines:**

```bash
# Pipe to jq for filtering
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.transfer.assetCode == "USDC")'

# Pipe to DuckDB for SQL analytics
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "SELECT COUNT(*) as transfers FROM read_json('/dev/stdin') WHERE transfer IS NOT NULL"

# Separate fetching from processing (reusable data)
nebu fetch 60200000 60200100 > ledgers.xdr
cat ledgers.xdr | token-transfer | jq 'select(.transfer != null)'
```

### As a Go Library

Nebu can be embedded as a Go library on top of Stellar's ingest primitives. See these runnable examples:

- [`examples/simple_origin/main.go`](./examples/simple_origin/main.go) — minimal origin + runtime wiring
- [`examples/go-library/transaction-stats/main.go`](./examples/go-library/transaction-stats/main.go) — transaction statistics over an RPC-backed ledger range
- [`examples/go-library/ledger-change-stats/main.go`](./examples/go-library/ledger-change-stats/main.go) — ledger change statistics over an RPC-backed ledger range
- [`examples/go-library/README.md`](./examples/go-library/README.md) — how the Go examples relate to their standalone CLI processor forms

Run them with:

```bash
go run ./examples/simple_origin/main.go
go run ./examples/go-library/transaction-stats/main.go
go run ./examples/go-library/ledger-change-stats/main.go
```

The same ideas are also packaged as example standalone Nebu processors:

- [`examples/processors/transaction-stats`](./examples/processors/transaction-stats)
- [`examples/processors/ledger-change-stats`](./examples/processors/ledger-change-stats)

## Getting Started

Common commands to get you started with nebu:

**Extract token transfers from Stellar:**

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100
```

**Filter events with jq (USDC transfers only):**

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.transfer.assetCode == "USDC")'
```

**Stream continuously from a ledger (like `tail -f`):**

```bash
token-transfer --start-ledger 60200000 --follow
```

**Send events to multiple destinations (NATS, file, and terminal):**

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  tee >(nats-sink --subject "stellar.transfers" --jetstream) | \
  tee >(json-file-sink --out transfers.jsonl) | \
  jq -r '"Ledger \(.meta.ledgerSequence): \(.transfer.amount)"'
```

**Analyze with SQL using DuckDB:**

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    SELECT
      json_extract_string(transfer, '$.assetCode') as asset,
      COUNT(*) as count,
      SUM(CAST(json_extract_string(transfer, '$.amount') AS DOUBLE)) as volume
    FROM read_json('/dev/stdin')
    WHERE transfer IS NOT NULL
    GROUP BY asset
    ORDER BY volume DESC
  "
```

**Fetch raw ledger XDR (separating fetch from processing):**

```bash
nebu fetch 60200000 60200100 > ledgers.xdr
cat ledgers.xdr | token-transfer | jq
```

**Fetch from historical archives (public AWS S3 bucket, no credentials):**

```bash
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "aws-public-blockchain/v1.1/stellar/ledgers/pubnet" \
  --region us-east-2 \
  62080000 62081000 | gzip > historical.xdr.gz
```

**Use premium RPC endpoints with authentication:**

```bash
export NEBU_RPC_AUTH="Api-Key YOUR_API_KEY"
token-transfer --start-ledger 60200000 --end-ledger 60200100 \
  --rpc-url https://rpc-pubnet.nodeswithobsrvr.co
```

**Build a complete pipeline (extract → filter → dedupe → store):**

```bash
token-transfer --start-ledger 60200000 --follow | \
  jq -c 'select(.transfer.assetCode == "USDC")' | \
  dedup --key meta.txHash | \
  json-file-sink --out usdc-transfers.jsonl
```

**List and install processors:**

```bash
nebu list
nebu install token-transfer
nebu install json-file-sink
```

## Installation

### For Users: `go install` (Recommended)

Install nebu without cloning the repository:

```bash
# Install nebu CLI
go install github.com/withObsrvr/nebu/cmd/nebu@latest

# Add Go bin to PATH (if not already done)
export PATH="$HOME/go/bin:$PATH"

# Verify installation
nebu --version
```

**How it works:**
- `nebu` CLI embeds the processor registry
- `nebu list` works immediately (no repo needed)
- `nebu install <processor>` automatically uses `go install` for processors

### For Developers: Clone and Build

For local development or contributing:

```bash
# Clone the repository
git clone https://github.com/withObsrvr/nebu
cd nebu

# Install nebu CLI
make install

# Or build locally without installing
make build-cli
./bin/nebu --version
```

**How it works:**
- Uses file system `registry.yaml` (can be edited)
- `nebu install <processor>` builds from local source
- Perfect for developing new processors

### PATH Setup

Both methods install binaries to `$GOPATH/bin` (typically `~/go/bin`). Add to PATH:

```bash
# Add to ~/.bashrc, ~/.zshrc, or ~/.profile
export PATH="$HOME/go/bin:$PATH"

# Reload configuration
source ~/.bashrc

# Verify
nebu --version
```

**Without PATH modification:**
```bash
# Use full paths
~/go/bin/nebu fetch 60200000 60200100 | ~/go/bin/token-transfer
```

### As a Go Library

```bash
go get github.com/withObsrvr/nebu
```

## Core Concepts

### Processors

Processors come in three types:

**Origin** - Consumes ledgers from Stellar, emits typed events
```go
type Origin interface {
    ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta)
}
```

**Transform** - Consumes events, emits transformed events

**Sink** - Consumes events, produces side effects (DB writes, etc.)
```go
type Sink interface {
    WriteEvent(ctx context.Context, event proto.Message)
}
```

### Sources

`pkg/source` defines the stable `LedgerSource` interface. Concrete implementations live in unstable subpackages such as `pkg/source/rpc` and `pkg/source/storage`:

```go
src, err := rpc.NewLedgerSource("https://archive-rpc.lightsail.network")
defer src.Close()

// Stream ledgers to a channel
ch := make(chan xdr.LedgerCloseMeta, 128)
src.Stream(ctx, 60200000, 60200100, ch)
```

### Runtime

The runtime wires everything together:

```go
rt := runtime.NewRuntime()
rt.RunOrigin(ctx, source, processor, startLedger, endLedger)
```

## Example Processors

Published processors live in the [nebu processor registry](https://github.com/withObsrvr/nebu-processor-registry/tree/main/processors). This repo retains two educational origins:

- [`transaction-stats`](./examples/processors/transaction-stats) — summarize transactions over a ledger range
- [`ledger-change-stats`](./examples/processors/ledger-change-stats) — summarize ledger-entry changes

**💡 DuckDB users:** See the [DuckDB Cookbook](docs/DUCKDB_COOKBOOK.md) for piping events directly to DuckDB without custom sinks.

### Basic Examples
- [`simple_origin`](./examples/simple_origin/) - Count and print ledger info

Run an example:
```bash
go run examples/simple_origin/main.go
```

**💡 Want to see Unix-style pipeline examples?** Check out [PIPELINE.md](./PIPELINE.md) for examples using `jq`, `tee`, filtering, and multi-sink fanouts.

See the [Processor Registry](#processor-registry) section to learn how processors are discovered and run.

## Architecture

```
┌───────────────────┐
│   Stellar RPC     │
└────────┬──────────┘
         │ LedgerCloseMeta (XDR)
         ▼
┌────────────────────┐
│      ORIGIN        │  (e.g., token-transfer)
│  (extracts events) │
└────────┬───────────┘
         │ typed events (JSON)
         ▼
┌────────────────────┐
│     TRANSFORM      │  (e.g., usdc-filter, dedup)
│  (filters/modify)  │
└────────┬───────────┘
         │ filtered events
         ▼
┌────────────────────┐
│       SINK         │  (e.g., json-file-sink, duckdb)
│  (stores/outputs)  │
└────────────────────┘

All processors communicate via Unix pipes (stdin/stdout)
```

## Development

```bash
# Run tests
make test

# Run all tests including integration
go test -v ./...

# Format code
make fmt

# Lint (requires golangci-lint)
make lint

# Run example
make run-example
```

To verify a built application against a live Stellar RPC server without starting
an unbounded stream, run the [bounded RPC end-to-end smoke
test](docs/SMOKE_TESTING.md). It fetches three recent ledgers and validates the
resulting schema-versioned JSONL.

## Project Structure

```
nebu/
├── pkg/
│   ├── source/     # RPC & ledger sources
│   ├── processor/  # Processor interfaces
│   ├── runtime/    # Pipeline execution
│   └── registry/   # Processor discovery
├── examples/
│   ├── processors/    # Educational processor implementations
│   │   ├── transaction-stats/
│   │   └── ledger-change-stats/
│   └── simple_origin/ # Basic usage example
├── cmd/
│   └── nebu/       # CLI tool
├── registry.yaml   # Processor registry
└── Makefile
```

## Features

### ⚡ Blazing Fast

Written in Go. Decouples fetching from processing. **Backfill 5 years of history in hours, not months**, by parallelizing `fetch` workers.

```bash
# Separate fetch from processing - reuse XDR across multiple processors
nebu fetch 60200000 60300000 > ledgers.xdr

# Process the same data multiple ways (no repeated RPC calls)
cat ledgers.xdr | token-transfer | jq 'select(.transfer.assetCode == "USDC")'
cat ledgers.xdr | contract-events | grep -i "swap"
```

### 🔧 The Unix Way

No heavy databases required. nebu respects `stdin` and `stdout`. Pipe directly into **DuckDB** for instant SQL analytics, **jq** for filtering, or any tool you want.

```bash
# Instant SQL analytics - no database setup
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "SELECT COUNT(*) FROM read_json('/dev/stdin') WHERE transfer IS NOT NULL"

# Filter with jq
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.transfer.assetCode == "USDC")'
```

### 🌐 SaaS Ready (NATS)

Bridge the gap between CLI and Cloud. Use `nats-sink` to turn your local pipeline into a distributed, real-time firehose for your API.

```bash
# Stream live events to NATS for your application
token-transfer --start-ledger 60200000 --follow | \
  nats-sink --subject "stellar.transfers" --jetstream

# Multiple destinations with tee
token-transfer --start-ledger 60200000 --follow | \
  tee >(nats-sink --subject "stellar.live") | \
  tee >(json-file-sink --out archive.jsonl) | \
  jq 'select(.transfer != null and (.transfer.amount | tonumber) > 1000000)'
```

## Design Principles

nebu is optimized for **simplicity and speed** - get from idea to results in minutes, not hours.

1. **Unix Philosophy** - Processors are composable via stdin/stdout pipes; each does one thing well
2. **Minimal Core** - nebu provides the runtime; processors are separate and composable
3. **CLI-Focused** - No service management, no orchestration, no YAML files - just commands and pipes
4. **JSON Wire Format** - Processors communicate via newline-delimited JSON (easy to debug, works with `jq`, DuckDB, etc.)
5. **Registry-Based Discovery** - Processors are registered in `registry.yaml`, not bundled
6. **Community Extensible** - Anyone can build and share processors
7. **Fast Prototyping** - Two-minute setup, instant results, easy debugging

**Note on Protobuf:** Processors use protobuf structs _internally_ (from Stellar SDK) for type safety, but output JSON for CLI compatibility. Future flowctl integration would use native protobuf over gRPC.

### When to Use nebu vs flowctl

**Use nebu for:**
- Quick prototyping and ad-hoc analysis
- Single-machine pipelines with Unix pipes
- Piping to `jq`, DuckDB, shell scripts
- Simple Go processors
- When you want results in 2 minutes

**Use [flowctl](https://github.com/withObsrvr/flowctl) for:**
- Production pipelines with service orchestration
- Multi-language processors (Python, Rust, TypeScript)
- Distributed systems across multiple machines
- Full observability (metrics, health checks, tracing)
- Complex DAG topologies

See [docs/ARCHITECTURE_DECISIONS.md](./docs/ARCHITECTURE_DECISIONS.md) for the full rationale, [docs/STABILITY.md](./docs/STABILITY.md) for which package surfaces are committed stable for external processors, and [docs/REGISTRY_SPEC.md](./docs/REGISTRY_SPEC.md) for the formal `registry.yaml` v1 and `description.yml` v1 schemas.

## Schema Versioning

nebu includes schema version information in all JSON output to prevent silent breakage when formats change.

Every JSON event includes:
- `_schema`: Schema version identifier (e.g., `nebu.token_transfer.v1`)
- `_nebu_version`: The nebu CLI version that produced the event (e.g., `0.4.0`)

```json
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "0.4.0",
  "meta": {
    "ledgerSequence": 60200000,
    "closedAtUnix": "1765158311",
    "txHash": "abc...",
    "transactionIndex": 1,
    "contractAddress": "CA..."
  },
  "transfer": {
    "from": "GA...",
    "to": "GB...",
    "assetCode": "USDC",
    "assetIssuer": "GA...",
    "amount": "1000000"
  }
}
```

### Why Schema Versioning?

When you pipe nebu output to DuckDB, jq, or other tools, those tools rely on the JSON schema. If nebu renames a field (e.g., `from` → `from_address`), your queries break. Schema versioning lets you:

- **Detect format changes**: Filter by `_schema` version in queries
- **Handle migrations**: Process old and new formats separately
- **Track provenance**: Know which nebu version produced your data

### Schema Version Policy

- **Breaking changes** (field renames, removals, type changes) → increment version (v1 → v2)
- **Non-breaking changes** (new fields, new event types) → keep version (stay at v1)

Each published processor documents its schema. For example:
- [Token Transfer Schema Documentation](https://github.com/withObsrvr/nebu-processor-registry/blob/main/processors/token-transfer/SCHEMA.md)

### Using Schema Versions

```bash
# Filter events by schema version in DuckDB
duckdb analytics.db -c "
  SELECT * FROM transfers
  WHERE _schema = 'nebu.token_transfer.v1'
"

# Check nebu version distribution
duckdb analytics.db -c "
  SELECT _nebu_version, COUNT(*) as count
  FROM transfers
  GROUP BY _nebu_version
"

# Filter with jq
cat events.jsonl | jq 'select(._schema == "nebu.token_transfer.v1")'
```

## Using the CLI

nebu provides a CLI for processor discovery, installation, and ledger fetching. **Processors run as standalone binaries** to keep nebu minimal.

### List available processors

```bash
# Show all processors in registry
nebu list

# Output:
# NAME              TYPE    LOCATION  DESCRIPTION
# token-transfer    origin  local     Stream token transfer events from Stellar...
```

### Install and run processors

Processors are **not embedded** in the nebu binary. Install them first:

```bash
# Install a processor
nebu install token-transfer

# This builds and installs the processor to $GOPATH/bin
# Output: Installed: /home/user/go/bin/token-transfer

# Run the processor directly (bounded range)
token-transfer --start-ledger 60200000 --end-ledger 60200100

# Stream continuously from ledger 60200000 (unbounded)
token-transfer --start-ledger 60200000

# Output is newline-delimited JSON
token-transfer --start-ledger 60200000 --end-ledger 60200001 | jq

# Use custom RPC endpoint and network
token-transfer \
  --start-ledger 60200000 \
  --end-ledger 60200100 \
  --rpc-url https://rpc-pubnet.nodeswithobsrvr.co \
  --network mainnet

# Use testnet
token-transfer \
  --start-ledger 100 \
  --end-ledger 200 \
  --network testnet
```

### Fetch ledgers (without processing)

Use `nebu fetch` to download raw ledger XDR that can be piped to processors:

```bash
# Fetch bounded range
nebu fetch 60200000 60200100 --output ledgers.xdr

# Fetch unbounded (stream continuously from ledger 60200000)
nebu fetch 60200000 0 > ledgers.xdr

# Or pipe directly to a processor
nebu fetch 60200000 60200100 | token-transfer

# This separates ledger fetching from processing,
# allowing you to process the same data multiple times
nebu fetch 60200000 60200100 > ledgers.xdr
cat ledgers.xdr | token-transfer | jq 'select(.transfer != null)'
cat ledgers.xdr | token-transfer | duckdb -c "SELECT COUNT(*) FROM read_json('/dev/stdin') WHERE transfer IS NOT NULL"
```

### Archive Mode (GCS/S3)

For historical data and data lakehouse building, use archive mode to fetch ledgers directly from cloud storage:

```bash
# Default: public AWS Stellar archive (pubnet, no AWS account required)
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "aws-public-blockchain/v1.1/stellar/ledgers/pubnet" \
  --region us-east-2 \
  62080000 62081000 > ledgers.xdr

# Private S3 bucket (uses standard AWS credential chain)
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "my-org-archive/stellar/ledgers/pubnet" \
  --region us-west-2 \
  60200000 60300000 > ledgers.xdr

# GCS bucket (uses Application Default Credentials)
nebu fetch --mode archive \
  --datastore-type GCS \
  --bucket-path "my-gcs-archive/landing/ledgers/pubnet" \
  60200000 60300000 > ledgers.xdr

# Use environment variables for configuration
export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=S3
export NEBU_BUCKET_PATH="aws-public-blockchain/v1.1/stellar/ledgers/pubnet"
export NEBU_REGION=us-east-2
export NEBU_BUFFER_SIZE=200
export NEBU_NUM_WORKERS=20

# Fetch and compress for data lake
nebu fetch 62000000 62100000 | gzip > historical.xdr.gz

# Pipe to processors (same as RPC mode)
nebu fetch 62080000 62080100 | token-transfer | jq -c 'select(.transfer)'
```

**Archive Mode Benefits:**
- **Full history access**: Read any ledger from Stellar's complete history
- **High performance**: 100-500 ledgers/sec vs 10-20 for RPC (configurable workers and buffering)
- **Cost effective**: Direct bucket access without RPC overhead or rate limits
- **Lakehouse ready**: Perfect for building Bronze layer data lakes

**Archive Configuration Options:**
- `--mode`: `rpc` (default) or `archive`
- `--datastore-type`: `GCS` or `S3`
- `--bucket-path`: Path to bucket containing ledger files
- `--region`: S3 region (required for S3, ignored for GCS)
- `--buffer-size`: Number of ledgers to cache (default: 100)
- `--num-workers`: Parallel fetch workers (default: 10)

**Archive Environment Variables:**
- `NEBU_MODE`
- `NEBU_DATASTORE_TYPE`
- `NEBU_BUCKET_PATH`
- `NEBU_REGION`
- `NEBU_BUFFER_SIZE`
- `NEBU_NUM_WORKERS`

### Configuration & Environment Variables

Configure processors and `nebu fetch` via flags or environment variables:

```bash
# Set defaults via environment (applies to both nebu fetch and processors)
export NEBU_RPC_URL="https://rpc-pubnet.nodeswithobsrvr.co"
export NEBU_NETWORK="mainnet"

# Run processor without specifying flags (uses environment)
token-transfer --start-ledger 60200000 --end-ledger 60200100

# Or use with nebu fetch
nebu fetch 60200000 60200100 | token-transfer
```

**Available environment variables:**
- `NEBU_RPC_URL` - Stellar RPC endpoint (default: `https://archive-rpc.lightsail.network`)
- `NEBU_NETWORK` - Network: `mainnet`, `testnet`, or full passphrase (default: mainnet)
- `NEBU_RPC_AUTH` - RPC authorization header value (e.g., `Api-Key YOUR_KEY`)

**RPC Authorization:**

Many premium RPC endpoints require authorization headers. Processors and `nebu fetch` support this via environment variables:

```bash
# Using environment variable (recommended for secrets)
export NEBU_RPC_AUTH="Api-Key YOUR_API_KEY_HERE"

# Works with processors
token-transfer \
  --rpc-url https://rpc-pubnet.nodeswithobsrvr.co \
  --start-ledger 60200000 --end-ledger 60200100

# Works with nebu fetch
nebu fetch 60200000 60200100 \
  --rpc-url https://rpc-pubnet.nodeswithobsrvr.co \
  --output ledgers.xdr

# Or pipe fetch to processor
nebu fetch 60200000 60200100 \
  --rpc-url https://rpc-pubnet.nodeswithobsrvr.co | token-transfer
```

**Quiet mode:**

Suppress informational output for scripting:

```bash
# With processors
token-transfer --quiet --start-ledger 60200000 --end-ledger 60200100 | jq

# With nebu fetch
nebu fetch --quiet 60200000 60200100 | token-transfer --quiet | jq
```

### Build a Pipeline

Stream events from origin processors into sink processors using Unix pipes:

```bash
# Install processors
nebu install token-transfer
nebu install json-file-sink

# Stream token transfers into a JSON file
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  json-file-sink --out events.jsonl

# Or build the registry processor manually
go install github.com/withObsrvr/nebu-processor-registry/processors/json-file-sink/cmd/json-file-sink@latest
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  json-file-sink --out events.jsonl

# Query the events
cat events.jsonl | jq 'select(.transfer != null) | {from: .transfer.from, to: .transfer.to, amount: .transfer.amount}'
```

For DuckDB integration, see the [DuckDB Cookbook](docs/DUCKDB_COOKBOOK.md).

## DuckDB Integration

DuckDB excels at analyzing nebu event streams via Unix pipes - often **replacing hundreds of lines of custom processor code with a single SQL query**.

**Quick example:**
```bash
# Stream events directly into DuckDB for analysis
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    SELECT
      json_extract_string(transfer, '$.assetCode') as asset,
      COUNT(*) as transfers,
      SUM(CAST(json_extract_string(transfer, '$.amount') AS DOUBLE)) as volume
    FROM read_json('/dev/stdin')
    WHERE transfer IS NOT NULL
    GROUP BY asset
    ORDER BY volume DESC
  "
```

**Why use DuckDB instead of custom processors?**
- Iterate in seconds (modify query vs recompile Go)
- Built-in aggregations, window functions, joins, exports
- Save reusable queries to `examples/queries/*.sql`
- Zero maintenance - no code to maintain

**See the [DuckDB Cookbook](docs/DUCKDB_COOKBOOK.md) for:**
- Extracting nested JSON from contract events
- Time-series analysis with window functions
- Multi-table analytics
- Export to CSV/Parquet/JSON
- Incremental updates
- Real-world query examples

## Processor Registry

Published processors are discovered from the external [nebu processor registry](https://github.com/withObsrvr/nebu-processor-registry). `registry.yaml` contains only processors implemented in this repository.

### Registry Format

See [REGISTRY_SPEC.md](./docs/REGISTRY_SPEC.md) and the registry's [`description.yml` files](https://github.com/withObsrvr/nebu-processor-registry/tree/main/processors).

### Adding Your Own Processor

1. **Create your processor** following the interfaces in `pkg/processor/`.
2. **Publish it through the external registry** using a `description.yml`.
3. **Install and run it**:
   ```bash
   nebu install my-processor
   my-processor --start-ledger 60200000 --end-ledger 60200100
   ```

### Community Processor Registry

Browse published and community-contributed processors at the [nebu Processor Registry](https://github.com/withObsrvr/nebu-processor-registry).

The external registry is a directory of standalone processors:
- processors are discovered automatically
- `nebu list` shows both in-repo educational and external processors
- `nebu install <name>` works as long as the processor's Go module or prebuilt release installs cleanly
- processors are maintained in the registry or by their authors, not in the nebu core repository

**Fastest way to use community processors:**
```bash
# Install nebu CLI
go install github.com/withObsrvr/nebu/cmd/nebu@latest
export PATH="$HOME/go/bin:$PATH"

# Browse everything available
nebu list

# Install a community processor
nebu install account-filter

# Use it in a pipeline
token-transfer --start-ledger 60200000 --end-ledger 60200001 | \
  account-filter --account GABC... | jq
```

**If you want to browse or build directly from the registry repo:**
```bash
# Clone the registry metadata + processor source repo
git clone https://github.com/withObsrvr/nebu-processor-registry
cd nebu-processor-registry

# Build a processor from the cloned repo
cd processors/account-filter
go install ./cmd/account-filter

# Then use the installed binary
account-filter --help
```

**Important:** cloning the registry repo alone does not install the `nebu` CLI. End users who just want data should usually install `nebu` first, then use `nebu list` / `nebu install`.

**Contributing your processor:**

See the [Contributing Guide](https://github.com/withObsrvr/nebu-processor-registry/blob/main/CONTRIBUTING.md) for submission guidelines.

## Roadmap

**Cycle 1** - Core Runtime ✅
- RPC source
- Processor interfaces
- Basic runtime
- Examples

**Cycle 2** - Token Transfer Processor ✅
- Wrap Stellar's token_transfer.EventsProcessor
- HTTP/JSON streaming service
- Integration tests

**Cycle 3** - CLI and Processor Infrastructure ✅
- `nebu install` command for building processors
- `nebu fetch` command for ledger XDR streaming
- `nebu list` for processor discovery
- Standalone processor binaries (not embedded in nebu)
- Registry-based processor management
- Schema versioning
- RPC authentication support
- Community processor registry

**Current Focus**
- Additional origin processors (Soroban events, AMM, DEX)
- More transform processor examples
- External processor support (install from git repos)
- Performance optimizations

## Contributing

nebu is under active development. Contributions welcome!

### Core nebu contributions
- Source improvements (RPC, ledger handling)
- Runtime enhancements
- CLI features
- Documentation and examples

### Processor contributions

**Building processors?** Submit them to the [Community Processor Registry](https://github.com/withObsrvr/nebu-processor-registry)!

We especially need:
- **Origin processors**: Soroban events, AMM operations, DEX trades, etc.
- **Transform processors**: Filtering, aggregation, enrichment
- **Sink processors**: Postgres, Kafka, TimescaleDB, ClickHouse, etc.

See the [Processor Contribution Guide](https://github.com/withObsrvr/nebu-processor-registry/blob/main/CONTRIBUTING.md) for details.

## License

MIT

## About OBSRVR

nebu is built by [OBSRVR](https://withobsrvr.com) as part of the Stellar ecosystem infrastructure.

---

**nebu** - /ˈnɛ.buː/ - *noun* - The vessel that carries you between worlds
