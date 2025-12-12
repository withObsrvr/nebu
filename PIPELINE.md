# Building Pipelines with nebu

nebu uses Unix pipes to stream events between processors. This guide shows you how to connect origin processors to sink processors.

## Quick Start

### 1. Stream to JSON File (Simplest)

```bash
# Build the JSON file sink
go build -o bin/json-file-sink ./examples/processors/json-file-sink/cmd/

# Stream 100 ledgers into a JSON file
./bin/nebu run origin token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  ./bin/json-file-sink --out events.jsonl

# Query the results
cat events.jsonl | jq 'select(.type == "transfer")'
```

### 2. Stream to DuckDB (Analytics)

```bash
# Build with Nix (handles CGO dependencies)
cd examples/processors/duckdb-sink
nix develop
go build -o duckdb-sink ./cmd

# Stream events into DuckDB
cd ../../..
./bin/nebu run origin token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  ./examples/processors/duckdb-sink/cmd/duckdb-sink --db events.db

# Run SQL analytics
duckdb events.db
```

```sql
-- Count events by type
SELECT event_type, COUNT(*) as count
FROM token_transfer_events
GROUP BY event_type;

-- Find USDC transfers
SELECT *
FROM token_transfer_events
WHERE asset_code = 'USDC' AND event_type = 'transfer'
LIMIT 10;

-- Analyze by hour
SELECT date_trunc('hour', timestamp) as hour, COUNT(*)
FROM token_transfer_events
GROUP BY hour
ORDER BY hour;
```

## How It Works

```
┌─────────────────┐
│  Stellar RPC    │
└────────┬────────┘
         │ XDR ledgers
         ▼
┌─────────────────┐
│ nebu run origin │
│ token-transfer  │
└────────┬────────┘
         │ JSON events (stdout)
         ▼
     Unix Pipe |
         │
         ▼
┌─────────────────┐
│   Sink Process  │
│  (json-file or  │
│   duckdb-sink)  │
└────────┬────────┘
         │
         ▼
   events.jsonl or events.db
```

## Event Format

Events are newline-delimited JSON:

```json
{
  "type": "transfer",
  "ledger_sequence": 60200000,
  "tx_hash": "abc123...",
  "contract_address": "CABC...",
  "from": "GABC...",
  "to": "GDEF...",
  "amount": "100.0",
  "asset": {
    "code": "USDC",
    "issuer": "GA5Z..."
  }
}
```

Event types: `transfer`, `mint`, `burn`, `clawback`, `fee`

## Building Custom Sinks

Create a program that:
1. Reads JSON from stdin
2. Processes each event
3. Writes to your target (DB, API, file, etc.)

Example:

```go
scanner := bufio.NewScanner(os.Stdin)
for scanner.Scan() {
    var event map[string]interface{}
    json.Unmarshal(scanner.Bytes(), &event)

    // Process event
    processEvent(event)
}
```

Then pipe nebu output into your sink:

```bash
nebu run origin token-transfer --start X --end Y | ./my-custom-sink
```

## Examples

### Filter and Transform

```bash
# Only USDC transfers
nebu run origin token-transfer --start 60200000 --end 60200100 | \
  jq 'select(.type == "transfer" and .asset.code == "USDC")' | \
  ./bin/json-file-sink --out usdc-only.jsonl
```

### Real-time Monitoring

```bash
# Watch transfers as they happen
nebu run origin token-transfer --start 60200000 --end 60201000 | \
  jq -c '{type, from, to, amount, asset: .asset.code}'
```

### Multi-Sink Fanout

```bash
# Tee events to multiple sinks
nebu run origin token-transfer --start 60200000 --end 60200100 | \
  tee >(./bin/json-file-sink --out backup.jsonl) | \
  ./examples/processors/duckdb-sink/cmd/duckdb-sink --db analytics.db
```

## Performance Tips

1. **Buffering**: Sinks should buffer writes (both examples do this)
2. **Batch Inserts**: For databases, batch multiple events per INSERT
3. **Parallel Processing**: Run multiple nebu instances for different ledger ranges
4. **Error Handling**: Sinks should continue on invalid events (log warnings)

## Next Steps

- Check `examples/processors/` for more sink examples
- Read `nebu new processor --type sink` to scaffold your own
- See `registry.yaml` to register custom processors
