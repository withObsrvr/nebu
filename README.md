# nebu

**A minimal, IDL-first streaming runtime for Stellar**

nebu (pronounced "neh-boo") is a lightweight framework for building modular data pipelines on Stellar. It provides the plumbing that connects Stellar RPC to your custom processors, enabling you to build indexers, analytics pipelines, and real-time automation.

Named after the Nebuchadnezzar from The Matrix, nebu is the vessel that carries data from the on-chain truth to your applications.

## Status

🚧 **Alpha (v0.3.0)** - Lightweight runtime with registry-based processor discovery

Currently shipping:
- ✅ RPC ledger source
- ✅ Processor interfaces (Origin, Transform, Sink)
- ✅ Runtime for wiring source → processor
- ✅ Registry-based processor discovery
- ✅ CLI for running and scaffolding processors
- ✅ Example processors (token-transfer, duckdb-sink)
- ✅ HTTP/JSON streaming service (`nebu-ttpd`)

Coming soon:
- Additional origin processors (Soroban events, AMM)
- Transform processor examples
- External processor support (git, go modules)
- Community processor registry

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    "github.com/stellar/go-stellar-sdk/xdr"
    "github.com/withObsrvr/nebu/pkg/processor"
    "github.com/withObsrvr/nebu/pkg/runtime"
    "github.com/withObsrvr/nebu/pkg/source"
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
    src, _ := source.NewRPCLedgerSource("https://mainnet.sorobanrpc.com")
    defer src.Close()

    // Create your processor
    counter := &Counter{}

    // Run!
    rt := runtime.NewRuntime()
    rt.RunOrigin(context.Background(), src, counter, 60200000, 60200009)

    fmt.Printf("Processed %d ledgers\n", counter.count)
}
```

## Installation

### As a library
```bash
go get github.com/withObsrvr/nebu
```

### CLI tool
```bash
# Install nebu command
make install

# Or build locally
make build-cli
./bin/nebu --version
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
src, err := source.NewRPCLedgerSource("https://mainnet.sorobanrpc.com")
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
- **[duckdb-sink](./examples/processors/duckdb-sink/)** - Write events to DuckDB for local analytics

### Basic Examples
- [`simple_origin`](./examples/simple_origin/) - Count and print ledger info

Run an example:
```bash
go run examples/simple_origin/main.go
```

See the [Processor Registry](#processor-registry) section to learn how processors are discovered and run.

## Architecture

```
┌───────────────────┐
│   Stellar RPC     │
└────────┬──────────┘
         │ LedgerCloseMeta (XDR)
         ▼
┌────────────────────┐
│      ORIGIN        │
│  (your processor)  │
└────────┬───────────┘
         │ typed events
         ▼
┌────────────────────┐
│     TRANSFORM      │  (future)
└────────┬───────────┘
         │
         ▼
┌────────────────────┐
│       SINK         │  (future)
└────────────────────┘
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
│   │   └── duckdb-sink/     # Sink: DuckDB storage
│   └── simple_origin/ # Basic usage example
├── cmd/
│   ├── nebu/       # CLI tool
│   └── nebu-ttpd/  # Token transfer HTTP service
├── registry.yaml   # Processor registry
└── Makefile
```

## Design Principles

1. **Minimal Core** - nebu provides the runtime; processors are separate and composable
2. **IDL-First** - All processors communicate via protobuf messages
3. **Registry-Based Discovery** - Processors are registered in `registry.yaml`, not bundled
4. **Community Extensible** - Anyone can build and share processors
5. **No Lock-In** - Works with any infrastructure

## Using the CLI

nebu provides a CLI for running processors and scaffolding new ones.

### List available processors

```bash
# Show all processors in registry
nebu list

# Output:
# NAME              TYPE    LOCATION                                    DESCRIPTION
# token-transfer    origin  ./examples/processors/token-transfer        Stream token transfer events from Stellar...
# duckdb-sink       sink    ./examples/processors/duckdb-sink           Sink processor that writes token transfer...
```

### Run a processor

```bash
# Stream token transfer events from ledgers
nebu run origin token-transfer --start-ledger 60200000 --end-ledger 60200100

# Output is newline-delimited JSON
nebu run origin token-transfer --start-ledger 60200000 --end-ledger 60200001 | jq

# Use custom RPC endpoint
nebu run origin token-transfer \
  --start-ledger 60200000 \
  --end-ledger 60200100 \
  --rpc-url https://rpc-pubnet.nodeswithobsrvr.co
```

### Build a Pipeline

Stream events from origin processors into sink processors using Unix pipes:

```bash
# Build a simple JSON file sink
go build -o bin/json-file-sink ./examples/processors/json-file-sink/cmd/

# Stream token transfers into a JSON file
nebu run origin token-transfer --start 60200000 --end 60200100 | \
  ./bin/json-file-sink --out events.jsonl

# Query the events
cat events.jsonl | jq 'select(.type == "transfer") | {from, to, amount, asset}'
```

For DuckDB sink (requires CGO):

```bash
# Build with Nix for proper dependencies
cd examples/processors/duckdb-sink
nix develop  # or: nix build

# Stream into DuckDB
nebu run origin token-transfer --start 60200000 --end 60200100 | \
  ./cmd/duckdb-sink --db events.db

# Query with DuckDB CLI
duckdb events.db "SELECT asset_code, COUNT(*) FROM token_transfer_events GROUP BY asset_code"
```

### Create a new processor

```bash
# Scaffold a new origin processor
nebu new processor my-indexer --type origin

# Scaffold a transform processor
nebu new processor usdc-filter --type transform

# Scaffold a sink processor
nebu new processor postgres-sink --type sink
```

This creates a directory with:
- `processor.go` - Your processor implementation
- `proto/*.proto` - Event schema definitions
- `manifest.yaml` - Processor metadata
- `README.md` - Documentation

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
     manifest: ./path/to/processor/manifest.yaml
   ```
3. **Run it**: `nebu run origin my-processor --start-ledger X --end-ledger Y`

### Future: External Processors

The registry will support external processors from git repos or go modules:

```yaml
processors:
  - name: soroban-events
    type: origin
    location:
      type: git
      url: https://github.com/withObsrvr/nebu-processor-soroban
    proto:
      source: github.com/withObsrvr/nebu-processor-soroban/proto
```

## Using the Token Transfer Service

You can also run `nebu-ttpd` as a standalone HTTP service:

```bash
# Build and run
make build-ttpd
./bin/nebu-ttpd

# Stream events from ledgers 60200000-60200100
curl "http://localhost:8080/events?start=60200000&end=60200100"

# Each line is a JSON event:
# {"type":"transfer","ledger_sequence":60200000,"tx_hash":"...","from":"...","to":"...","amount":"100.0","asset":{"code":"USDC","issuer":"..."}}
# {"type":"mint","ledger_sequence":60200001,"tx_hash":"...","to":"...","amount":"50.0","asset":{"code":"native"}}
```

Environment variables:
- `NEBU_RPC_URL` - Stellar RPC endpoint (default: mainnet)
- `NEBU_LISTEN_ADDR` - HTTP listen address (default: :8080)
- `NEBU_NETWORK` - Network passphrase (default: mainnet)

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

**Cycle 3** - Basic CLI ✅
- `nebu run origin` command with JSON output
- `nebu new processor` scaffolding
- Template generation for all processor types

**Future**
- Transform & Sink processor examples
- Pipeline YAML specs
- Community processor registry
- More origin processors (Soroban events, AMM, etc.)
- Multi-processor pipelines

## Contributing

nebu is under active development. Contributions welcome!

Areas we need help:
- Additional origin processors
- Transform processor examples
- Sink implementations (Postgres, Kafka, DuckDB, etc.)
- Documentation and examples

## License

MIT

## About OBSRVR

nebu is built by [OBSRVR](https://withobsrvr.com) as part of the Stellar ecosystem infrastructure.

---

**nebu** - /ˈnɛ.buː/ - *noun* - The vessel that carries you between worlds
