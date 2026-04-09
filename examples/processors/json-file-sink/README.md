# JSON File Sink

A simple sink processor that writes newline-delimited JSON (JSONL) to a file.

## Recommended usage

Install the processors, then run the binaries directly:

```bash
nebu install token-transfer
nebu install json-file-sink

token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  json-file-sink --out events.jsonl
```

## Query the results

```bash
cat events.jsonl | jq 'select(.transfer != null and .transfer.assetCode == "USDC")'

cat events.jsonl | jq -s '
  map(
    if .transfer != null then {event_type: "transfer"}
    elif .mint != null then {event_type: "mint"}
    elif .burn != null then {event_type: "burn"}
    elif .clawback != null then {event_type: "clawback"}
    elif .fee != null then {event_type: "fee"}
    else {event_type: "other"}
    end
  )
  | group_by(.event_type)
  | map({type: .[0].event_type, count: length})
'
```

## Development workflow

If you're working from a clone of this repo, you can build locally:

```bash
go build -o json-file-sink ./cmd/json-file-sink

token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  ./json-file-sink --out events.jsonl
```

## Features

- **Simple**: no external dependencies
- **Fast**: buffered writes for performance
- **Unix-friendly**: reads from stdin, writes to a file
- **Portable**: pure Go

## Example pipeline

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200001 | \
  json-file-sink --out /tmp/events.jsonl

cat /tmp/events.jsonl | jq 'select(.transfer != null) | {
  ledger: .meta.ledgerSequence,
  from: .transfer.from,
  to: .transfer.to,
  amount: .transfer.amount,
  asset: .transfer.assetCode
}'
```
