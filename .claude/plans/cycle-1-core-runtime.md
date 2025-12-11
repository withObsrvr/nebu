# Cycle 1 Implementation Plan: Core Runtime & RPC Source

**Appetite:** 1 week (5 working days)
**Shape:** [01-core-runtime.md](../.claude/shapes/01-core-runtime.md)
**Start Date:** TBD
**Status:** Ready to start

---

## Overview

Build the foundational runtime that connects Stellar RPC to processors. This is pure plumbing - no UI, no fancy features, just solid infrastructure.

**Success = A working example that streams ledgers from RPC through a simple origin processor**

---

## The Hill

```
Figuring it out          │          Making it happen
    (0-50%)              │              (50-100%)
─────────────────────────┼─────────────────────────
                         │
```

**Target: Reach 50% (top of hill) by end of Day 2**

If we're still on the left side by Day 3, we cut scope immediately.

---

## Work Breakdown

### Day 1: Foundation & RPC Connection (Left Side 🔍)

**Goal:** Get RPC streaming working end-to-end

#### Task 1.1: Project scaffolding
- [ ] Initialize Go module: `go mod init github.com/withObsrvr/nebu`
- [ ] Create directory structure:
  ```
  nebu/
  ├── pkg/source/
  ├── pkg/processor/
  ├── pkg/runtime/
  ├── examples/
  └── Makefile
  ```
- [ ] Set up basic `Makefile` with `test`, `build`, `lint` targets
- [ ] Add `.gitignore` for Go projects

**Time box:** 30 minutes
**Cut if stuck:** Skip Makefile, use go commands directly

---

#### Task 1.2: Implement `pkg/source/source.go` interface
- [ ] Define `LedgerSource` interface
  ```go
  type LedgerSource interface {
      Stream(ctx context.Context, start, end uint32, out chan<- xdr.LedgerCloseMeta) error
      Close() error
  }
  ```
- [ ] Add godoc comments
- [ ] Commit: "Add LedgerSource interface"

**Time box:** 15 minutes

---

#### Task 1.3: Implement `pkg/source/rpc_source.go`
- [ ] Add Stellar dependencies:
  ```bash
  go get github.com/stellar/go/ingest/ledgerbackend
  go get github.com/stellar/go/xdr
  ```
- [ ] Create `RPCLedgerSource` struct
- [ ] Implement `NewRPCLedgerSource(rpcURL string)`
- [ ] Implement `Stream()` method:
  - Validate start/end are non-zero
  - Call `PrepareRange(BoundedRange(start, end))`
  - Loop through ledgers with `GetLedger()`
  - Send to channel with context cancellation support
- [ ] Implement `Close()` to clean up backend
- [ ] Commit: "Implement RPCLedgerSource"

**Time box:** 2 hours
**Acceptance:** Can instantiate source and call Stream() without panicking

---

#### Task 1.4: Write first integration test
- [ ] Create `pkg/source/rpc_source_test.go`
- [ ] Test: Stream 5 ledgers from public RPC
  ```go
  func TestRPCLedgerSource_Stream(t *testing.T) {
      src, err := NewRPCLedgerSource("https://mainnet.sorobanrpc.com")
      require.NoError(t, err)
      defer src.Close()

      ch := make(chan xdr.LedgerCloseMeta, 10)
      ctx := context.Background()

      go src.Stream(ctx, 58155263, 58155267, ch)

      count := 0
      for range ch {
          count++
      }
      assert.Equal(t, 5, count)
  }
  ```
- [ ] Run test: `go test ./pkg/source/...`
- [ ] Commit: "Add RPC source integration test"

**Time box:** 1 hour
**Cut if stuck:** Skip test, manually verify with simple main.go instead

---

**End of Day 1 checkpoint:**
- ✅ Can stream ledgers from Stellar RPC
- ✅ Code compiles and basic test passes
- 🎯 Hill position: ~25%

---

### Day 2: Processor Interfaces & Runtime (Climb to Top 🔍)

**Goal:** Define processor contract and wire up runtime

#### Task 2.1: Define processor types
- [ ] Create `pkg/processor/types.go`
- [ ] Define `Type` enum (Origin, Transform, Sink)
- [ ] Define base `Processor` interface
- [ ] Add godoc with examples
- [ ] Commit: "Add processor type definitions"

**Time box:** 30 minutes

---

#### Task 2.2: Define Origin interface
- [ ] Create `pkg/processor/origin.go`
- [ ] Define `Origin` interface:
  ```go
  type Origin interface {
      Processor
      ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error
  }
  ```
- [ ] Add comprehensive godoc explaining the contract
- [ ] Commit: "Add Origin processor interface"

**Time box:** 20 minutes

---

#### Task 2.3: Build Emitter helper
- [ ] Create `pkg/processor/emitter.go`
- [ ] Implement generic `Emitter[T proto.Message]`:
  - `NewEmitter(bufSize int)`
  - `Emit(ev T)`
  - `Out() <-chan T`
  - `Close()`
- [ ] Add simple test for emitter behavior
- [ ] Commit: "Add Emitter helper for processors"

**Time box:** 45 minutes
**Cut if stuck:** This is NICE-TO-HAVE, skip and use raw channels

---

#### Task 2.4: Implement Runtime
- [ ] Create `pkg/runtime/runner.go`
- [ ] Define `Runtime` struct (can be empty for now)
- [ ] Implement `NewRuntime()`
- [ ] Implement `RunOrigin()`:
  ```go
  func (r *Runtime) RunOrigin(
      ctx context.Context,
      src source.LedgerSource,
      origin processor.Origin,
      start, end uint32,
  ) error
  ```
- [ ] Wire up: source → channel → origin.ProcessLedger() loop
- [ ] Handle context cancellation properly
- [ ] Add error propagation
- [ ] Commit: "Implement Runtime.RunOrigin"

**Time box:** 2 hours
**Acceptance:** Can wire source + origin and process ledgers end-to-end

---

**End of Day 2 checkpoint:**
- ✅ All interfaces defined
- ✅ Runtime can wire source → origin
- 🎯 Hill position: **50% (TOP OF HILL)**
- ⚠️ Decision point: Are we confident in the abstractions?

---

### Day 3: Example Origin & Integration (Right Side ⚡)

**Goal:** Prove it works with a concrete example

#### Task 3.1: Build simple example origin
- [ ] Create `examples/simple_origin/main.go`
- [ ] Implement trivial origin that counts ledgers:
  ```go
  type CountingOrigin struct {
      count int
  }

  func (o *CountingOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
      o.count++
      fmt.Printf("Processed ledger %d (total: %d)\n", ledger.LedgerSequence(), o.count)
      return nil
  }
  ```
- [ ] Wire with runtime and run against real RPC
- [ ] Commit: "Add counting origin example"

**Time box:** 1 hour

---

#### Task 3.2: Build event-emitting example
- [ ] Create `examples/event_origin/main.go`
- [ ] Implement origin that emits basic events (ledger sequence + close time)
- [ ] Use `Emitter[*LedgerEvent]` helper
- [ ] Consume events in main and print
- [ ] Commit: "Add event-emitting origin example"

**Time box:** 1.5 hours
**Cut if behind:** Skip this, counting example is enough

---

#### Task 3.3: Add cancellation test
- [ ] Create `pkg/runtime/runner_test.go`
- [ ] Test: Context cancellation stops streaming mid-range
- [ ] Test: Origin receives expected number of ledgers
- [ ] Commit: "Add runtime cancellation tests"

**Time box:** 1 hour

---

**End of Day 3 checkpoint:**
- ✅ Working examples demonstrate the API
- ✅ Tests prove cancellation works
- 🎯 Hill position: ~75%

---

### Day 4: Polish & Documentation (Right Side ⚡)

**Goal:** Make it production-ready

#### Task 4.1: Error handling pass
- [ ] Review all error returns
- [ ] Add context to errors: `fmt.Errorf("failed to get ledger %d: %w", seq, err)`
- [ ] Ensure all resources clean up on error paths
- [ ] Test error scenarios (invalid range, RPC unreachable)
- [ ] Commit: "Improve error handling and messages"

**Time box:** 2 hours

---

#### Task 4.2: Add logging
- [ ] Create `pkg/logging/logger.go` - simple interface wrapper
- [ ] Add optional logger to Runtime
- [ ] Log key events: stream start, ledger processed, errors
- [ ] Keep it minimal (no dependencies like zap/logrus yet)
- [ ] Commit: "Add basic logging support"

**Time box:** 1 hour
**Cut if behind:** COULD-HAVE, skip for MVP

---

#### Task 4.3: Documentation
- [ ] Add package-level godoc to `pkg/source`
- [ ] Add package-level godoc to `pkg/processor`
- [ ] Add package-level godoc to `pkg/runtime`
- [ ] Write `examples/README.md` explaining the examples
- [ ] Commit: "Add package documentation"

**Time box:** 1.5 hours
**Cut if behind:** Minimal inline comments only

---

**End of Day 4 checkpoint:**
- ✅ Error handling is solid
- ✅ Code is documented
- 🎯 Hill position: ~90%

---

### Day 5: Final Testing & Ship (Right Side ⚡)

**Goal:** Confidence to ship

#### Task 5.1: Extended integration test
- [ ] Test streaming 100+ ledgers from mainnet
- [ ] Test with testnet RPC
- [ ] Verify memory doesn't grow unbounded
- [ ] Test cancellation at various points
- [ ] Commit: "Add extended integration tests"

**Time box:** 2 hours

---

#### Task 5.2: README and examples
- [ ] Create top-level `README.md`:
  - What is nebu (1 paragraph)
  - Quick example
  - Link to examples/
  - Current status (MVP)
- [ ] Ensure examples run without modification
- [ ] Commit: "Add project README"

**Time box:** 1 hour

---

#### Task 5.3: Final review & cleanup
- [ ] Run `go fmt ./...`
- [ ] Run `go vet ./...`
- [ ] Run all tests
- [ ] Check for TODOs that should be resolved
- [ ] Remove dead code
- [ ] Tag: `v0.1.0-alpha.1`

**Time box:** 1 hour

---

#### Task 5.4: Prep for next cycle
- [ ] Create branch for Cycle 2 work
- [ ] Update project board
- [ ] Document any learnings/changes for next shape

**Time box:** 30 minutes

---

**End of Day 5:**
- ✅ Shipped: Core runtime with working examples
- ✅ Tagged release
- 🎯 Hill position: **100% DONE**

---

## Scope Management

### If we're behind by Day 3 midday, cut in this order:

1. **FIRST CUT:** Emitter helper → use raw channels
2. **SECOND CUT:** Event-emitting example → counting example only
3. **THIRD CUT:** Logging support → just use fmt.Printf
4. **FOURTH CUT:** Extended docs → minimal inline comments only

### If we're ahead:

1. Add buffer size configuration to Runtime
2. Add simple benchmarks
3. Add more test coverage
4. Start prototyping token_transfer processor (peek at Cycle 2)

---

## Definition of Done

**Must have all of these to ship:**

- [ ] `pkg/source/` compiles with no errors
- [ ] `pkg/processor/` defines Origin interface
- [ ] `pkg/runtime/` implements RunOrigin
- [ ] At least one working example in `examples/`
- [ ] Integration test proves RPC → Origin flow works
- [ ] Cancellation test passes
- [ ] README.md explains what this is
- [ ] Code is formatted and vetted
- [ ] Tagged as v0.1.0-alpha.1

**Nice to have (don't block ship):**

- [ ] Emitter helper
- [ ] Multiple examples
- [ ] Comprehensive godoc
- [ ] Logging support

---

## Daily Standups (Solo)

Each morning, write 3 bullets:
1. **Where on the hill?** (e.g., "35%, still figuring out channel wiring")
2. **What's blocking?** (e.g., "Nothing" or "RPC rate limits in tests")
3. **Scope cuts needed?** (e.g., "None yet" or "Dropping emitter helper")

---

## Success Metrics

At the end of this cycle, you should be able to:

1. Show someone this code:
   ```go
   src, _ := source.NewRPCLedgerSource("https://mainnet.sorobanrpc.com")
   origin := &MyOrigin{}
   runtime.NewRuntime().RunOrigin(ctx, src, origin, 100, 200)
   ```

2. Run an example that processes real Stellar ledgers

3. Point to clean, understandable interfaces for processors

4. Say "this is the foundation, token transfers build on this next week"

---

## Risks

### Risk: RPC rate limiting breaks tests
**Mitigation:** Use bounded ranges of 5-10 ledgers. If still blocked, mock the backend.

### Risk: Stellar SDK API changes or is incompatible
**Mitigation:** Pin to specific version in go.mod. Document version used.

### Risk: Channel design doesn't support backpressure well
**Mitigation:** Buffered channels + context cancellation is enough for MVP. Perfect backpressure is v2.

### Risk: Abstractions feel over-engineered
**Mitigation:** Keep interfaces minimal. If in doubt, remove methods.

---

## Post-Cycle Review Questions

After shipping, answer these:

1. Did the interfaces feel right when building examples?
2. What was harder than expected?
3. What should we change before Cycle 2?
4. Did we cut the right scope?
5. Are we still confident in the overall nebu vision?

---

## Next Cycle Preview

**Cycle 2:** Token Transfer Origin Processor + gRPC Server
- Wraps Stellar's token_transfer.EventsProcessor
- Implements nebu Origin interface
- Serves events over gRPC
- This is where nebu becomes useful to others

But that's next week. This week: nail the foundation.
