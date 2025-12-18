# nebu Examples

This directory contains working examples demonstrating how to use nebu to build Stellar data pipelines.

## Running Examples

All examples can be run directly with `go run`:

```bash
go run examples/simple_origin/main.go
```

Or use the Makefile shortcut:

```bash
make run-example
```

## Examples

### `simple_origin` - Counting Origin Processor

**What it demonstrates:**
- Creating an RPC ledger source
- Implementing a basic origin processor
- Using the runtime to wire source → processor
- Processing a bounded range of ledgers
- Graceful shutdown on Ctrl+C

**Code walkthrough:**

1. **Define your processor** - Implement the `processor.Origin` interface:
   ```go
   type CountingOrigin struct {
       count int
   }

   func (o *CountingOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
       o.count++
       fmt.Printf("Processed ledger %d\n", ledger.LedgerSequence())
       return nil
   }
   ```

2. **Create the source** - Connect to Stellar RPC:
   ```go
   src, err := source.NewRPCLedgerSource("https://archive-rpc.lightsail.network")
   defer src.Close()
   ```

3. **Wire and run** - Use the runtime:
   ```go
   rt := runtime.NewRuntime()
   rt.RunOrigin(ctx, src, origin, 60200000, 60200009)
   ```

**Run it:**
```bash
go run examples/simple_origin/main.go
```

**Environment variables:**
- `NEBU_RPC_URL` - Override the RPC endpoint (default: mainnet)

## Coming Soon

### `token_transfers` - Token Transfer Events
- Stream token transfer events using Stellar's token_transfer processor
- Filter by asset type
- Emit structured protobuf events

### `event_emitter` - Using the Emitter Helper
- Demonstrate the `Emitter[T]` pattern
- Consume events from an origin processor
- Pipeline multiple processors

### `custom_sink` - Writing to External Systems
- Consume events and write to a database
- Batch processing
- Error handling and retry logic

## Building Your Own

To build your own processor:

1. **Implement the interface** - Origin, Transform, or Sink
2. **Handle context cancellation** - Respect `ctx.Done()`
3. **Return errors clearly** - Help debugging
4. **Test with real RPC** - Use recent ledger numbers

Example skeleton:

```go
package main

import (
    "context"
    "github.com/stellar/go-stellar-sdk/xdr"
    "github.com/withObsrvr/nebu/pkg/processor"
    "github.com/withObsrvr/nebu/pkg/runtime"
    "github.com/withObsrvr/nebu/pkg/source"
)

type MyProcessor struct{}

func (p *MyProcessor) Name() string { return "my-processor" }
func (p *MyProcessor) Type() processor.Type { return processor.TypeOrigin }

func (p *MyProcessor) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
    // Your logic here
    return nil
}

func main() {
    src, _ := source.NewRPCLedgerSource("https://archive-rpc.lightsail.network")
    defer src.Close()

    processor := &MyProcessor{}
    rt := runtime.NewRuntime()

    rt.RunOrigin(context.Background(), src, processor, 60200000, 60200009)
}
```

## Tips

**Finding recent ledger numbers:**
- RPC typically retains ~24 hours of ledgers
- Recent ledgers are in the 60M+ range (as of Dec 2024)
- Use smaller ranges (5-10 ledgers) for testing

**Performance:**
- Buffer sizes matter - see `pkg/source/rpc_source.go`
- Process ledgers quickly to avoid backpressure
- Consider using goroutines for I/O within processors

**Error handling:**
- Always check errors from `RunOrigin`
- Implement graceful shutdown with context cancellation
- Log errors with context (ledger number, timestamp, etc.)

## Questions?

See the main [README](../README.md) for core concepts and API docs.
