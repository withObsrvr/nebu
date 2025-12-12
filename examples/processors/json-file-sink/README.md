# JSON File Sink

A simple sink processor that writes events to a newline-delimited JSON (JSONL) file.

## Usage

```bash
# Build the sink
go build -o json-file-sink ./cmd

# Stream events into file
nebu run origin token-transfer --start 60200000 --end 60200100 | \
  ./json-file-sink --out events.jsonl

# Query the events
cat events.jsonl | jq 'select(.asset.code == "USDC")'
cat events.jsonl | jq -s 'group_by(.type) | map({type: .[0].type, count: length})'
```

## Features

- **Simple**: No external dependencies
- **Fast**: Buffered writes for performance
- **Unix-friendly**: Reads from stdin, writes to file
- **Portable**: Pure Go, works everywhere

## Example Pipeline

```bash
# Count event types
nebu run origin token-transfer --start 60200000 --end 60200001 | \
  ./json-file-sink --out /tmp/events.jsonl

cat /tmp/events.jsonl | jq -s 'group_by(.type) | map({type: .[0].type, count: length})'
# Output:
# [
#   {"type": "fee", "count": 1500},
#   {"type": "transfer", "count": 126}
# ]
```
