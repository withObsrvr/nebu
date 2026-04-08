# nebu

**Query live Stellar data with one command.**

Unix pipes for blockchain indexing. No complex infrastructure required.

nebu (pronounced "neh-buh") streams Stellar ledger data through composable processors. Pipe to `jq` for filtering, `duckdb` for SQL analytics, or build custom pipelines with tools you already know.

Named after the Nebuchadnezzar from The Matrix, nebu is the vessel that carries data from the on-chain truth to your applications.

## Status

🚀 **v0.4.0** - Production-hardened with retry, auto-resume, conditional upserts, and metrics

**What's new in v0.4.0:**
- **Retry with backoff**: `postgres-sink --max-retries 5 --retry-backoff 5s` survives transient DB failures
- **Auto-resume**: `nebu resume --dsn $DSN --table events -- "pipeline..."` picks up where you left off
- **Conditional upserts**: `--conflict update-if-newer` prevents data regression during backfills
- **Prometheus metrics**: `--metrics-port 9090` exposes `/metrics` for operational visibility
- **ledger_sequence column**: Explicit column with index for efficient resume queries

**What's included:**
- **CLI & Fetch**: `nebu fetch` for streaming XDR from RPC or historical archives (GCS/S3)
- **Origin Processors**: `token-transfer`, `contract-events`, `contract-invocation` - extract typed events from ledgers
- **Transform Processors**: `usdc-filter`, `amount-filter`, `dedup`, `time-window` - filter and transform event streams
- **Sink Processors**: `json-file-sink`, `nats-sink`, `postgres-sink` - route events to storage or message queues
- **Registry System**: `nebu list` and `nebu install` for processor discovery and installation
- **Archive Mode**: Fetch historical data from Google Cloud Storage for backfilling
- **Authentication**: Premium RPC endpoint support with `NEBU_RPC_AUTH`
- **Follow Mode**: Live-streaming with `--follow` (like `tail -f`)

**Coming in v0.4:**
- Hasura GraphQL integration (examples available now in `examples/hasura`)
- Additional origin processors (AMM pools, liquidity events)
- External processor support (install from any git repo)
- Performance improvements for archive fetching

## Quick Start

**Get running in 2 minutes:**

### Option A: Using `go install` (Recommended)

```bash
# 1. Install nebu CLI (10 seconds)
go install github.com/withObsrvr/nebu/cmd/nebu@latest

# 2. Add Go bin to PATH (if not already done)
export PATH="$HOME/go/bin:$PATH"

# 3. Install token-transfer processor (30 seconds)
nebu install token-transfer

# 4. See results! (30 seconds - processes 2 ledgers)
token-transfer --start-ledger 60200000 --end-ledger 60200001
```

### Option B: Clone and Build (For Development)

```bash
# 1. Clone and install nebu CLI (30 seconds)
git clone https://github.com/withObsrvr/nebu && cd nebu
make install

# 2. Add Go bin to PATH (if not already done)
export PATH="$HOME/go/bin:$PATH"

# 3. Install token-transfer processor (builds locally)
nebu install token-transfer

# 4. See results!
token-transfer --start-ledger 60200000 --end-ledger 60200001
```

**Output:** You'll see newline-delimited JSON events streaming to stdout, like:
```json
{"_schema":"nebu.token-transfer.v1","_nebu_version":"0.4.0","meta":{"ledgerSequence":60200000,"closedAt":"2025-12-08T01:45:11Z","txHash":"abc...","transactionIndex":1,"contractAddress":"CA..."},"transfer":{"from":"GA...","to":"GB...","asset":{"issuedAsset":{"assetCode":"USDC","issuer":"GA..."}},"amount":"1000000"}}
```

**Next steps - Build pipelines:**

```bash
# Pipe to jq for filtering
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.transfer.asset.issuedAsset.assetCode == "USDC")'

# Pipe to DuckDB for SQL analytics
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "SELECT COUNT(*) as transfers FROM read_json('/dev/stdin') WHERE transfer IS NOT NULL"

# Separate fetching from processing (reusable data)
nebu fetch 60200000 60200100 > ledgers.xdr
cat ledgers.xdr | token-transfer | jq 'select(.transfer != null)'
```

### As a Go Library

```go
package main

import (
    "context"
    "fmt"

    "github.com/stellar/go-stellar-sdk/xdr"
    "github.com/withObsrvr/nebu/pkg/processor"
    "github.com/withObsrvr/nebu/pkg/runtime"
    "github.com/withObsrvr/nebu/pkg/source/rpc"
)

// Simple processor that counts ledgers
type Counter struct{ count int }

func (c *Counter) Name() string                 { return "counter" }
func (c *Counter) Type() processor.Type         { return processor.TypeOrigin }
func (c *Counter) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
    c.count++
    fmt.Printf("Processed ledger %d\n", ledger.LedgerSequence())
    return nil
}

func main() {
    // Connect to Stellar RPC
    src, _ := rpc.NewLedgerSource("https://archive-rpc.lightsail.network")
    defer src.Close()

    // Create your processor
    counter := &Counter{}

    // Run!
    rt := runtime.NewRuntime()
    rt.RunOrigin(context.Background(), src, counter, 60200000, 60200009)

    fmt.Printf("Processed %d ledgers\n", counter.count)
}
```

## Getting Started

Common commands to get you started with nebu:

**Extract token transfers from Stellar:**

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100
```

**Filter events with jq (USDC transfers only):**

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.transfer.asset.issuedAsset.assetCode == "USDC")'
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
      json_extract_string(transfer, '$.asset.issuedAsset.assetCode') as asset,
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

**Fetch from historical archives (GCS/S3) for data lakehouse:**

```bash
nebu fetch --mode archive \
  --bucket-path "my-bucket/stellar/ledgers" \
  60200000 60300000 | gzip > historical.xdr.gz
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
  jq -c 'select(.transfer.asset.issuedAsset.assetCode == "USDC")' | \
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
    ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error
}
```

**Transform** - Consumes events, emits transformed events *(coming soon)*

**Sink** - Consumes events, produces side effects (DB writes, etc.)
```go
type Sink interface {
    ConsumeEvent(ctx context.Context, event interface{}) error
}
```

### Sources

Sources stream ledger data:

```go
src, err := source.NewRPCLedgerSource("https://archive-rpc.lightsail.network")
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

nebu ships with example processors in [`examples/processors/`](./examples/processors/):

### Origin Processors
- **[token-transfer](./examples/processors/token-transfer/)** - Stream token transfer events (transfers, mints, burns, clawbacks, fees)

### Sink Processors
- **[json-file-sink](./examples/processors/json-file-sink/)** - Write events to JSONL files (simplest sink)

**💡 DuckDB users:** See the [DuckDB Cookbook](docs/DUCKDB_COOKBOOK.md) for piping events directly to DuckDB without custom sinks

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

## Project Structure

```
nebu/
├── pkg/
│   ├── source/     # RPC & ledger sources
│   ├── processor/  # Processor interfaces
│   ├── runtime/    # Pipeline execution
│   └── registry/   # Processor discovery
├── examples/
│   ├── processors/    # Example processor implementations
│   │   ├── token-transfer/  # Origin: token transfers
│   │   └── json-file-sink/  # Sink: JSONL file storage
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
cat ledgers.xdr | token-transfer | jq 'select(.transfer.asset.issuedAsset.assetCode == "USDC")'
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
  jq 'select(.transfer.asset.issuedAsset.assetCode == "USDC")'
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
  jq 'select(.transfer.amount > 1000000)'
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
- `_schema`: Schema version identifier (e.g., `nebu.token-transfer.v1`)
- `_nebu_version`: The nebu CLI version that produced the event (e.g., `0.4.0`)

```json
{
  "_schema": "nebu.token-transfer.v1",
  "_nebu_version": "0.4.0",
  "meta": {
    "ledgerSequence": 60200000,
    "closedAt": "2025-12-08T01:45:11Z",
    "txHash": "abc...",
    "transactionIndex": 1,
    "contractAddress": "CA..."
  },
  "transfer": {
    "from": "GA...",
    "to": "GB...",
    "asset": {
      "issuedAsset": {
        "assetCode": "USDC",
        "issuer": "GA..."
      }
    },
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

Each processor documents its schema in `SCHEMA.md`:
- [Token Transfer Schema Documentation](./examples/processors/token-transfer/SCHEMA.md)

### Using Schema Versions

```bash
# Filter events by schema version in DuckDB
duckdb analytics.db -c "
  SELECT * FROM transfers
  WHERE _schema = 'nebu.token-transfer.v1'
"

# Check nebu version distribution
duckdb analytics.db -c "
  SELECT _nebu_version, COUNT(*) as count
  FROM transfers
  GROUP BY _nebu_version
"

# Filter with jq
cat events.jsonl | jq 'select(._schema == "nebu.token-transfer.v1")'
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
cat ledgers.xdr | token-transfer | jq 'select(.type == "transfer")'
cat ledgers.xdr | token-transfer | duckdb-sink --db events.db
```

### Archive Mode (GCS/S3)

For historical data and data lakehouse building, use archive mode to fetch ledgers directly from cloud storage:

```bash
# Fetch from GCS bucket
nebu fetch --mode archive \
  --datastore-type GCS \
  --bucket-path "my-bucket/stellar/ledgers" \
  60200000 60300000 > ledgers.xdr

# Fetch from S3 bucket
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "my-s3-bucket/path/to/ledgers" \
  --region us-west-2 \
  60200000 60300000 > ledgers.xdr

# Use environment variables for configuration
export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=GCS
export NEBU_BUCKET_PATH="obsrvr-stellar-data/ledgers/mainnet"
export NEBU_BUFFER_SIZE=200
export NEBU_NUM_WORKERS=20

# Fetch and compress for data lake
nebu fetch 1000000 2000000 | gzip > historical.xdr.gz

# Pipe to processors (same as RPC mode)
nebu fetch 60200000 60200100 | token-transfer | jq -c 'select(.transfer)'
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

# Or build manually
go build -o bin/json-file-sink ./examples/processors/json-file-sink/cmd/json-file-sink
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  ./bin/json-file-sink --out events.jsonl

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
      json_extract_string(transfer, '$.asset.issuedAsset.assetCode') as asset,
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

Processors are discovered through `registry.yaml` in the project root. This lightweight approach keeps nebu's core minimal while supporting extensibility.

### Registry Format

```yaml
version: 1
processors:
  - name: token-transfer
    type: origin
    description: Stream token transfer events from Stellar ledgers
    location:
      type: local
      path: ./examples/processors/token-transfer
    proto:
      source: github.com/stellar/go-stellar-sdk/protos/processors/token_transfer
    manifest: ./examples/processors/token-transfer/manifest.yaml
```

### Adding Your Own Processor

1. **Create your processor** following the interfaces in `pkg/processor/`
2. **Add to registry.yaml**:
   ```yaml
   - name: my-processor
     type: origin  # or transform, sink
     description: What it does
     location:
       type: local
       path: ./path/to/processor
     maintainer:
       name: Your Name
       url: https://github.com/yourname
   ```
3. **Install and run it**:
   ```bash
   nebu install my-processor
   my-processor --start-ledger 60200000 --end-ledger 60200100
   ```

### Community Processor Registry

Browse community-contributed processors at the [nebu Community Processor Registry](https://github.com/withObsrvr/nebu-processor-registry).

The community registry is a directory of processors built by the community:
- Each processor lives in its own GitHub repository
- Submissions are validated automatically (builds, tests, documentation)
- Processors are maintained by their authors, not the nebu core team

**Discovering community processors:**
```bash
# Browse at: https://github.com/withObsrvr/nebu-processor-registry
# View processor list: https://github.com/withObsrvr/nebu-processor-registry/blob/main/PROCESSORS.md
```

**Installing community processors (currently manual):**
```bash
# Clone and build from processor's repository
git clone https://github.com/user/awesome-processor
cd awesome-processor
go build -o $GOPATH/bin/awesome-processor ./cmd

# Use like any other processor
awesome-processor --start-ledger 60200000 --end-ledger 60200100 | jq
```

**Contributing your processor:**

See the [Contributing Guide](https://github.com/withObsrvr/nebu-processor-registry/blob/main/CONTRIBUTING.md) for submission guidelines.

### Future: External Processors

The registry will support installing directly from git repos:

```bash
# Future: install community processors with one command
nebu install awesome-processor  # Clones from git, builds, installs
```

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
