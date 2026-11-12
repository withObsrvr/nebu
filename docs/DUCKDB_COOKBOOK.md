# DuckDB Cookbook

DuckDB can read JSON directly from stdin using `read_json('/dev/stdin')`, making it perfect for analyzing nebu event streams without custom sinks. The JSON extension is auto-loaded in modern DuckDB versions.

**Why DuckDB instead of custom processors?**
- **Iterate faster** - modify queries in seconds instead of recompiling Go code
- **Ad-hoc analysis** - explore data without writing code
- **Built-in power** - aggregations, window functions, joins, exports
- **Zero maintenance** - no custom processors to maintain

## Table of Contents

- [Basic Ingestion](#basic-ingestion)
- [If you are running these from an AI agent](#if-you-are-running-these-from-an-ai-agent)
- [Getting the numbers right](#getting-the-numbers-right)
- [Ad-Hoc Analytics](#ad-hoc-analytics-no-persistence)
- [Transform on Ingestion](#transform-on-ingestion)
- [Multi-Table Analytics](#multi-table-analytics)
- [Time-Series Analysis](#time-series-analysis)
- [Address Activity Tracking](#address-activity-tracking)
- [Extracting Structured Data from Contract Events](#extracting-structured-data-from-contract-events)
- [Verifying coverage](#verifying-coverage)
- [Querying the status stream](#querying-the-status-stream)
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
duckdb events.db -c "
  SELECT
    CASE
      WHEN json_exists(to_json(transfers), '$.transfer') AND transfer IS NOT NULL THEN 'transfer'
      WHEN json_exists(to_json(transfers), '$.mint') AND mint IS NOT NULL THEN 'mint'
      WHEN json_exists(to_json(transfers), '$.fee') AND fee IS NOT NULL THEN 'fee'
      ELSE 'other'
    END AS event_type,
    COUNT(*) AS count
  FROM transfers
  GROUP BY event_type
"
```

## If you are running these from an AI agent

DuckDB 2.0.0 enables an "agent mode" when `AI_AGENT` or `CLAUDECODE` is set
and stdout is not a terminal. Its documented effects are presentational —
markdown tables, row caps, JSON errors on stderr — but it also collapses
`read_json`'s inferred schema to a single `json` column, so a recipe below
fails with a binder error that names a column you can see in the data:

```
Referenced column "contractId" not found in FROM clause!
Candidate bindings: "json"
```

Turn it off and the schema comes back:

```bash
export DUCKDB_AGENT_MODE=0     # or pass -no-agent per invocation
```

`just docs-smoke` and the nix dev shell already pin it off, so the repo's own
checks mean the same thing whichever way they are run. DuckDB 1.x is
unaffected and ignores the variable.

Double-quoted paths — `read_json("/dev/stdin")` — also now raise a
deprecation warning in 2.0.0. Single quotes are the string literal; double
quotes are an identifier. Every recipe here uses single quotes.

## Getting the numbers right

Three things about this data will produce plausible, wrong answers if you
don't know them. They apply to every query below.

### Amounts are integers wider than a double

Amounts are stroops — 1/10,000,000 of a unit — as decimal strings, and they
reach into `int64`. `DOUBLE` holds integers exactly only up to 2^53 (about
9.0 × 10^15), and single Stellar amounts already exceed that: a transfer of
4.06 × 10^16 stroops is an ordinary XLM movement.

```sql
-- Wrong: silently loses precision.
SUM(CAST(json_extract_string(transfer, '$.amount') AS DOUBLE))

-- Right: exact.
SUM(json_extract_string(transfer, '$.amount')::HUGEINT)
```

Over 51 ledgers of pubnet traffic the two disagree by 648 stroops. The error
grows with the range and never announces itself, which is why it is worth
getting right in the first query rather than the audit.

Divide by `1e7` only at the end, for display.

### Fee events include failed transactions

A failed transaction still pays its fee, so `fee` events are emitted for
transactions that did nothing else. In the same 51-ledger sample, **39% of fee
events came from failed transactions**. Asset movements do not have this
problem — `transfer`, `mint` and `burn` events are only derived from
successful operations — but any query that sums fees, or counts "events per
ledger", is mixing the two unless you say otherwise:

```sql
-- Fees actually paid by successful activity.
SELECT SUM(fee.amount::HUGEINT) AS fees
FROM read_json('/dev/stdin')
WHERE fee IS NOT NULL AND meta.inSuccessfulTx
```

`meta.inSuccessfulTx` is the filter, and it is on every event.

### Close times are whole seconds

`meta.closedAtUnix` is a **string** of whole seconds, not a number and not
milliseconds. Cast it explicitly (`::BIGINT`) and convert with
`to_timestamp()`.

It is also not a safe sort key. Protocol 30's CAP-88 gives ledger close times
millisecond resolution, so two consecutive ledgers can close within the same
second — second-resolution bucketing and ordering stop being deterministic
rather than producing an error. Bucket and order by `meta.ledgerSequence` when
the answer has to be deterministic, and treat the close time as a label.

## Ad-Hoc Analytics (No Persistence)

Query event streams directly without saving to disk:

```bash
# Count transfers by asset
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    SELECT
      json_extract_string(transfer, '$.assetCode') as asset_code,
      COUNT(*) as transfer_count,
      SUM(json_extract_string(transfer, '$.amount')::HUGEINT) as total_volume
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
        SUM(json_extract_string(transfer, '$.amount')::HUGEINT) as volume
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
    WITH events AS (
      SELECT * FROM read_json('/dev/stdin')
    ),
    address_activity AS (
      SELECT json_extract_string(transfer, '$.from') as address, COUNT(*) as sends, 0 as receives
      FROM events
      WHERE transfer IS NOT NULL AND json_extract_string(transfer, '$.from') IS NOT NULL
      GROUP BY json_extract_string(transfer, '$.from')

      UNION ALL

      SELECT json_extract_string(transfer, '$.to') as address, 0 as sends, COUNT(*) as receives
      FROM events
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

## Verifying coverage

A dataset that is missing ledgers looks exactly like one that is complete.
`--on-ledger-error strict` makes nebu refuse to produce a partial dataset in
the first place, and nebu reports coverage on stderr at the end of a run — but
once the data is at rest, the question becomes a query.

Find the holes:

```bash
duckdb events.db -c "
  WITH seen AS (
    SELECT DISTINCT meta.ledgerSequence AS seq FROM transfers
  ),
  bounds AS (SELECT MIN(seq) AS lo, MAX(seq) AS hi FROM seen)
  SELECT g.seq AS missing_ledger
  FROM bounds, range(bounds.lo, bounds.hi + 1) AS g(seq)
  WHERE g.seq NOT IN (SELECT seq FROM seen)
  ORDER BY 1
"
```

Two caveats worth stating, because they are the difference between this query
being a check and being a false reassurance:

- It only finds gaps **inside** the range you actually loaded. It cannot know
  you meant to fetch 1,000 ledgers and stopped at 900. Compare `MIN`/`MAX`
  against what you asked for as well.
- A ledger with no matching events is indistinguishable from a missing one.
  Most ledgers contain token transfers, so this is a good signal in practice,
  but for an exact answer load an event type that appears in every ledger —
  fee events do — or keep the fetch's own checkpoint and coverage output.

## Querying the status stream

`nebu.status.v1` is NDJSON, so DuckDB reads a run's progress the same way it
reads the data. Every record carries a `run_id`, and under flowctl — or with
`NEBU_RUN_ID` set yourself — every stage of one pipeline shares it. That makes
a multi-process pipeline queryable as one thing:

```bash
export NEBU_RUN_ID=$(uuidgen)

NEBU_COMPONENT_ID=fetch nebu fetch --status-file fetch.ndjson 65430000 65430020 \
  | NEBU_COMPONENT_ID=transfers token-transfer --status-file transfers.ndjson - \
  > transfers.jsonl

duckdb -c "
  SELECT
    component_id,
    app,
    COUNT(*)            AS records,
    MAX(current_ledger) AS last_ledger,
    MAX(ledger_lag)     AS max_lag,
    any_value(state) FILTER (WHERE lifecycle <> 'running') AS final_state
  FROM read_json('*.ndjson', union_by_name = true)
  WHERE run_id = '$NEBU_RUN_ID'
  GROUP BY component_id, app
  ORDER BY component_id
"
```

```
┌──────────────┬────────────────┬─────────┬─────────────┬─────────┬─────────────┐
│ component_id │      app       │ records │ last_ledger │ max_lag │ final_state │
├──────────────┼────────────────┼─────────┼─────────────┼─────────┼─────────────┤
│ fetch        │ nebu           │      23 │    65430020 │      20 │ done        │
│ transfers    │ token-transfer │      23 │    65430020 │    NULL │ done        │
└──────────────┴────────────────┴─────────┴─────────────┴─────────┴─────────────┘
```

`union_by_name = true` is required: a stage reading from stdin has no target
ledger, so its records carry no `target_ledger` or `ledger_lag` and the two
files have different field sets. The `NULL` lag above is that, not a bug.

`final_state` is `done`, `error` or `idle` — `idle` meaning cancelled. The
exit code remains authoritative, but this tells you *which stage* ended badly,
which the exit code of a shell pipeline will not.

Processors must be built against nebu v0.7.1 or later to emit `run_id`; an
older binary writes the field as null.

## Export Results

Export query results to various formats:

```bash
# Export to CSV
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    COPY (
      SELECT
        CASE
          WHEN json_exists(to_json(e), '$.transfer') AND transfer IS NOT NULL THEN 'transfer'
          WHEN json_exists(to_json(e), '$.mint') AND mint IS NOT NULL THEN 'mint'
          WHEN json_exists(to_json(e), '$.fee') AND fee IS NOT NULL THEN 'fee'
          ELSE 'other'
        END as event_type,
        COUNT(*) as count
      FROM read_json('/dev/stdin') AS e
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
  SELECT meta.ledgerSequence AS ledger_sequence, meta.txHash AS tx_hash, COUNT(*)
  FROM transfers
  GROUP BY meta.ledgerSequence, meta.txHash
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
