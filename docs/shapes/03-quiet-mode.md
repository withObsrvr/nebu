# Shape: Add Quiet Mode (Rule of Silence)

## Problem
nebu currently outputs progress messages to stderr:
```
Processing ledgers 60200000 to 60200100...
```

This violates Unix "Rule of Silence" - tools should be silent on success. Progress messages pollute stderr in scripts and pipelines, making it hard to distinguish real errors from status updates.

## Appetite
**1 day** - Smallest, most focused improvement

## Solution

Add `--quiet` (short: `-q`) flag to suppress non-error output on stderr:

```bash
# Default: shows progress
$ nebu fetch 60200000 60200100 2>&1 | head -2
Processing ledgers 60200000 to 60200100...
<XDR data...>

# Quiet: silent on success
$ nebu fetch --quiet 60200000 60200100 2>&1 | head -2
<XDR data...>

# Still shows errors
$ nebu fetch --quiet 99999999 99999999 2>&1
Error: ledger 99999999 not found
```

### Implementation Sketch

1. **Add global flag** to root command:
   ```go
   var quietMode bool
   rootCmd.PersistentFlags().BoolVarP(&quietMode, "quiet", "q", false, "suppress non-error output")
   ```

2. **Create logging helper**:
   ```go
   func logInfo(format string, args ...interface{}) {
       if !quietMode {
           fmt.Fprintf(os.Stderr, format+"\n", args...)
       }
   }

   func logError(format string, args ...interface{}) {
       fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
   }
   ```

3. **Replace direct stderr writes**:
   - Find all `fmt.Fprintf(os.Stderr, ...)` for progress/info
   - Replace with `logInfo(...)`
   - Keep errors as `logError(...)` (always shown)

### Scope Line

```
MUST HAVE ══════════════
- --quiet/-q flag suppresses progress messages
- Errors still shown on stderr
- Works on all subcommands (fetch, process, run)

NICE TO HAVE ───────────
- Respect NO_COLOR environment variable
- --verbose flag for extra debug output

COULD HAVE ─────────────
- Multiple verbosity levels (-q, -qq, -qqq)
- Structured logging (JSON output)
```

## Rabbit Holes

**Don't** implement verbose mode now - just quiet mode.

**Don't** add structured logging or log levels - simple quiet vs normal.

**Don't** make quiet mode suppress warnings - only suppress progress/info.

## No-Gos

- **No logging framework dependencies** - use simple fmt.Fprintf
- **No structured logging (JSON)** - just silence progress messages
- **No log levels beyond quiet** - keep it simple
- **Errors must always show** - never suppress actual errors

## Done

When complete:

```bash
# Default: chatty
$ nebu fetch 60200000 60200100 | nebu process token-transfer
Processing ledgers 60200000 to 60200100...
Processing 101 ledgers through token-transfer...
{"type":"fee",...}
{"type":"transfer",...}

# Quiet: silent on success
$ nebu fetch --quiet 60200000 60200100 | nebu process --quiet token-transfer
{"type":"fee",...}
{"type":"transfer",...}

# Errors always show
$ nebu fetch --quiet 99999999 99999999
Error: failed to fetch ledger 99999999: not found

# Works in pipelines
$ nebu fetch -q 60200000 60200100 | nebu process -q token-transfer | duckdb
(no stderr pollution, clean DuckDB output)
```

## gRPC Compatibility

✅ **This does NOT affect gRPC processors**

This is purely about CLI output behavior. gRPC processors communicate over network - this flag only controls local stderr output from the nebu CLI.
