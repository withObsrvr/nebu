# nebu Shape Up Documents

This directory contains Shape Up documents for nebu improvements.

## Overview

These shapes cover two themes:
1. **Unix philosophy** (Cycles 1-5) - Making nebu more composable with stdin/stdout, pipes, and standalone tools
2. **Agent-friendly interface** (Cycle 6) - Better discoverability, error messages, and MCP wrapper for AI agents

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

### Agent-Friendly Interface (Cycle 6)
7. **[Better Error Messages](./07-better-error-messages.md)** - Actionable errors with suggestions
   - Appetite: 2 days
   - Impact: Faster iteration for humans and agents

8. **[Richer Help Text](./08-richer-help-text.md)** - Examples and patterns in --help
   - Appetite: 2 days
   - Impact: Self-documenting tools

9. **[Richer nebu list](./09-richer-nebu-list.md)** - Grouped output + describe command
   - Appetite: 2 days
   - Impact: Better processor discovery

10. **[MCP Wrapper](./10-mcp-wrapper.md)** - MCP server for AI agents
    - Appetite: 1 week
    - Impact: Safe, discoverable interface for agents

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

### Cycle 5: Already Complete
- See CYCLE_5_COMPLETE.md for details

### Cycle 6: Agent-Friendly Interface (2 weeks)
- Shape 07: Better Error Messages (2 days)
- Shape 08: Richer Help Text (2 days)
- Shape 09: Richer nebu list (2 days)
- Shape 10: MCP Wrapper (1 week)
- Ship: nebu that's great for humans AND AI agents

Total: ~30 days across 6 cycles (or cherry-pick based on needs)

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

## Agent-Friendly Design Philosophy

Cycle 6 makes nebu work well for AI agents **without changing** the human experience:

| Aspect | Humans | Agents |
|--------|--------|--------|
| **Interface** | CLI with Unix pipes | MCP server (wraps CLI) |
| **Output control** | `| head`, `| wc -l` | Built-in limits, format options |
| **Discovery** | `--help`, trial-and-error | `nebu_describe_processor` tool |
| **Errors** | Read and fix | Actionable suggestions |

The key insight: Unix composability **is** agent-friendly. Agents can pipe through `head`, filter with `jq`, and count with `wc -l` just like humans. The MCP wrapper just adds guardrails (default limits) and better discovery (tool schemas).

## Questions?

Each shape document includes:
- Problem statement
- Time appetite
- Solution sketch
- Rabbit holes to avoid
- Definition of "done"

Start with the quick wins, then build up to full composability. Cycle 6 can be done in parallel since it's orthogonal to the Unix philosophy shapes.
