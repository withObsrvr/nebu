# Building Pipelines with nebu

nebu processors compose through Unix pipes. A typical pipeline looks like:

```text
nebu fetch | origin | transform | sink
```

Or, more commonly for installed processors:

```text
token-transfer | usdc-filter | amount-filter | json-file-sink
```

## Recommended workflow

For end users, the canonical flow is:

```bash
go install github.com/withObsrvr/nebu/cmd/nebu@latest
nebu install token-transfer
nebu install json-file-sink
```

Then run the installed binaries directly:

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  json-file-sink --out events.jsonl
```

Use repo-local `go build` examples only when developing a processor.

## Quick start

### 1. Stream to a JSONL file

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  json-file-sink --out events.jsonl

cat events.jsonl | jq 'select(.transfer != null)'
```

### 2. Stream to DuckDB

DuckDB can read JSON directly from stdin, so you often do not need a custom sink.

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb events.db -c "CREATE TABLE transfers AS SELECT * FROM read_json('/dev/stdin')"
```

Query the table:

```bash
duckdb events.db -c "
  SELECT
    json_extract_string(transfer, '$.assetCode') AS asset,
    COUNT(*) AS count,
    SUM(CAST(json_extract_string(transfer, '$.amount') AS DOUBLE)) AS volume
  FROM transfers
  WHERE transfer IS NOT NULL
  GROUP BY asset
  ORDER BY volume DESC
"
```

## How it works

```text
┌─────────────────┐
│  Stellar RPC    │
└────────┬────────┘
         │ XDR ledgers
         ▼
┌─────────────────┐
│ token-transfer  │  origin processor
└────────┬────────┘
         │ JSON events on stdout
         ▼
     Unix pipe
         │
         ▼
┌─────────────────┐
│   Destination   │
│ jq / DuckDB /   │
│ sink processor  │
└─────────────────┘
```

## Event format

Token-transfer events are newline-delimited JSON. A transfer event looks like:

```json
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "v0.6.11",
  "meta": {
    "ledgerSequence": 60200000,
    "closedAtUnix": "1765158311",
    "txHash": "abc123...",
    "transactionIndex": 1,
    "contractAddress": "CABC..."
  },
  "transfer": {
    "from": "GABC...",
    "to": "GDEF...",
    "assetCode": "USDC",
    "assetIssuer": "GA5Z...",
    "amount": "100"
  }
}
```

Top-level event fields include `transfer`, `mint`, `burn`, `clawback`, and `fee`.

## Common pipeline patterns

### Filter with jq

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.transfer != null and .transfer.assetCode == "USDC")'
```

### Chain nebu transforms

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  usdc-filter | \
  amount-filter --min 1000000 | \
  json-file-sink --out usdc-large.jsonl
```

### Reuse fetched ledger data

```bash
nebu fetch 60200000 60200100 > ledgers.xdr
cat ledgers.xdr | token-transfer | jq 'select(.transfer != null)'
```

### Multi-sink fanout

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  tee >(json-file-sink --out backup.jsonl) | \
  tee >(nats-sink --subject stellar.transfers) | \
  jq -c 'select(.transfer != null)'
```

### Real-time monitoring

```bash
token-transfer --start-ledger 60200000 --follow | \
  jq -c 'select(.transfer != null) | {
    ledger: .meta.ledgerSequence,
    from: .transfer.from,
    to: .transfer.to,
    amount: .transfer.amount,
    asset: .transfer.assetCode
  }'
```

## Building a custom sink

A custom sink only needs to:

1. read JSON lines from stdin
2. process each event
3. write to its target system

Minimal pattern:

```go
scanner := bufio.NewScanner(os.Stdin)
for scanner.Scan() {
    var event map[string]interface{}
    json.Unmarshal(scanner.Bytes(), &event)
    processEvent(event)
}
```

Then use it in a pipeline:

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | ./my-custom-sink
```

## Performance tips

1. buffer writes in sinks
2. batch DB inserts or HTTP sends
3. separate fetch from processing when reusing the same ledger range
4. keep logs on stderr so stdout remains clean JSON/XDR
5. use DuckDB for ad-hoc analytics before writing a custom processor

## See also

- [`README.md`](../README.md)
- [`BUILDING_PROCESSORS.md`](./BUILDING_PROCESSORS.md)
- [`DUCKDB_COOKBOOK.md`](./DUCKDB_COOKBOOK.md)
- [`REGISTRY_SPEC.md`](./REGISTRY_SPEC.md)
