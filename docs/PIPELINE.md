# Building Pipelines with nebu

nebu uses Unix pipes to stream events between processors. This guide shows you how to connect origin processors to sink processors.

## Quick Start

### 1. Stream to JSON File (Simplest)

```bash
# Build the JSON file sink
go build -o bin/json-file-sink ./examples/processors/json-file-sink/cmd/

# Stream 100 ledgers into a JSON file
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  ./bin/json-file-sink --out events.jsonl

# Query the results
cat events.jsonl | jq 'select(.transfer != null)'
```

### 2. Stream to DuckDB (Analytics)

DuckDB can read JSON directly from stdin, no custom sink needed:

```bash
# Stream events into DuckDB and create a table
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb events.db -c "CREATE TABLE transfers AS SELECT * FROM read_json('/dev/stdin')"

# Run SQL analytics
duckdb events.db -c "
  SELECT
    CASE
      WHEN transfer IS NOT NULL THEN 'transfer'
      WHEN mint IS NOT NULL THEN 'mint'
      WHEN burn IS NOT NULL THEN 'burn'
      WHEN fee IS NOT NULL THEN 'fee'
      ELSE 'other'
    END as event_type,
    COUNT(*) as count
  FROM transfers
  GROUP BY event_type
"

# Find USDC transfers
duckdb events.db -c "
  SELECT
    json_extract(meta, '$.ledgerSequence') as ledger,
    json_extract_string(transfer, '$.from') as from_addr,
    json_extract_string(transfer, '$.to') as to_addr,
    json_extract_string(transfer, '$.amount') as amount
  FROM transfers
  WHERE transfer IS NOT NULL
    AND json_extract_string(transfer, '$.asset.issuedAsset.assetCode') = 'USDC'
  LIMIT 10
"
```

## How It Works

```
┌─────────────────┐
│  Stellar RPC    │
└────────┬────────┘
         │ XDR ledgers
         ▼
┌─────────────────┐
│ token-transfer  │
│  (processor)    │
└────────┬────────┘
         │ JSON events (stdout)
         ▼
     Unix Pipe |
         │
         ▼
┌─────────────────┐
│   Destination   │
│ (json-file-sink,│
│  DuckDB, jq,    │
│  custom tools)  │
└────────┬────────┘
         │
         ▼
  events.jsonl / events.db / etc.
```

## Event Format

Events are newline-delimited JSON with protobuf structure:

```json
{
  "_schema": "nebu.token-transfer.v1",
  "_nebu_version": "0.3.0",
  "meta": {
    "ledgerSequence": 60200000,
    "closedAt": "2025-12-08T01:45:11Z",
    "txHash": "abc123...",
    "transactionIndex": 1,
    "contractAddress": "CABC..."
  },
  "transfer": {
    "from": "GABC...",
    "to": "GDEF...",
    "asset": {
      "issuedAsset": {
        "assetCode": "USDC",
        "issuer": "GA5Z..."
      }
    },
    "amount": "100"
  }
}
```

Event types (as top-level fields): `transfer`, `mint`, `burn`, `clawback`, `fee`

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

Then pipe processor output into your sink:

```bash
token-transfer --start-ledger X --end-ledger Y | ./my-custom-sink
```

## Examples

### Filter and Transform

```bash
# Only USDC transfers
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.transfer != null and .transfer.asset.issuedAsset.assetCode == "USDC")' | \
  ./bin/json-file-sink --out usdc-only.jsonl
```

### Real-time Monitoring

```bash
# Watch transfers as they happen
token-transfer --start-ledger 60200000 --end-ledger 60201000 | \
  jq -c 'select(.transfer != null) | {from: .transfer.from, to: .transfer.to, amount: .transfer.amount, asset: .transfer.asset.issuedAsset.assetCode}'
```

### Multi-Sink Fanout

```bash
# Tee events to multiple destinations
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  tee >(./bin/json-file-sink --out backup.jsonl) | \
  duckdb analytics.db -c "CREATE TABLE transfers AS SELECT * FROM read_json('/dev/stdin')"
```

## Performance Tips

1. **Buffering**: Custom sinks should buffer writes for better performance
2. **Batch Inserts**: For databases, batch multiple events per INSERT
3. **Parallel Processing**: Run multiple processors for different ledger ranges and combine results
4. **Error Handling**: Sinks should continue on invalid events (log warnings)
5. **Use DuckDB**: For analytics, pipe directly to DuckDB instead of building custom sinks

## Next Steps

- Check `examples/processors/` for more processor examples
- See [BUILDING_PROCESSORS.md](./BUILDING_PROCESSORS.md) to build your own processors
- Check the [DuckDB Cookbook](./DUCKDB_COOKBOOK.md) for analytics examples and reusable queries
- See `registry.yaml` to understand how processors are discovered
