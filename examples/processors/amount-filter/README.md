# amount-filter Transform Processor

Filter token transfer events based on amount ranges and asset codes.

## Features

- Filter by minimum amount
- Filter by maximum amount
- Filter by asset code
- **Dual-mode**: Works as both CLI tool and gRPC service
- Shared core logic between CLI and gRPC modes

## Quick Start

### CLI Mode (Local)

```bash
# Build CLI
go build -o amount-filter ./cmd

# Filter for amounts >= 10M stroops
cat events.jsonl | amount-filter --min 10000000

# Filter for USDC transfers >= 10M
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  amount-filter --min 10000000 --asset USDC

# Filter for amounts between 1M and 100M
cat events.jsonl | amount-filter --min 1000000 --max 100000000
```

### gRPC Mode (Remote)

**Start server:**
```bash
# Build gRPC server
go build -o amount-filter-grpc-server ./cmd/grpc-server

# Start on default port (9001)
./amount-filter-grpc-server

# Start on custom port
./amount-filter-grpc-server --port 9002

# Start with environment configuration
MIN_AMOUNT=10000000 ASSET_CODE=USDC ./amount-filter-grpc-server
```

**Use in pipeline:**
```bash
# Configure via nebu CLI (future)
nebu run \
  --origin token-transfer --start-ledger 60200000 --end-ledger 60200100 \
  --transform grpc://localhost:9001?min=10000000&asset=USDC \
  --sink json-file-sink --out results.jsonl
```

**Test with grpcurl:**
```bash
# List services
grpcurl -plaintext localhost:9001 list

# Get current configuration
grpcurl -plaintext localhost:9001 \
  nebu.amount_filter.AmountFilterService/GetConfig

# Configure filter
grpcurl -plaintext -d '{"config":{"min_amount":10000000,"asset_code":"USDC"}}' \
  localhost:9001 nebu.amount_filter.AmountFilterService/Configure

# Transform single event
grpcurl -plaintext -d '{"event_json":"eyJhbW91bnQiOiIxMDAwMDAwMCJ9"}' \
  localhost:9001 nebu.amount_filter.AmountFilterService/Transform
```

## Architecture

```
examples/processors/amount-filter/
├── filter.go                  # Core filtering logic (shared)
├── cmd/
│   ├── main.go                # CLI binary
│   └── grpc-server/
│       └── main.go            # gRPC server binary
├── server/
│   └── server.go              # gRPC service implementation
├── proto/
│   ├── amount_filter.proto    # Protobuf definition
│   ├── amount_filter.pb.go    # Generated protobuf code
│   └── amount_filter_grpc.pb.go
└── README.md
```

**Key principle**: The same `filter.FilterEvent()` logic powers both modes.

## CLI Usage

### Flags

- `--min <amount>` - Minimum amount in stroops (inclusive, 0 = no minimum)
- `--max <amount>` - Maximum amount in stroops (inclusive, 0 = no maximum)
- `--asset <code>` - Asset code to filter (e.g., "USDC", "XLM", empty = any asset)
- `-q, --quiet` - Suppress progress messages

### Examples

**Filter by minimum amount:**
```bash
$ cat events.jsonl | amount-filter --min 10000000 | wc -l
42
```

**Filter by amount range:**
```bash
$ cat events.jsonl | amount-filter --min 1000000 --max 50000000 > medium-transfers.jsonl
```

**Filter by asset:**
```bash
$ token-transfer | amount-filter --asset USDC > usdc-only.jsonl
```

**Combine filters:**
```bash
$ token-transfer | amount-filter --min 100000000 --asset USDC > usdc-whales.jsonl
```

**Chain with other processors:**
```bash
$ nebu fetch 60200000 60200100 | \
  token-transfer | \
  amount-filter --min 10000000 | \
  dedup --key tx_hash | \
  json-file-sink --out large-transfers.jsonl
```

## gRPC Usage

### Protocol

The gRPC service exposes four RPCs:

1. **Configure** - Set filter parameters
2. **GetConfig** - Get current filter configuration
3. **Transform** - Filter a single event
4. **TransformStream** - Filter a stream of events (bidirectional)

### Environment Configuration

Configure the filter via environment variables:

- `MIN_AMOUNT` - Minimum amount (stroops)
- `MAX_AMOUNT` - Maximum amount (stroops)
- `ASSET_CODE` - Asset code to filter

```bash
MIN_AMOUNT=10000000 MAX_AMOUNT=1000000000 ASSET_CODE=USDC \
  amount-filter-grpc-server
```

### Docker Deployment

**Dockerfile:**
```dockerfile
FROM golang:1.25 AS builder
WORKDIR /app
COPY . .
RUN go build -o /bin/amount-filter-grpc-server \
    ./examples/processors/amount-filter/cmd/grpc-server

FROM alpine:latest
COPY --from=builder /bin/amount-filter-grpc-server /usr/local/bin/
EXPOSE 9001
ENTRYPOINT ["amount-filter-grpc-server"]
CMD ["--port", "9001"]
```

**Docker Compose:**
```yaml
version: '3.8'

services:
  amount-filter:
    build: .
    ports:
      - "9001:9001"
    environment:
      - MIN_AMOUNT=10000000
      - ASSET_CODE=USDC
```

### Load Balancing

Run multiple instances for horizontal scaling:

```bash
# Start 3 instances
docker-compose up --scale amount-filter=3

# nebu CLI will load balance across them
nebu run \
  --transform grpc://amount-filter-1:9001,grpc://amount-filter-2:9001,grpc://amount-filter-3:9001
```

## Development

### Building

**CLI:**
```bash
go build -o amount-filter ./cmd
```

**gRPC Server:**
```bash
go build -o amount-filter-grpc-server ./cmd/grpc-server
```

**Both with Make:**
```bash
make build-processors  # Builds all processors including amount-filter CLI
```

### Generating Protobuf Code

If you modify `proto/amount_filter.proto`:

```bash
# Using nix flake (includes protoc)
nix develop --command make gen-protos

# Or manually
protoc \
  --go_out=. \
  --go_opt=paths=source_relative \
  --go-grpc_out=. \
  --go-grpc_opt=paths=source_relative \
  proto/amount_filter.proto
```

### Testing

**CLI tests:**
```bash
# Test basic filtering
echo '{"amount":"50000000"}
{"amount":"5000000"}
{"amount":"75000000"}' | \
  amount-filter --min 10000000

# Expected: 2 events (5M filtered out)

# Test asset filtering
echo '{"amount":"50000000","asset":{"code":"USDC"}}
{"amount":"50000000","asset":{"code":"XLM"}}' | \
  amount-filter --asset USDC

# Expected: 1 event (XLM filtered out)
```

**gRPC tests:**
```bash
# Start server
amount-filter-grpc-server &
SERVER_PID=$!

# Test configuration
grpcurl -plaintext -d '{"config":{"min_amount":10000000}}' \
  localhost:9001 nebu.amount_filter.AmountFilterService/Configure

# Test filtering
# (encode JSON event as base64 for event_json field)

# Cleanup
kill $SERVER_PID
```

## When to Use Each Mode

### Use CLI Mode When:
- ✅ Developing and testing locally
- ✅ Running on a single machine
- ✅ Processing ad-hoc data files
- ✅ Chaining with other Unix tools (jq, grep, etc.)
- ✅ No network overhead is important

### Use gRPC Mode When:
- ✅ Deploying in a distributed system
- ✅ Need horizontal scaling (multiple instances)
- ✅ Sharing infrastructure across teams
- ✅ Mixing languages (Python client, Go service, etc.)
- ✅ Need load balancing and failover

### Hybrid Approach:
```bash
# Origin is remote (shared RPC infrastructure)
# Transform is local (fast, no network)
# Sink is remote (shared database)

grpc://token-transfer:9000 | \
  amount-filter --min 10000000 | \
  grpc://postgres-sink:9002
```

## Performance

**CLI Mode:**
- ~100k events/sec single-threaded
- Zero network overhead
- Direct stdin/stdout pipes

**gRPC Mode:**
- ~50k events/sec single connection
- Network latency overhead (~1-2ms)
- Supports streaming for reduced overhead
- Horizontal scaling: N instances = N × throughput

## Troubleshooting

**CLI Issues:**

*Binary not found:*
```bash
# Add to PATH or use full path
export PATH="$PATH:./bin"
# or
./bin/amount-filter
```

*No output:*
```bash
# Check if all events are being filtered
# Try without filters first
cat events.jsonl | amount-filter
```

**gRPC Issues:**

*Connection refused:*
```bash
# Verify server is running
ps aux | grep amount-filter-grpc-server

# Check port is correct
netstat -tulpn | grep 9001
```

*Events not filtered:*
```bash
# Check configuration
grpcurl -plaintext localhost:9001 \
  nebu.amount_filter.AmountFilterService/GetConfig

# Reconfigure if needed
grpcurl -plaintext -d '{"config":{"min_amount":10000000}}' \
  localhost:9001 nebu.amount_filter.AmountFilterService/Configure
```

## See Also

- [Building Custom Processors](../../../docs/BUILDING_PROCESSORS.md) - Guide for creating processors
- [gRPC Processors Architecture](../../../docs/GRPC_PROCESSORS.md) - Detailed gRPC integration guide
- [USDC Filter](../usdc-filter/) - Simpler single-asset filter example
- [Time Window](../time-window/) - Time-based filtering example

## License

Apache 2.0
