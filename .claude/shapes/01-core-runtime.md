# Shape: Core Runtime & RPC Source

**Appetite:** 1 week
**Status:** Ready to bet

---

## Problem

We need the foundational plumbing that connects Stellar RPC to processors. Without this, we have nothing to build on. Every Stellar data pipeline needs to:
- Pull ledger data from RPC reliably
- Stream it through processors
- Handle backpressure and cancellation gracefully

Right now there's no minimal framework that does just this and nothing more.

---

## Solution (Fat-Marker Sketch)

Build three tiny packages that form the runtime core:

**`pkg/source`** - Abstraction over Stellar RPC
- `LedgerSource` interface
- `RPCLedgerSource` implementation wrapping Stellar's `ledgerbackend.RPCLedgerBackend`
- Streams `LedgerCloseMeta` XDR into a channel
- Bounded ranges only for MVP (tail mode is out of scope)

**`pkg/processor`** - Core processor contracts
- `Processor` base interface (Name, Type)
- `Origin` interface that consumes `LedgerCloseMeta`
- `Emitter[T]` helper for typed event channels
- Keep Transform/Sink interfaces as TODOs for now

**`pkg/runtime`** - The glue
- `Runtime.RunOrigin()` that wires source → origin processor
- Handle context cancellation cleanly
- Simple channel-based flow (no fancy orchestration)

---

## Rabbit Holes

**Don't implement tail mode yet** - Bounded ranges [start, end] are enough. Following the tip is a nice-to-have that can wait.

**Don't build pipeline orchestration** - We're not wiring multiple processors together yet. Just source → single origin.

**Don't add metrics/observability** - Console logs are fine. Structured logging/metrics come later.

**Don't optimize for performance** - Correctness first. Batching, parallel processing, etc. can wait.

---

## No-Gos

- ❌ No transforms or sinks yet
- ❌ No YAML pipeline specs
- ❌ No registry/discovery
- ❌ No distributed mode/gRPC yet
- ❌ No retry logic or error recovery

---

## Done Looks Like

A working Go program that:

```go
func main() {
    ctx := context.Background()

    src, _ := source.NewRPCLedgerSource("https://mainnet.sorobanrpc.com")
    defer src.Close()

    origin := &SimpleOrigin{} // prints ledger numbers

    rt := runtime.NewRuntime()
    rt.RunOrigin(ctx, src, origin, 58155263, 58155280)
}
```

When run:
- Connects to Stellar RPC
- Pulls ledgers 58155263-58155280
- Calls `origin.ProcessLedger()` for each
- Handles cancellation via Ctrl+C
- Exits cleanly

**Tests:**
- Source streams expected number of ledgers
- Cancellation stops streaming immediately
- Invalid ranges return errors

---

## Scope Line

### MUST HAVE ════════════════
- `LedgerSource` interface + RPC implementation
- `Processor` + `Origin` interfaces
- `Runtime.RunOrigin()` with bounded range support
- Context cancellation
- Basic error propagation
- At least one integration test hitting real RPC

### NICE TO HAVE ─────────────
- Emitter helper (makes downstream easier but not critical)
- Structured logging
- Better error messages
- Unit tests (vs just integration tests)

### COULD HAVE ───────────────
- Connection retry logic
- Configurable buffer sizes
- Performance benchmarks

---

## Notes

This is the foundation. Everything else builds on this. Keep it absolutely minimal - resist any urge to add "just one more thing."

Once this is solid, the Token Transfer processor becomes trivial to wire in.
