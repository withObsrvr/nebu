# Shape: Token Transfer Origin Processor + gRPC Server

**Appetite:** 1 week
**Status:** Ready to bet
**Depends on:** Core Runtime & RPC Source

---

## Problem

We need a concrete example that shows nebu actually works for real Stellar data. The Token Transfer Processor from Stellar's SDK is perfect because:
- It's production-ready code we can wrap
- Token transfers are universally useful
- It demonstrates the full origin → events flow
- It's complex enough to be real, simple enough to understand

This becomes the "ship" - the reference implementation everyone copies.

---

## Solution (Fat-Marker Sketch)

Build the first complete processor + service:

**`processors/core/token_transfer/processor.go`**
- Wraps Stellar's `token_transfer.EventsProcessor`
- Implements nebu's `Origin` interface
- Calls `EventsFromLedger()` and emits events

**`processors/core/token_transfer/proto/`**
- Simple gRPC service definition
- `GetEvents(start, end) returns stream TokenTransferEvent`
- Use Stellar's existing `token_transfer.proto` for the events themselves

**`processors/core/token_transfer/server.go`**
- gRPC server implementation
- On each `GetEvents` request: streams ledgers from RPC → runs processor → sends events back
- Clean shutdown on context cancel

**`cmd/nebu-ttpd/main.go`**
- Standalone daemon
- Configurable via env vars (RPC URL, listen address, network)
- Graceful shutdown on SIGTERM

**`processors/core/token_transfer/manifest.yaml`**
- Registry metadata (even though registry doesn't exist yet)
- Documents inputs, outputs, requirements

---

## Rabbit Holes

**Don't build filtering yet** - Client can filter events themselves. Adding "only USDC" or contract filters is a transform processor's job.

**Don't optimize the gRPC streaming** - Simple send-one-at-a-time is fine. Batching can wait.

**Don't add authentication/authorization** - Open gRPC for MVP. Security comes later.

**Don't create multiple origin variants** - One processor for token transfers is enough. AMM events, Soroban logs, etc. can wait.

---

## No-Gos

- ❌ No filtering or transformation logic in the origin
- ❌ No persistence/caching
- ❌ No metrics or monitoring
- ❌ No authentication
- ❌ No rate limiting
- ❌ No support for multiple concurrent streams (single request at a time is fine)

---

## Done Looks Like

**Running the service:**
```bash
export NEBU_RPC_URL=https://mainnet.sorobanrpc.com
./nebu-ttpd
# Listening on :50051...
```

**Client test (using grpcurl):**
```bash
grpcurl -plaintext \
  -d '{"start_ledger": 58155263, "end_ledger": 58155280}' \
  localhost:50051 \
  nebu.ttp.TokenTransferService/GetEvents
```

Returns a stream of `TokenTransferEvent` messages with transfers, mints, burns, fees.

**Or a simple Go client:**
```go
conn, _ := grpc.Dial("localhost:50051", grpc.WithInsecure())
client := token_transferpb.NewTokenTransferServiceClient(conn)

stream, _ := client.GetEvents(ctx, &token_transferpb.GetEventsRequest{
    StartLedger: 58155263,
    EndLedger: 58155280,
})

for {
    event, err := stream.Recv()
    if err == io.EOF { break }
    fmt.Printf("Event: %+v\n", event)
}
```

**What good looks like:**
- Service starts cleanly
- Connects to Stellar RPC
- Streams events without errors for a known good ledger range
- Handles client disconnect gracefully
- Shuts down cleanly on Ctrl+C

---

## Scope Line

### MUST HAVE ════════════════
- `Origin` processor wrapping Stellar's token_transfer
- gRPC service with `GetEvents` RPC
- `nebu-ttpd` binary that runs the service
- Proto definitions + code generation
- Manifest file documenting the processor
- Integration test that verifies events stream correctly

### NICE TO HAVE ─────────────
- Example client in Go showing usage
- Environment-based configuration
- Better error messages when RPC is unreachable
- Logging of ledger progress

### COULD HAVE ───────────────
- Example client in Node.js or Python
- Docker image for the service
- Prometheus metrics
- Structured logging with levels

---

## Notes

This is the **proof of concept**. If we can ship this cleanly, we've validated:
- The runtime abstraction works
- The processor contract makes sense
- gRPC + protobuf is the right choice
- We can wrap Stellar's SDK components easily

Everything after this is "more processors" and "better DX." This is the ship that proves nebu is real.

Once this lands, we can show it off and get early feedback before building the full CLI.
