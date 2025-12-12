# Shape: Support `-` as Explicit stdin Marker

## Problem
While processors can auto-detect stdin (from Shape 02), there's no explicit way to force stdin mode. Unix tools conventionally use `-` to mean "read from stdin", which:
- Makes intent explicit in scripts
- Allows mixing stdin with file arguments
- Follows established conventions (tar, cat, gzip, etc.)

Currently you can't write:
```bash
nebu process token-transfer - < fixtures.xdr
```

## Appetite
**1 day** - Very small, focused addition to stdin support

## Solution

Accept `-` as a positional argument to explicitly read from stdin:

```bash
# Explicit stdin marker
nebu process token-transfer - < ledgers.xdr

# Works in pipelines too
cat ledgers.xdr | nebu process token-transfer -

# Mix with flags
nebu process token-transfer - --quiet < test-fixtures.xdr
```

### Implementation Sketch

1. **Add positional argument** to process command:
   ```go
   var processCmd = &cobra.Command{
       Use:   "process <processor-name> [input-file]",
       Args:  cobra.RangeArgs(1, 2),
       RunE: func(cmd *cobra.Command, args []string) error {
           processorName := args[0]

           var inputSource io.Reader
           if len(args) == 2 && args[1] == "-" {
               // Explicit stdin
               inputSource = os.Stdin
           } else if len(args) == 2 {
               // File input
               f, _ := os.Open(args[1])
               defer f.Close()
               inputSource = f
           } else {
               // Auto-detect stdin or use RPC flags
               inputSource = detectInput()
           }

           return runProcessor(processorName, inputSource)
       },
   }
   ```

2. **Update help text**:
   ```
   Usage:
     nebu process <processor-name> [input-file]

   Arguments:
     processor-name    Name of processor to run
     input-file        XDR ledger file to process, or - for stdin (optional)

   If input-file is omitted, reads from stdin if available, otherwise uses RPC.
   ```

### Scope Line

```
MUST HAVE ══════════════
- Accept - as positional argument
- Read from stdin when - is specified
- Update help text with examples

NICE TO HAVE ───────────
- Support file path argument (not just -)
- Mix stdin and file arguments in future

COULD HAVE ─────────────
- Multiple input files
- Concatenate multiple sources
```

## Rabbit Holes

**Don't** implement multiple file support - just stdin (-) or single file.

**Don't** add smart input detection - explicit is better than implicit.

**Don't** make - work for output (stdout) - that's already the default.

## No-Gos

- **No breaking changes** - auto-detection still works without -
- **No new input formats** - still just XDR
- **No input concatenation** - single source only
- **No output redirection with -** - stdout is already default

## Done

When complete, these all work:

```bash
# Explicit stdin from pipe
$ cat ledgers.xdr | nebu process token-transfer -
{"type":"fee",...}

# Explicit stdin from redirect
$ nebu process token-transfer - < ledgers.xdr
{"type":"fee",...}

# Auto-detection still works (backward compat)
$ cat ledgers.xdr | nebu process token-transfer
{"type":"fee",...}

# File input (future enhancement)
$ nebu process token-transfer ledgers.xdr
{"type":"fee",...}

# Error handling
$ nebu process token-transfer /nonexistent.xdr
Error: failed to open input file: no such file or directory

# Help shows usage
$ nebu process --help
Usage:
  nebu process <processor-name> [input-file]

Arguments:
  processor-name    Processor to run (e.g., token-transfer)
  input-file        XDR ledger file, or - for stdin (default: auto-detect)

Examples:
  # Process from stdin
  cat ledgers.xdr | nebu process token-transfer -

  # Process from file
  nebu process token-transfer ledgers.xdr

  # Fetch from RPC
  nebu process token-transfer --start-ledger 60200000 --end-ledger 60200100
```

## gRPC Compatibility

✅ **This does NOT affect gRPC processors**

This is about input source selection for local CLI mode. gRPC processors receive data over network, not from stdin/files.

However, this enables testing gRPC processors locally:
```bash
# Save RPC data to file
$ nebu fetch 60200000 60200100 > test-ledgers.xdr

# Test local processor
$ nebu process token-transfer - < test-ledgers.xdr > expected.jsonl

# Test gRPC processor gives same results
$ grpc-client call token-transfer:9000 ProcessLedgers - < test-ledgers.xdr > actual.jsonl

$ diff expected.jsonl actual.jsonl
(no diff = gRPC implementation matches local)
```
