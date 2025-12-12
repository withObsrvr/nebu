# Shape: Support stdin Input for Processors

## Problem
Processors can only receive ledgers from live RPC fetches. They can't process:
- Pre-fetched ledger files
- Test fixtures
- Historical archives
- Ledgers from alternative sources (S3, local archives, other networks)

This breaks the Unix philosophy of accepting input from pipes.

## Appetite
**2 days** - Clear scope, mostly plumbing work

## Solution

Make processors accept XDR ledger data from stdin in addition to RPC:

```bash
# Read from stdin
cat ledgers.xdr | nebu process token-transfer

# Explicit stdin marker (Unix convention)
nebu process token-transfer - < ledgers.xdr

# Still support RPC (backward compat)
nebu process token-transfer --start-ledger X --end-ledger Y
```

### Implementation Sketch

1. **Update processor runner** to detect stdin:
   ```go
   func runProcessor(cmd *cobra.Command, proc Processor) error {
       stat, _ := os.Stdin.Stat()
       if (stat.Mode() & os.ModeCharDevice) == 0 {
           // stdin is a pipe, read from it
           return runFromStdin(proc)
       } else {
           // no stdin, use RPC flags
           return runFromRPC(proc, startLedger, endLedger)
       }
   }
   ```

2. **Add XDR decoder for stdin**:
   - Read XDR-encoded ledgers from stdin
   - Feed to processor's ProcessLedger() method
   - Handle errors gracefully

3. **Support `-` as explicit stdin marker**:
   - `nebu process token-transfer -` forces stdin mode
   - Matches standard Unix conventions (`tar`, `gzip`, etc.)

### Scope Line

```
MUST HAVE ══════════════
- Detect stdin pipe automatically
- Read XDR ledgers from stdin
- Process them through existing processor interface
- Error handling for malformed input

NICE TO HAVE ───────────
- Support - as explicit stdin marker
- Auto-detect input format (XDR vs JSON)

COULD HAVE ─────────────
- Resume from specific ledger in stream
- Parallel processing of stdin batches
```

## Rabbit Holes

**Don't** implement format auto-detection - assume XDR for v1. Add JSON support later if needed.

**Don't** add buffering/batching optimization - simple streaming first.

**Don't** implement checkpointing/resume logic - out of scope for v1.

## No-Gos

- **No breaking changes** - RPC mode must still work as default
- **No new dependencies** - use existing XDR decoder from stellar SDK
- **No stdin buffering magic** - simple streaming only
- **No format conversion** - accept XDR, output JSON (existing behavior)

## Done

When complete, all these workflows work:

```bash
# Pipe from fetch
$ nebu fetch 60200000 60200100 | nebu process token-transfer | head -5
{"type":"fee","ledger_sequence":60200000,...}
{"type":"fee","ledger_sequence":60200000,...}
...

# Process saved files
$ cat cached-ledgers.xdr | nebu process token-transfer > events.jsonl

# Explicit stdin marker
$ nebu process token-transfer - < test-fixtures.xdr

# Old RPC mode still works
$ nebu process token-transfer --start-ledger X --end-ledger Y
```

Error handling:
```bash
$ echo "invalid data" | nebu process token-transfer
Error: failed to decode XDR ledger at byte 0: invalid XDR header
$ echo $?
1
```

## gRPC Compatibility

✅ **This does NOT break gRPC processors**

This is about *input sources*, not processor implementation. Processors can still be:
- Local Go code (current)
- gRPC services (future)
- External binaries (future)

The processor interface doesn't change - it still receives `xdr.LedgerCloseMeta` objects. This just adds a new way to source those objects (stdin vs RPC).
