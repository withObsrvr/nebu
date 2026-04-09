# DuckDB Cookbook

DuckDB can read JSON directly from stdin using `read_json('/dev/stdin')`, making it perfect for analyzing nebu event streams without custom sinks. The JSON extension is auto-loaded in modern DuckDB versions.

**Why DuckDB instead of custom processors?**
- **Iterate faster** - modify queries in seconds instead of recompiling Go code
- **Ad-hoc analysis** - explore data without writing code
- **Built-in power** - aggregations, window functions, joins, exports
- **Zero maintenance** - no custom processors to maintain

## Table of Contents

- [Basic Ingestion](#basic-ingestion)
- [Ad-Hoc Analytics](#ad-hoc-analytics-no-persistence)
- [Transform on Ingestion](#transform-on-ingestion)
- [Multi-Table Analytics](#multi-table-analytics)
- [Time-Series Analysis](#time-series-analysis)
- [Address Activity Tracking](#address-activity-tracking)
- [Extracting Structured Data from Contract Events](#extracting-structured-data-from-contract-events)
- [Export Results](#export-results)
- [Incremental Updates](#incremental-updates)
- [Tips](#tips)

---

## Basic Ingestion

Pipe events into a persistent DuckDB table:

```bash
# Create table from event stream
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb events.db -c "CREATE TABLE transfers AS SELECT * FROM read_json('/dev/stdin')"

# Query the data
duckdb events.db -c "SELECT type, COUNT(*) FROM transfers GROUP BY type"
```

## Ad-Hoc Analytics (No Persistence)

Query event streams directly without saving to disk:

```bash
# Count transfers by asset
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    SELECT
      json_extract_string(transfer, '$.assetCode') as asset_code,
      COUNT(*) as transfer_count,
      SUM(CAST(json_extract_string(transfer, '$.amount') AS DOUBLE)) as total_volume
    FROM read_json('/dev/stdin')
    WHERE transfer IS NOT NULL
    GROUP BY asset_code
    ORDER BY total_volume DESC
  "

# Find largest transfers
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    SELECT
      json_extract_string(transfer, '$.from') as from_address,
      json_extract_string(transfer, '$.to') as to_address,
      CAST(json_extract_string(transfer, '$.amount') AS DOUBLE) / 10000000.0 as amount_decimal,
      json_extract_string(transfer, '$.assetCode') as asset
    FROM read_json('/dev/stdin')
    WHERE transfer IS NOT NULL
    ORDER BY CAST(json_extract_string(transfer, '$.amount') AS DOUBLE) DESC
    LIMIT 10
  "
```

## Transform on Ingestion

Apply SQL transformations while ingesting data:

```bash
# Filter and transform USDC transfers only
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb analytics.db -c "
    CREATE TABLE usdc_transfers AS
    SELECT
      json_extract(meta, '$.ledgerSequence') as ledger_sequence,
      json_extract_string(meta, '$.txHash') as tx_hash,
      json_extract_string(transfer, '$.from') as \"from\",
      json_extract_string(transfer, '$.to') as \"to\",
      CAST(json_extract_string(transfer, '$.amount') AS BIGINT) as amount_stroops,
      CAST(json_extract_string(transfer, '$.amount') AS BIGINT) / 10000000.0 as amount_usd
    FROM read_json('/dev/stdin')
    WHERE
      transfer IS NOT NULL
      AND json_extract_string(transfer, '$.assetCode') = 'USDC'
  "

# Query transformed data
duckdb analytics.db -c "
  SELECT
    ledger_sequence,
    COUNT(*) as transfers,
    SUM(amount_usd) as volume_usd
  FROM usdc_transfers
  GROUP BY ledger_sequence
  ORDER BY ledger_sequence
"
```

## Multi-Table Analytics

Create multiple related tables from a single stream:

```bash
# Process stream once, create multiple views
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb analytics.db -c "
    CREATE TABLE all_events AS SELECT * FROM read_json('/dev/stdin');

    CREATE VIEW transfers AS
      SELECT * FROM all_events WHERE transfer IS NOT NULL;

    CREATE VIEW mints AS
      SELECT * FROM all_events WHERE mint IS NOT NULL;

    CREATE VIEW burns AS
      SELECT * FROM all_events WHERE burn IS NOT NULL;
  "

# Cross-table analytics
duckdb analytics.db -c "
  SELECT
    'transfers' as event_type, COUNT(*) FROM transfers
  UNION ALL
  SELECT 'mints', COUNT(*) FROM mints
  UNION ALL
  SELECT 'burns', COUNT(*) FROM burns
"
```

## Time-Series Analysis

Analyze event patterns over time:

```bash
token-transfer --start-ledger 60200000 --end-ledger 60300000 | \
  duckdb -c "
    WITH ledger_stats AS (
      SELECT
        json_extract(meta, '$.ledgerSequence') as ledger_sequence,
        json_extract_string(transfer, '$.assetCode') as asset_code,
        COUNT(*) as event_count,
        SUM(CAST(json_extract_string(transfer, '$.amount') AS DOUBLE)) as volume
      FROM read_json('/dev/stdin')
      WHERE transfer IS NOT NULL
      GROUP BY ledger_sequence, asset_code
    )
    SELECT
      ledger_sequence,
      asset_code,
      event_count,
      volume,
      AVG(volume) OVER (
        PARTITION BY asset_code
        ORDER BY ledger_sequence
        ROWS BETWEEN 99 PRECEDING AND CURRENT ROW
      ) as moving_avg_100_ledgers
    FROM ledger_stats
    ORDER BY ledger_sequence, volume DESC
  "
```

## Address Activity Tracking

Track top addresses by activity:

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    WITH address_activity AS (
      SELECT json_extract_string(transfer, '$.from') as address, COUNT(*) as sends, 0 as receives
      FROM read_json('/dev/stdin')
      WHERE transfer IS NOT NULL AND json_extract_string(transfer, '$.from') IS NOT NULL
      GROUP BY json_extract_string(transfer, '$.from')

      UNION ALL

      SELECT json_extract_string(transfer, '$.to') as address, 0 as sends, COUNT(*) as receives
      FROM read_json('/dev/stdin')
      WHERE transfer IS NOT NULL AND json_extract_string(transfer, '$.to') IS NOT NULL
      GROUP BY json_extract_string(transfer, '$.to')
    )
    SELECT
      address,
      SUM(sends) as total_sends,
      SUM(receives) as total_receives,
      SUM(sends + receives) as total_activity
    FROM address_activity
    GROUP BY address
    ORDER BY total_activity DESC
    LIMIT 20
  "
```

## Extracting Structured Data from Contract Events

When working with contract events, you often need to navigate deeply nested JSON structures and reshape them into flat, analyzable tables. DuckDB excels at this - **replacing hundreds of lines of custom Go code with a single SQL query**.

### Example: Payment Events from Kwickbit Contract

The `contract-events` processor outputs raw contract events with nested data like:

```json
{
  "eventType": "payment",
  "dataDecoded": {
    "entries": {
      "payment_id": {"hex": "abc123"},
      "token": {"address": "CA..."},
      "amount": 1000000,
      "from": {"address": "GA..."},
      "merchant": {"address": "GB..."},
      "royalty_amount": 50000
    }
  },
  "meta": {
    "ledgerSequence": 60200000,
    "txHash": "165fee...",
    "closedAt": "2025-12-08T01:45:11Z"
  }
}
```

**Instead of writing a custom processor**, use a SQL query:

```bash
# Extract payment events to JSON
contract-events --start-ledger 60200000 --end-ledger 60300000 | \
  duckdb -c "$(cat examples/queries/extract_payment_events.sql)" -json

# Or save to a table
contract-events --start-ledger 60200000 --end-ledger 60300000 | \
  duckdb payments.db -c "
    CREATE TABLE payments AS $(cat examples/queries/extract_payment_events.sql)
  "

# Or pipe to another processor
contract-events --start-ledger 60200000 --end-ledger 60300000 | \
  duckdb -c "$(cat examples/queries/extract_payment_events.sql)" -json | \
  my-analytics-processor
```

See [`examples/queries/extract_payment_events.sql`](../examples/queries/extract_payment_events.sql) for the full query with comments.

**Query highlights:**
- **Nested JSON navigation**: `json_extract_string(event, '$.dataDecoded.entries.payment_id.hex')`
- **Type conversion**: `CAST(json_extract(...) AS UBIGINT)`
- **String manipulation**: `'0x' || json_extract_string(...)` to add hex prefix
- **Filtering**: `WHERE json_extract_string(event, '$.eventType') = 'payment'`

**Result:** Flat, analyzable structure ready for dashboards, exports, or further processing.

### When to use DuckDB vs Custom Processors

**Use DuckDB when:**
- Extracting/reshaping nested JSON fields
- Filtering by event types or field values
- Aggregating, grouping, or joining data
- You want fast iteration (modify query, not code)
- You need ad-hoc exploration

**Use custom processors when:**
- Complex stateful logic (e.g., detecting multi-event patterns)
- Real-time alerting or webhooks
- Side effects beyond data transformation
- Performance-critical hot paths

**Rule of thumb:** Start with DuckDB. Only write custom processors when SQL becomes too complex or you need side effects.

## Export Results

Export query results to various formats:

```bash
# Export to CSV
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    COPY (
      SELECT
        CASE
          WHEN transfer IS NOT NULL THEN 'transfer'
          WHEN mint IS NOT NULL THEN 'mint'
          WHEN burn IS NOT NULL THEN 'burn'
          WHEN fee IS NOT NULL THEN 'fee'
          ELSE 'other'
        END as event_type,
        COUNT(*) as count
      FROM read_json('/dev/stdin')
      GROUP BY event_type
    ) TO 'summary.csv' (HEADER, DELIMITER ',')
  "

# Export to Parquet (columnar format)
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    COPY (
      SELECT * FROM read_json('/dev/stdin')
    ) TO 'transfers.parquet' (FORMAT PARQUET)
  "

# Export to JSON
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    COPY (
      SELECT * FROM read_json('/dev/stdin')
      WHERE transfer IS NOT NULL
    ) TO 'transfers.json' (FORMAT JSON)
  "
```

## Incremental Updates

Append new data to existing tables:

```bash
# Initial load
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb events.db -c "CREATE TABLE transfers AS SELECT * FROM read_json('/dev/stdin')"

# Append new data
token-transfer --start-ledger 60200101 --end-ledger 60200200 | \
  duckdb events.db -c "INSERT INTO transfers SELECT * FROM read_json('/dev/stdin')"

# Check for duplicates
duckdb events.db -c "
  SELECT ledger_sequence, tx_hash, COUNT(*)
  FROM transfers
  GROUP BY ledger_sequence, tx_hash
  HAVING COUNT(*) > 1
"
```

## Tips

- **Schema Detection**: DuckDB auto-detects JSON schema with `read_json_auto()`. Top-level fields become columns, nested objects remain as JSON strings
- **Extract Nested Fields**: Use `json_extract_string(column, '$.path')` for nested data within columns
- **Debugging Queries**:
  - See what columns are available: `... | duckdb -c "SELECT * FROM read_json_auto('/dev/stdin') LIMIT 1"`
  - Check column names: `... | duckdb -c "DESCRIBE SELECT * FROM read_json_auto('/dev/stdin')"`
  - Run queries with `LIMIT 1` first to catch errors quickly
- **Performance**: For large datasets, use Parquet format or create indexes on frequently queried columns
- **Memory**: DuckDB is in-process and memory-efficient, but very large streams may need batching
- **Parallel Processing**: Run multiple processors for different ledger ranges, then `UNION ALL` in DuckDB
- **Reusable Queries**: Save queries to `.sql` files in `examples/queries/` for easy reuse via `$(cat query.sql)`
- **Output Formats**: Use `-json` for JSON output, `-csv` for CSV, or omit for table format

---

**More examples?** Contributions welcome! Add your queries to `examples/queries/` and submit a PR.
