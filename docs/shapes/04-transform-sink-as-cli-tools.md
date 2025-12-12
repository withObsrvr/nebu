# Shape: Make Transform/Sink Processors Standalone CLI Tools

## Problem
Currently only Origin processors can be run via CLI. Transform and Sink processors are Go interfaces with no CLI exposure. This breaks Unix composability - you can't chain processors together:

```bash
# Can't do this (desired):
nebu fetch X Y | token-transfer | usdc-filter | aggregate-hourly | postgres-sink

# Only this works (coupled):
nebu run origin token-transfer --start X --end Y
```

Transform and Sink processors are locked into Go codebases instead of being composable Unix filters.

## Appetite
**5 days** - This is a bigger architectural shift with examples needed

## Solution

Make Transform and Sink processors standalone binaries that read JSON from stdin and write JSON to stdout:

```bash
# Origin processor: XDR → JSON events
nebu fetch X Y | token-transfer > events.jsonl

# Transform processor: JSON events → filtered JSON events
cat events.jsonl | usdc-filter > usdc-events.jsonl

# Sink processor: JSON events → side effects (database, files, etc)
cat usdc-events.jsonl | postgres-sink --db-url postgresql://...

# Chain them all
nebu fetch X Y | token-transfer | usdc-filter | aggregate-hourly | postgres-sink
```

### Implementation Sketch

1. **Define standard JSON event format**:
   ```json
   {"type":"transfer","ledger_sequence":60200000,"data":{...}}
   ```

2. **Create CLI wrapper for Transform processors**:
   ```go
   // examples/processors/usdc-filter/cmd/main.go
   func main() {
       scanner := bufio.NewScanner(os.Stdin)
       for scanner.Scan() {
           var event Event
           json.Unmarshal(scanner.Bytes(), &event)

           // Transform logic
           if transformed := processor.Transform(event); transformed != nil {
               json.NewEncoder(os.Stdout).Encode(transformed)
           }
       }
   }
   ```

3. **Create example processors**:
   - `usdc-filter` - Transform: filter for USDC transfers
   - `aggregate-hourly` - Transform: aggregate events by hour
   - `postgres-sink` - Sink: write to PostgreSQL

4. **Update registry** to support transform/sink types with CLI binaries

### Scope Line

```
MUST HAVE ══════════════
- Standard JSON event format (schema defined)
- CLI wrapper pattern for Transform processors
- CLI wrapper pattern for Sink processors
- One example of each: transform + sink
- Updated registry.yaml format

NICE TO HAVE ───────────
- Generator: nebu new transform creates CLI wrapper
- Multiple transform examples
- Multiple sink examples (postgres, kafka, S3)

COULD HAVE ─────────────
- Binary event format (protobuf) for performance
- Parallel processing (sharding)
- Stateful transforms (aggregations)
```

## Rabbit Holes

**Don't** optimize event format (JSON vs protobuf) - JSON first, optimize later.

**Don't** implement complex transforms (stateful aggregations, joins) - simple filters only.

**Don't** build a framework - just document the pattern with examples.

## No-Gos

- **No breaking changes to Go interfaces** - Transform/Sink interfaces stay the same
- **No new protocols** - just JSON over stdin/stdout
- **No state management** - transforms are stateless filters for v1
- **No orchestration** - each tool runs independently, shell manages pipeline

## Done

When complete, this pipeline works end-to-end:

```bash
# Build example processors
$ make build-examples
Building token-transfer...
Building usdc-filter...
Building aggregate-hourly...
Building postgres-sink...

# Run composable pipeline
$ nebu fetch 60200000 60200100 | \
  ./bin/token-transfer | \
  ./bin/usdc-filter | \
  tee filtered-events.jsonl | \
  ./bin/postgres-sink --db-url postgresql://localhost/stellar

Processed 1636 events
Filtered to 234 USDC transfers
Inserted 234 rows into transfers table

# Each processor works standalone
$ echo '{"type":"transfer","asset":{"code":"USDC"},...}' | ./bin/usdc-filter
{"type":"transfer","asset":{"code":"USDC"},...}

$ echo '{"type":"transfer","asset":{"code":"XLM"},...}' | ./bin/usdc-filter
(no output - filtered out)
```

Documentation shows the pattern:
```bash
$ cat examples/processors/usdc-filter/README.md
# USDC Filter Transform Processor

Filters token transfer events for USDC only.

## Usage
cat events.jsonl | usdc-filter > usdc-events.jsonl

## Input Format
Newline-delimited JSON events from token-transfer processor...

## Output Format
Same as input, filtered for USDC transfers only...
```

## gRPC Compatibility

✅ **This COMPLEMENTS gRPC processors**

This creates *local CLI binaries* for processors. You can still have:

- **Local mode**: stdin/stdout binaries (this shape)
  ```bash
  cat events.jsonl | usdc-filter | postgres-sink
  ```

- **Remote mode**: gRPC processors over network
  ```bash
  nebu run origin token-transfer --transform grpc://usdc-filter:9000 --sink grpc://postgres:9001
  ```

- **Hybrid**: Mix local and remote
  ```bash
  nebu fetch X Y | token-transfer | grpc-call usdc-filter:9000 | postgres-sink
  ```

The processor interfaces (Origin, Transform, Sink) remain the same. This just adds CLI wrappers for local execution.

Future work could create a `nebu-grpc` wrapper that exposes any CLI processor as a gRPC service.
