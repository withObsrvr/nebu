# nats-sink

**Type:** Sink Processor
**Version:** 0.1.0

Publish JSON events from nebu pipelines to NATS message bus for real-time distribution to microservices, websocket servers, and analytics workers.

## Overview

`nats-sink` bridges the Unix pipe world (stdout) to the distributed messaging world (NATS), enabling event-driven architectures and real-time data distribution.

```
token-transfer | nats-sink --subject "stellar.{transfer.assetCode}"
```

**Key Features:**
- 🚀 **Simple** - One command to publish to NATS
- 🎯 **Dynamic Routing** - Template-based subject resolution
- 💪 **Reliable** - JetStream support for durable delivery
- 🔧 **Flexible** - Static or dynamic subjects, nested field access

## Installation

```bash
# Build from source
cd examples/processors/nats-sink
go build -o nats-sink ./cmd/nats-sink

# Or use Makefile
make build-processors
```

## Quick Start

### 1. Start NATS Locally

```bash
# Using Docker
docker run -p 4222:4222 nats:latest

# Or install NATS server
# https://docs.nats.io/running-a-nats-service/introduction/installation
```

### 2. Publish Events

```bash
# Static subject - all events to one topic
token-transfer --start-ledger 60200000 --end-ledger 60200010 | \
  nats-sink --url nats://localhost:4222 --subject "stellar.events"

# Dynamic subject - route by asset code
token-transfer --start-ledger 60200000 --end-ledger 60200010 | \
  nats-sink --url nats://localhost:4222 --subject "stellar.{transfer.assetCode}"
```

### 3. Subscribe to Events

```bash
# Install NATS CLI
# https://github.com/nats-io/natscli

# Subscribe to all events
nats sub "stellar.>"

# Subscribe to specific asset streams
nats sub "stellar.USDC"
nats sub "stellar.XLM"
```

## Usage

### Basic Syntax

```bash
nats-sink [flags]

Flags:
  --url string         NATS server URL (default "nats://localhost:4222")
  --subject string     Subject template (default "events")
  --jetstream          Use JetStream for reliable delivery
  --creds string       Path to NATS credentials file (optional)
  --name string        Connection name for monitoring (default "nats-sink")
  --timeout int        Connection timeout in seconds (default 5)
  --strict             Fail on missing template variables
  -q, --quiet          Suppress info logs
```

### Environment Variables

```bash
export NATS_URL="nats://my-server:4222"
export NATS_CREDS="/path/to/creds.file"

# Then use without flags
token-transfer | nats-sink --subject "events"
```

## Subject Templates

Subject templates allow dynamic routing based on event content.

### Static Subjects

```bash
# All events to one subject
nats-sink --subject "stellar.events"
```

### Dynamic Subjects with Top-Level Fields

```bash
# Route contract-events by top-level eventType
nats-sink --subject "stellar.{eventType}"
# → fee events → "stellar.fee"
# → swap events → "stellar.swap"
```

### Dynamic Subjects with Nested Fields

```bash
# Route token-transfer events by asset code
nats-sink --subject "stellar.transfers.{transfer.assetCode}"
# → USDC transfer-like events → "stellar.transfers.USDC"
# → XLM fee events → "stellar.transfers.XLM"

# Route by nested field
nats-sink --subject "stellar.{transfer.assetCode}"
# Access nested fields with dot notation
```

### Missing Field Behavior

```bash
# Default: Use "_unknown" for missing fields
nats-sink --subject "stellar.{missingField}"
# → "stellar._unknown"

# Strict: Fail pipeline on missing field
nats-sink --subject "stellar.{missingField}" --strict
# → Error and exit
```

## Examples

### Example 1: Simple Event Publishing

```bash
# Fetch ledgers and publish all events
token-transfer --start-ledger 60200000 --end-ledger 60200010 | \
  nats-sink --url nats://localhost:4222 --subject "stellar.events"
```

### Example 2: Filter and Route by Type

```bash
# Route contract-events by eventType
contract-events --start-ledger 60200000 --end-ledger 60200100 | \
  nats-sink --subject "stellar.{eventType}"

# Subscribers can listen to specific types
# Terminal 1: nats sub "stellar.fee"
# Terminal 2: nats sub "stellar.swap"
# Terminal 3: nats sub "stellar.transfer"
```

### Example 3: Multi-Level Routing

```bash
# Route token-transfer events by asset
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  nats-sink --subject "stellar.transfers.{transfer.assetCode}"

# Wildcard subscriptions
nats sub "stellar.transfers.*"     # All token-transfer assets
nats sub "stellar.transfers.USDC"  # USDC events
nats sub "stellar.>"               # All events
```

### Example 4: JetStream for Reliability

```bash
# Use JetStream for durable, replicated storage
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  nats-sink --url nats://localhost:4222 \
    --subject "stellar.events" \
    --jetstream

# JetStream provides:
# - At-least-once delivery
# - Message persistence
# - Replay capability
# - Horizontal scalability
```

### Example 5: Filter Before Publishing

```bash
# Only publish USDC transfers
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  jq -c 'select(.transfer != null and .transfer.assetCode == "USDC")' | \
  nats-sink --subject "stellar.usdc.transfers"
```

### Example 6: Multi-Sink Fan-Out

```bash
# Publish to NATS and save to file
token-transfer --start-ledger 60200000 --end-ledger 60200010 | \
  tee >(nats-sink --subject "stellar.events") | \
  json-file-sink --out backup.jsonl
```

### Example 7: Production with Credentials

```bash
# Use credentials file for authentication
token-transfer | \
  nats-sink \
    --url nats://production.example.com:4222 \
    --creds ~/.nats/prod.creds \
    --subject "stellar.prod.{transfer.assetCode}" \
    --jetstream
```

## Architecture Patterns

### Pattern 1: Real-Time Dashboard

```
nebu fetch (continuous) → token-transfer → nats-sink
                                              ↓
                                        NATS JetStream
                                              ↓
                                        WebSocket Server
                                              ↓
                                      Real-Time Dashboard
```

```bash
# Producer (run once, streams forever)
nebu fetch 60200000 0 | \
  token-transfer -q | \
  nats-sink --subject "stellar.live.{transfer.assetCode}" --jetstream
```

### Pattern 2: Multi-Consumer Analytics

```
token-transfer → nats-sink → NATS → ┬→ Database Writer
                                     ├→ Analytics Engine
                                     ├→ Alerting System
                                     └→ Metrics Collector
```

```bash
# Producer
token-transfer | nats-sink --subject "stellar.transfers.{transfer.assetCode}"

# Consumer 1: Database
nats sub "stellar.>" | database-writer

# Consumer 2: Alerts
nats sub "stellar.transfers.USDC" | alert-on-large-transfers

# Consumer 3: Metrics
nats sub "stellar.>" | prometheus-exporter
```

### Pattern 3: Event Replay

```bash
# Publish with JetStream
token-transfer --start-ledger 60200000 --end-ledger 60300000 | \
  nats-sink --subject "stellar.events" --jetstream

# Later: Replay from start
nats sub "stellar.events" --deliver all
```

## JetStream Setup

For production use with JetStream:

### 1. Create Stream

```bash
# Create stream to store all stellar events
nats stream add STELLAR_EVENTS \
  --subjects "stellar.>" \
  --retention limits \
  --max-msgs=-1 \
  --max-age=7d \
  --storage file \
  --replicas 3
```

### 2. Publish with JetStream

```bash
contract-events | \
  nats-sink \
    --subject "stellar.{eventType}" \
    --jetstream
```

### 3. Create Consumers

```bash
# Durable consumer for processing
nats consumer add STELLAR_EVENTS PROCESSOR \
  --filter "stellar.transfer" \
  --deliver all \
  --ack explicit \
  --max-deliver 3
```

## Performance

### Throughput

- **Core NATS**: 100,000+ msgs/sec
- **JetStream**: 50,000+ msgs/sec (with ack)
- **Typical pipeline**: 1,000-5,000 events/sec

### Resource Usage

- **Memory**: ~10MB base + NATS client overhead
- **CPU**: Minimal (< 5% on modern CPU)
- **Network**: Depends on event size and rate

## Troubleshooting

### Connection Failed

```bash
Error: failed to connect to NATS at nats://localhost:4222
```

**Solution:**
- Verify NATS server is running: `nats server check`
- Check URL is correct
- Check firewall/network connectivity

### JetStream Not Enabled

```bash
Error: failed to create JetStream context: JetStream not enabled
```

**Solution:**
```bash
# Start NATS with JetStream enabled
nats-server -js
```

### Events Not Appearing

**Check 1:** Verify subject subscription matches publish subject
```bash
# Publisher
nats-sink --subject "stellar.events"

# Subscriber must match
nats sub "stellar.events"  # ✅ Correct
nats sub "stellar.transfers"  # ❌ Wrong
```

**Check 2:** Use wildcards for dynamic subjects
```bash
# Publisher uses template
nats-sink --subject "stellar.{eventType}"

# Subscriber should use wildcard
nats sub "stellar.>"    # Matches all
nats sub "stellar.fee"  # Matches only fee events
```

## Integration with nebu

### Registry Entry

`nats-sink` is registered in `registry.yaml`:

```yaml
- name: nats-sink
  type: sink
  description: Publish events to NATS message bus
  location:
    type: local
    path: ./examples/processors/nats-sink
```

### Build

```bash
# Build all processors
make build-processors

# Binary location
./bin/nats-sink
```

## Comparison with Other Sinks

| Feature | nats-sink | json-file-sink | duckdb-sink |
|---------|-----------|----------------|-------------|
| **Real-time** | ✅ Yes | ❌ No | ❌ No |
| **Multi-consumer** | ✅ Yes | ❌ No | ❌ No |
| **Persistent** | ✅ JetStream | ✅ File | ✅ Database |
| **Queryable** | ❌ No | ⚠️ Manual | ✅ SQL |
| **Distributed** | ✅ Yes | ❌ No | ❌ No |

**Use nats-sink when:**
- Building real-time systems
- Multiple services need the same events
- Event-driven architecture
- Scaling across multiple machines

**Use json-file-sink when:**
- Archiving events
- Single-machine processing
- Debugging pipelines

**Use duckdb-sink when:**
- Ad-hoc analytics
- SQL queries on events
- Local data warehouse

## Advanced Configuration

### NATS Cluster

```bash
# Connect to NATS cluster
nats-sink --url nats://server1:4222,server2:4222,server3:4222
```

### TLS Connection

```bash
# Use TLS (server must support it)
nats-sink --url tls://secure.example.com:4222 \
  --creds /path/to/creds.file
```

### Custom Connection Name

```bash
# Set connection name for monitoring
nats-sink --url nats://localhost:4222 \
  --name "stellar-pipeline-prod"

# View in NATS monitoring
nats server info
```

## Contributing

Found a bug or want a feature? Open an issue or PR!

## License

Part of the nebu project - see root LICENSE file.

## Resources

- [NATS Documentation](https://docs.nats.io/)
- [JetStream Guide](https://docs.nats.io/nats-concepts/jetstream)
- [nebu Documentation](https://github.com/withObsrvr/nebu)
- [NATS CLI](https://github.com/nats-io/natscli)
