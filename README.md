# nebu

**A minimal, IDL-first streaming runtime for Stellar**

nebu (pronounced "neh-boo") is a lightweight framework for building modular data pipelines on Stellar. It provides the plumbing that connects Stellar RPC to your custom processors, enabling you to build indexers, analytics pipelines, and real-time automation.

Named after the Nebuchadnezzar from The Matrix, nebu is the vessel that carries data from the on-chain truth to your applications.

## Status

🚧 **Alpha (v0.2.0)** - Token Transfer processor shipped, under active development

Currently shipping:
- ✅ RPC ledger source
- ✅ Processor interfaces (Origin, Transform, Sink)
- ✅ Runtime for wiring source → processor
- ✅ Token Transfer origin processor
- ✅ HTTP/JSON streaming service (`nebu-ttpd`)
- ✅ Working examples

Coming soon:
- CLI tooling (`nebu run`, `nebu new`)
- Additional origin processors (Soroban events, AMM)
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

**Sink** - Consumes events, produces side effects (DB writes, etc.) *(coming soon)*

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

## Examples

See the [`examples/`](./examples/) directory for working examples:

- [`simple_origin`](./examples/simple_origin/) - Count and print ledger info

Run an example:
```bash
go run examples/simple_origin/main.go
```

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
│   └── runtime/    # Pipeline execution
├── examples/       # Working examples
└── Makefile
```

## Design Principles

1. **Minimal Core** - nebu does one thing: stream data through processors
2. **IDL-First** - All processors communicate via protobuf messages
3. **Community Extensible** - Anyone can build and share processors
4. **No Lock-In** - Works with any infrastructure

## Using the Token Transfer Service

nebu ships with `nebu-ttpd`, a standalone HTTP service for streaming token transfer events:

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

**Cycle 3** (Next) - Basic CLI
- `nebu run origin` command
- `nebu new processor` scaffolding

**Future**
- Transform & Sink processors
- Pipeline YAML specs
- Community processor registry
- More origin processors (Soroban events, AMM, etc.)

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
