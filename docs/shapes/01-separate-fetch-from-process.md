# Shape: Separate Fetch from Process

## Problem
Currently `nebu run origin` couples fetching ledgers from RPC with processing them. This violates Unix philosophy (do one thing well) and prevents:
- Caching expensive RPC calls for reuse
- Processing the same ledgers multiple times with different processors
- Testing processors with fixture data
- Using alternative ledger sources (files, S3, local network)

## Appetite
**3 days** - This is a focused extraction with clear boundaries

## Solution

Create a new `nebu fetch` command that only fetches ledgers from RPC and outputs them to stdout:

```bash
# Basic usage - output XDR to stdout
nebu fetch 60200000 60200100 > ledgers.xdr

# Pipe directly to processors
nebu fetch 60200000 60200100 | nebu process token-transfer

# Cache and reuse
nebu fetch 60200000 60200100 --output ledgers.xdr
cat ledgers.xdr | nebu process token-transfer > transfers.jsonl
cat ledgers.xdr | nebu process soroban-events > events.jsonl
```

### Implementation Sketch

1. **New `fetch` subcommand** in `cmd/nebu/fetch.go`:
   - Flags: `--start-ledger`, `--end-ledger`, `--rpc-url`, `--network`, `--output`
   - Outputs: Raw XDR ledger data to stdout (or file with `--output`)
   - Stderr: Progress messages (suppressible with `--quiet`)

2. **Modify `process` subcommand** (rename from `run origin`):
   - Accepts ledgers from stdin OR fetches from RPC (backward compat)
   - If stdin has data, read from there
   - Else fall back to RPC fetch

3. **Keep `run` as convenience wrapper**:
   - `nebu run token-transfer --start X --end Y` still works
   - Internally chains fetch → process

### Scope Line

```
MUST HAVE ══════════════
- nebu fetch command outputs XDR to stdout
- nebu process reads from stdin if available
- Progress messages go to stderr

NICE TO HAVE ───────────
- --output flag to save to file
- Backward compat: nebu run still works

COULD HAVE ─────────────
- Format negotiation (XDR vs JSON)
- Compression support
```

## Rabbit Holes

**Don't** implement format conversion in fetch - just output raw XDR. Let other tools handle format conversion.

**Don't** add filtering/transformation to fetch - it's only for fetching ledgers, nothing more.

**Don't** worry about streaming vs buffering optimization yet - simple pipe semantics first.

## No-Gos

- **No breaking changes to existing CLI** - `nebu run origin processor` must still work
- **No new RPC features** - use existing RPCLedgerSource as-is
- **No caching logic in fetch** - just a thin wrapper around source streaming
- **No output format negotiation** - XDR only for v1

## Done

When complete, this workflow works:

```bash
# Fetch once, process multiple times
$ nebu fetch 60200000 60200100 --output ledgers.xdr
Fetching ledgers 60200000 to 60200100...
Fetched 101 ledgers (15.2 MB)

# Process cached ledgers with different processors
$ cat ledgers.xdr | nebu process token-transfer | wc -l
1636 events

$ cat ledgers.xdr | nebu process soroban-events | wc -l
2451 events

# Old syntax still works
$ nebu run origin token-transfer --start-ledger 60200000 --end-ledger 60200100
(outputs events as before)
```

## gRPC Compatibility

✅ **This does NOT break gRPC processors**

This change separates *ledger fetching* from *ledger processing*. Processors can still be:
- **Local stdin/stdout binaries** (Unix pipes)
- **gRPC services** (network calls)
- **Go library imports** (in-process)

The processor interface remains the same. This just adds a new way to feed ledgers into processors.
