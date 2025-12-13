# DuckDB Query Examples

This directory contains reusable SQL queries for analyzing nebu event streams with DuckDB.

## Available Queries

### Payment Event Extraction

#### [`extract_payment_events.sql`](./extract_payment_events.sql)

Extracts structured payment events from contract events (any contract). Demonstrates how to:
- Navigate deeply nested JSON from contract events
- Convert between data types (bytes to hex strings, JSON to integers)
- Flatten nested structures into analyzable tables
- Filter events by type

**Usage:**
```bash
# Stream to stdout
contract-events --start-ledger 60200000 --end-ledger 60300000 | \
  duckdb -c "$(cat examples/queries/extract_payment_events.sql)" -json

# Save to table
contract-events --start-ledger 60200000 --end-ledger 60300000 | \
  duckdb payments.db -c "CREATE TABLE payments AS $(cat examples/queries/extract_payment_events.sql)"
```

#### [`extract_payment_events_filtered.sql`](./extract_payment_events_filtered.sql)

Same as above but filtered to a specific contract address. Edit the `WHERE` clause to specify your contract.

**Usage:**
```bash
# Update the contract address in the file first, then run:
contract-events --start-ledger 60200000 --end-ledger 60300000 | \
  duckdb -c "$(cat examples/queries/extract_payment_events_filtered.sql)" -json
```

### Token Transfer Filtering

#### [`usdc_by_contract.sql`](./usdc_by_contract.sql)

Filters token-transfer events for USDC transfers from a specific contract. Shows how to combine asset and contract filters.

**Usage:**
```bash
# Update the contract address in the file first, then run:
token-transfer --start-ledger 60200000 --end-ledger 60300000 | \
  duckdb -c "$(cat examples/queries/usdc_by_contract.sql)" -json
```

## Common Filtering Patterns

### Filter by Contract Address

**contract-events:**
```sql
WHERE contractId = 'CXXXXX...'
```

**token-transfer:**
```sql
WHERE json_extract_string(meta, '$.contractAddress') = 'CXXXXX...'
```

### Filter by Event Type

**contract-events:**
```sql
WHERE eventType = 'payment'  -- or 'transfer', 'mint', etc.
```

**token-transfer:**
```sql
WHERE transfer IS NOT NULL  -- for transfers
WHERE mint IS NOT NULL      -- for mints
WHERE burn IS NOT NULL      -- for burns
```

### Filter by Transaction Success

**contract-events only** (token-transfer does not have this field):
```sql
WHERE inSuccessfulTx = true  -- Only successful transactions

WHERE inSuccessfulTx = false  -- Only failed transactions
```

**Important:** Failed transactions can still emit events. Always filter by `inSuccessfulTx = true` unless you specifically want to analyze failures.

### Filter by Asset

**token-transfer:**
```sql
-- USDC
WHERE json_extract_string(transfer, '$.asset.issuedAsset.assetCode') = 'USDC'

-- Native XLM
WHERE json_extract_string(transfer, '$.asset.native') = 'true'

-- Any issued asset
WHERE transfer IS NOT NULL
  AND json_extract(transfer, '$.asset.issuedAsset') IS NOT NULL
```

### Combine Multiple Filters

**token-transfer:**
```sql
-- USDC from specific contract with minimum amount
WHERE transfer IS NOT NULL
  AND json_extract_string(transfer, '$.asset.issuedAsset.assetCode') = 'USDC'
  AND json_extract_string(meta, '$.contractAddress') = 'CXXXXX...'
  AND CAST(json_extract_string(transfer, '$.amount') AS BIGINT) >= 10000000
```

**contract-events:**
```sql
-- Payment events from specific contract (successful only)
WHERE eventType = 'payment'
  AND contractId = 'CXXXXX...'
  AND inSuccessfulTx = true
  AND CAST(json_extract(to_json(dataDecoded), '$.entries.amount') AS UBIGINT) >= 10000000
```

## Dynamic Filtering with Variables

Instead of editing SQL files, use bash variables:

```bash
CONTRACT="CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA"
ASSET="USDC"

token-transfer --start-ledger 60200000 --end-ledger 60200001 | \
  duckdb -c "
    SELECT *
    FROM read_json_auto('/dev/stdin')
    WHERE transfer IS NOT NULL
      AND json_extract_string(transfer, '\$.asset.issuedAsset.assetCode') = '$ASSET'
      AND json_extract_string(meta, '\$.contractAddress') = '$CONTRACT'
  "
```

## Testing Queries

Use the test script to verify payment extraction works:

```bash
./examples/queries/test_extract_payment_events.sh
```

## Adding Your Own Queries

1. Create a `.sql` file with descriptive name (e.g., `analyze_usdc_volume.sql`)
2. Add comments explaining what the query does and how to use it
3. Test with real data: `processor-name | duckdb -c "$(cat your_query.sql)"`
4. Update this README with a brief description

## Why SQL Queries Instead of Custom Processors?

Many data transformations can be expressed as SQL queries instead of custom Go code. Benefits:

- **Faster iteration** - modify query in seconds vs recompiling code
- **Ad-hoc exploration** - test hypotheses without writing processors
- **Built-in power** - aggregations, window functions, joins, exports
- **Zero maintenance** - no Go dependencies or version management
- **Easier sharing** - SQL queries are portable and readable

See the [DuckDB Cookbook](../../docs/DUCKDB_COOKBOOK.md) for more examples and patterns.
