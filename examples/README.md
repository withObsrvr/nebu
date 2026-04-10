# nebu Examples

This directory contains runnable examples for building Stellar data pipelines with nebu.

## Running Examples

Run the shipped examples directly:

```bash
go run examples/simple_origin/main.go
go run examples/go-library/transaction-stats/main.go
go run examples/go-library/ledger-change-stats/main.go
```

Or via Make for the simple origin example:

```bash
make run-example
```

## Included Examples

### `simple_origin` — Counting Origin Processor

This example demonstrates:
- creating an RPC-backed ledger source
- implementing a basic `processor.Origin`
- wiring source → runtime → processor
- processing a bounded ledger range
- graceful shutdown on Ctrl+C

### `go-library/transaction-stats` — Transaction Statistics

This example demonstrates:
- embedding nebu as a Go library
- using `runtime.RunOrigin` over an RPC-backed range
- reading transactions from `xdr.LedgerCloseMeta`
- computing successful/failed transaction and operation counts

Run with:

```bash
go run examples/go-library/transaction-stats/main.go
```

### `go-library/ledger-change-stats` — Ledger Change Statistics

This example demonstrates:
- embedding nebu as a Go library
- reading ledger entry changes from `xdr.LedgerCloseMeta`
- classifying created / updated / deleted changes
- classifying fee / transaction / operation-caused changes

Run with:

```bash
go run examples/go-library/ledger-change-stats/main.go
```

### Standalone processor forms

The same ideas also exist as example standalone processors:
- `examples/processors/transaction-stats`
- `examples/processors/ledger-change-stats`

### Core shape

Implement `processor.Origin`:

```go
type CountingOrigin struct {
    count int
}

func (o *CountingOrigin) Name() string { return "counting-origin" }
func (o *CountingOrigin) Type() processor.Type { return processor.TypeOrigin }

func (o *CountingOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
    o.count++
    fmt.Printf("Processed ledger %d\n", ledger.LedgerSequence())
}
```

Create the source:

```go
src, err := rpc.NewLedgerSource("https://archive-rpc.lightsail.network")
if err != nil {
    panic(err)
}
defer src.Close()
```

Run it with the runtime:

```go
rt := runtime.NewRuntime()
err = rt.RunOrigin(ctx, src, origin, 60200000, 60200009)
```

### Environment variables

- `NEBU_RPC_URL` — override the default RPC endpoint

## Building Your Own

General guidelines:

1. Implement `processor.Origin`, `processor.Transform`, or `processor.Sink`
2. Respect `ctx.Done()` for cancellation
3. Report per-item problems via the processor reporter helpers instead of returning stream-killing errors
4. Test against a small real ledger range first

Example skeleton:

```go
package main

import (
    "context"

    "github.com/stellar/go-stellar-sdk/xdr"
    "github.com/withObsrvr/nebu/pkg/processor"
    "github.com/withObsrvr/nebu/pkg/runtime"
    "github.com/withObsrvr/nebu/pkg/source/rpc"
)

type MyProcessor struct{}

func (p *MyProcessor) Name() string { return "my-processor" }
func (p *MyProcessor) Type() processor.Type { return processor.TypeOrigin }

func (p *MyProcessor) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
    _ = ctx
    _ = ledger
}

func main() {
    src, err := rpc.NewLedgerSource("https://archive-rpc.lightsail.network")
    if err != nil {
        panic(err)
    }
    defer src.Close()

    proc := &MyProcessor{}
    rt := runtime.NewRuntime()

    if err := rt.RunOrigin(context.Background(), src, proc, 60200000, 60200009); err != nil {
        panic(err)
    }
}
```

## Tips

- Use small ranges first, e.g. 5–10 ledgers
- `pkg/source/rpc/source.go` is the reference RPC implementation
- Always check the error returned by `RunOrigin`

## Questions?

See the main [README](../README.md) for core concepts and API docs.
