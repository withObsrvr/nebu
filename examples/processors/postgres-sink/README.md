# postgres-sink

Store nebu events in PostgreSQL with JSONB schema and automatic idempotency.

## Features

- **JSONB Storage**: Flexible schema that works with any processor output
- **TOID-based Idempotency**: Automatic deduplication using Stellar's Total Order IDs (SEP-35)
- **Batched COPY**: 100x faster than row-by-row inserts
- **Production Ready**: Connection pooling, retry logic, graceful shutdown
- **Auto-Schema**: Creates tables and indexes automatically

## Installation

```bash
# Install from nebu repo
nebu install postgres-sink

# Or build manually
go install github.com/withObsrvr/nebu/examples/processors/postgres-sink/cmd/postgres-sink@latest
```

## Quick Start

### 1. Start PostgreSQL

```bash
# Using Docker
docker run --name nebu-postgres \
  -e POSTGRES_PASSWORD=postgres \
  -p 5432:5432 \
  -d postgres:16

# Create database
docker exec nebu-postgres createdb -U postgres stellar
```

### 2. Stream Events

```bash
# Stream token transfers to Postgres
token-transfer --start-ledger 60200000 --follow | \
  postgres-sink --dsn "postgres://postgres:postgres@localhost/stellar"
```

### 3. Query Events

```sql
-- Find all USDC transfers
SELECT
  id,
  data->'transfer'->>'from' as sender,
  data->'transfer'->>'to' as recipient,
  (data->'transfer'->>'amount')::numeric as amount,
  created_at
FROM events
WHERE data->>'type' = 'transfer'
  AND data->'transfer'->'asset'->>'code' = 'USDC'
ORDER BY created_at DESC
LIMIT 10;
```

## Usage

### Basic Usage

```bash
postgres-sink --dsn "postgres://user:password@localhost/db"
```

### Flags

```
--dsn string           PostgreSQL connection string (required, or set POSTGRES_DSN)
--table string         Table name for storing events (default: "events")
--batch-size int       Events per batch before COPY (default: 1000)
--conflict string      Conflict resolution: "ignore" or "update" (default: "ignore")
```

### Environment Variables

```bash
export POSTGRES_DSN="postgres://user:password@localhost/db"
postgres-sink < events.jsonl
```

## Schema

postgres-sink automatically creates this table:

```sql
CREATE TABLE events (
    id BIGINT PRIMARY KEY,           -- TOID (deterministic, unique per event)
    event_type TEXT,                 -- Auto-detected from "type" field or protobuf oneof
    data JSONB NOT NULL,             -- Full event JSON
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for common queries
CREATE INDEX idx_events_data ON events USING GIN (data);
CREATE INDEX idx_events_event_type ON events (event_type) WHERE event_type IS NOT NULL;
CREATE INDEX idx_events_created_at ON events (created_at);
```

### Event Type Detection

postgres-sink automatically detects the event type from:
- Simple `"type"` field (e.g., `{"type": "transfer", ...}`)
- Protobuf oneof fields (e.g., `{"transfer": {...}, "meta": {...}}`)

Supported types: `transfer`, `mint`, `burn`, `clawback`, `fee`, `payment`, `invoke`

Example queries using event_type:
```sql
-- Count events by type
SELECT event_type, COUNT(*)
FROM events
GROUP BY event_type;

-- Filter by type
SELECT * FROM events WHERE event_type = 'transfer';
```

## Examples

### Store Token Transfers

```bash
token-transfer --start-ledger 60200000 --follow | \
  postgres-sink \
    --dsn "postgres://localhost/stellar" \
    --table token_events \
    --batch-size 5000
```

### Store Contract Events

```bash
contract-events --start-ledger 60200000 --follow | \
  postgres-sink \
    --dsn "postgres://localhost/stellar" \
    --table contract_events
```

### Filter and Store USDC Only

```bash
token-transfer --start-ledger 60200000 --follow | \
  jq -c 'select(.transfer.asset.code == "USDC")' | \
  postgres-sink \
    --table usdc_transfers \
    --conflict update
```

## Querying JSON Data

### Basic Queries

```sql
-- Count events by type
SELECT
  event_type,
  COUNT(*) as count
FROM events
WHERE event_type IS NOT NULL
GROUP BY event_type
ORDER BY count DESC;

-- Find large transfers
SELECT
  id,
  (data->'transfer'->>'amount')::numeric as amount,
  data->'transfer'->'asset'->>'code' as asset
FROM events
WHERE data->>'type' = 'transfer'
  AND (data->'transfer'->>'amount')::numeric > 1000000
ORDER BY amount DESC;
```

### Create Views for Convenience

```sql
-- USDC transfers view
CREATE VIEW usdc_transfers AS
SELECT
  id,
  (data->'transfer'->>'amount')::numeric as amount,
  data->'transfer'->>'from' as from_account,
  data->'transfer'->>'to' as to_account,
  data->'meta'->>'txHash' as tx_hash,
  created_at
FROM events
WHERE data->>'type' = 'transfer'
  AND data->'transfer'->'asset'->>'code' = 'USDC';

-- Query the view
SELECT * FROM usdc_transfers
WHERE amount > 100
ORDER BY created_at DESC
LIMIT 10;
```

### Add Extracted Columns (Phase 2)

For better query performance, add columns via trigger:

```sql
-- Add columns
ALTER TABLE events
  ADD COLUMN amount NUMERIC,
  ADD COLUMN asset_code TEXT,
  ADD COLUMN from_account TEXT,
  ADD COLUMN to_account TEXT;

-- Extract on insert
CREATE OR REPLACE FUNCTION extract_transfer_fields()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.data->>'type' = 'transfer' THEN
        NEW.amount = (NEW.data->'transfer'->>'amount')::NUMERIC;
        NEW.asset_code = NEW.data->'transfer'->'asset'->>'code';
        NEW.from_account = NEW.data->'transfer'->>'from';
        NEW.to_account = NEW.data->'transfer'->>'to';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER extract_on_insert
BEFORE INSERT ON events
FOR EACH ROW EXECUTE FUNCTION extract_transfer_fields();

-- Add indexes
CREATE INDEX idx_events_asset_code ON events (asset_code) WHERE asset_code IS NOT NULL;
CREATE INDEX idx_events_amount ON events (amount) WHERE amount IS NOT NULL;
CREATE INDEX idx_events_from ON events (from_account) WHERE from_account IS NOT NULL;

-- Now queries are fast!
SELECT * FROM events
WHERE asset_code = 'USDC'
  AND amount > 100
ORDER BY created_at DESC;
```

## Idempotency

postgres-sink uses **TOIDs (Total Order IDs)** for idempotent inserts. TOIDs are deterministic IDs calculated from:
- Ledger sequence number
- Transaction index
- Operation index

This means:
- ✅ **Restart safe**: Re-processing the same ledgers won't create duplicates
- ✅ **SEP-35 compliant**: Compatible with Stellar Horizon's ID format
- ✅ **Unique per event**: Each operation gets a globally unique ID

```sql
-- Check for duplicates (should be 0)
SELECT id, COUNT(*)
FROM events
GROUP BY id
HAVING COUNT(*) > 1;
```

## Performance

### Benchmarks

| Method | Events/sec | Time for 1M events |
|--------|------------|-------------------|
| Row-by-row INSERT | 500 | 33 minutes |
| Batched COPY (size=1000) | 50,000 | 20 seconds |
| Batched COPY (size=5000) | 75,000 | 13 seconds |

### Tuning

```bash
# Increase batch size for backfill
postgres-sink --batch-size 10000

# Smaller batches for real-time (lower latency)
postgres-sink --batch-size 100
```

## Conflict Resolution

### Mode: ignore (default)

Ignores duplicates on restart:

```bash
postgres-sink --conflict ignore  # ON CONFLICT DO NOTHING
```

### Mode: update

Updates existing records:

```bash
postgres-sink --conflict update  # ON CONFLICT DO UPDATE
```

## Production Tips

### Connection Pooling

postgres-sink uses connection pooling automatically:
- Max open connections: 25
- Max idle connections: 5
- Connection max lifetime: 5 minutes

### Graceful Shutdown

Press Ctrl+C to flush pending batches before exit:

```
Received shutdown signal, flushing...
✓ Flushed 437 events
```

### Monitoring

```sql
-- Check insertion rate
SELECT
  DATE_TRUNC('minute', created_at) as minute,
  COUNT(*) as events_per_minute
FROM events
WHERE created_at > NOW() - INTERVAL '1 hour'
GROUP BY minute
ORDER BY minute DESC;

-- Table size
SELECT
  pg_size_pretty(pg_total_relation_size('events')) as total_size,
  pg_size_pretty(pg_relation_size('events')) as table_size,
  pg_size_pretty(pg_indexes_size('events')) as indexes_size;
```

## Troubleshooting

### Connection refused

```bash
# Check Postgres is running
pg_isready

# Test connection
psql "postgres://localhost/stellar"
```

### Slow queries

```sql
-- Check index usage
SELECT
  schemaname,
  tablename,
  indexname,
  idx_scan,
  idx_tup_read
FROM pg_stat_user_indexes
WHERE tablename = 'events';

-- Analyze query plans
EXPLAIN ANALYZE
SELECT * FROM events WHERE data->>'type' = 'transfer';
```

### Out of disk space

```sql
-- Vacuum old data
VACUUM FULL events;

-- Archive old events
DELETE FROM events WHERE created_at < NOW() - INTERVAL '90 days';
```

## Roadmap (Phase 2)

- [ ] Multiple ID strategies (`--id-mode field/hash/serial`)
- [ ] Column extraction (`--extract "amount:numeric,asset:text"`)
- [ ] Complex mapping configs
- [ ] Checkpoint/resume support

See `docs/shapes/postgres-sink-flexibility.md` for details.

## See Also

- [TOID Package](../../../pkg/toid/) - TOID generation library
- [Shape Document](../../../docs/shapes/postgres-sink.md) - Implementation plan
- [SEP-35 Spec](https://github.com/stellar/stellar-protocol/blob/master/ecosystem/sep-0035.md) - TOID standard
