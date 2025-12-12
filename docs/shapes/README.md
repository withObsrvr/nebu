# nebu Shape Up Documents

This directory contains Shape Up documents for improving nebu's Unix philosophy adherence.

## Overview

These shapes make nebu more composable, following the Unix philosophy of "do one thing well" and "work together through text streams."

## Shapes (Recommended Order)

### Quick Wins (1-2 days each)
1. **[Quiet Mode](./03-quiet-mode.md)** - Add `--quiet` flag (Rule of Silence)
   - Appetite: 1 day
   - Impact: Immediate improvement for scripts/pipelines

2. **[Explicit stdin Marker](./05-explicit-stdin-marker.md)** - Support `-` for stdin
   - Appetite: 1 day
   - Impact: Better Unix conventions
   - Depends: Shape 02

### Foundation (2-3 days each)
3. **[stdin Input Support](./02-stdin-input-support.md)** - Processors accept pipes
   - Appetite: 2 days
   - Impact: Enables caching and reprocessing

4. **[Separate Fetch from Process](./01-separate-fetch-from-process.md)** - `nebu fetch` command
   - Appetite: 3 days
   - Impact: True separation of concerns
   - Depends: Shape 02

5. **[Standalone Processor Binaries](./06-standalone-processor-binaries.md)** - Install processors globally
   - Appetite: 3 days
   - Impact: Processors feel like native Unix tools

### Advanced (5 days)
6. **[Transform/Sink as CLI Tools](./04-transform-sink-as-cli-tools.md)** - Full pipeline composability
   - Appetite: 5 days
   - Impact: Complete Unix-style processing chains

## Implementation Strategy

### Cycle 1: Quick Wins (3 days)
- Shape 01: Quiet Mode (1 day)
- Shape 05: Explicit stdin (1 day)
- Ship: Cleaner CLI for scripts

### Cycle 2: Foundation (5 days)
- Shape 02: stdin Support (2 days)
- Shape 01: Separate Fetch (3 days)
- Ship: Cacheable ledgers, composable fetching

### Cycle 3: Standalone Tools (3 days)
- Shape 06: Install processors as binaries
- Ship: `token-transfer` as a standalone command

### Cycle 4: Full Composability (5 days)
- Shape 04: Transform/Sink CLI tools
- Ship: End-to-end pipelines

Total: ~16 days across 4 cycles (or cherry-pick based on needs)

## gRPC Compatibility

**All shapes maintain gRPC compatibility.** These improvements focus on:
- **Local execution mode**: stdin/stdout, Unix pipes
- **CLI improvements**: Better command-line ergonomics

gRPC processors (remote execution) remain fully supported:
- Local mode: `cat ledgers | token-transfer | postgres-sink`
- Remote mode: `nebu run --transform grpc://filter:9000 --sink grpc://postgres:9001`
- Hybrid mode: Mix local and remote processors

The processor interfaces (Origin, Transform, Sink) don't change. These shapes add new execution modes without removing existing ones.

## Success Metrics

After implementing all shapes:

**Before:**
```bash
# Tightly coupled
nebu run origin token-transfer --start 60200000 --end 60200100 > events.jsonl
```

**After:**
```bash
# Composable Unix pipeline
nebu fetch 60200000 60200100 > ledgers.xdr
cat ledgers.xdr | token-transfer | usdc-filter | postgres-sink

# Or simplified
nebu fetch 60200000 60200100 | token-transfer | duckdb events.db
```

## Questions?

Each shape document includes:
- Problem statement
- Time appetite
- Solution sketch
- Rabbit holes to avoid
- Definition of "done"
- gRPC compatibility notes

Start with the quick wins, then build up to full composability.
