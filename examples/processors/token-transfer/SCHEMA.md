# Token Transfer JSON Schema

This document defines the JSON output schema for the token-transfer origin processor.

## Current Version: v1

**Schema identifier**: `nebu.token_transfer.v1`
**First released**: nebu v0.3.0
**Status**: Stable

---

## Changelog

### v1 (2025-12-12)
- Initial schema version
- Supports event types: transfer, mint, burn, clawback, fee
- All events include metadata fields: `_schema`, `_nebu_version`
- Base fields: `ledger_sequence`, `tx_hash`, `type`
- Optional fields: `contract_address`, `asset`
- Type-specific fields: `from`, `to`, `amount`

---

## Versioning Policy

### Breaking Changes (bump schema version)

The following changes will increment the schema version (v1 → v2):

- **Rename field**: `"from"` → `"from_address"`
- **Remove field**: Delete `"contract_address"`
- **Change type**: `"amount"` string → number
- **Change structure**: Flatten nested `asset` object
- **Change enum values**: `"transfer"` → `"xfer"`

### Non-Breaking Changes (keep schema version)

The following changes do NOT increment the schema version:

- **Add new field**: Add `"timestamp"` to events
- **Add new event type**: Support `"approve"` event type
- **Add new optional field**: Add optional `"memo"` field
- **Expand enum values**: Add new asset types

When non-breaking changes are made, the schema version stays at `v1`, but the changelog will document additions.

---

## Schema Structure

All events from the token-transfer processor share this base structure:

```json
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "0.3.0",
  "ledger_sequence": 60200000,
  "tx_hash": "abc123...",
  "type": "transfer",
  "contract_address": "CA...",
  "asset": {
    "code": "USDC",
    "issuer": "GA..."
  },
  ...type-specific fields...
}
```

### Metadata Fields

Every event includes these metadata fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `_schema` | string | Yes | Schema version identifier (e.g., `"nebu.token_transfer.v1"`) |
| `_nebu_version` | string | Yes | nebu CLI version that produced this event (e.g., `"0.3.0"`) |

**Note**: The `_` prefix prevents collision with event data fields.

### Base Fields

Every event includes these base fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ledger_sequence` | number | Yes | Stellar ledger sequence number |
| `tx_hash` | string | Yes | Transaction hash (hex-encoded) |
| `type` | string | Yes | Event type: `"transfer"`, `"mint"`, `"burn"`, `"clawback"`, or `"fee"` |
| `contract_address` | string | No | Smart contract address (if applicable) |
| `asset` | object | No | Asset information (see Asset Object below) |

### Asset Object

When present, the `asset` field has this structure:

```json
{
  "code": "USDC",
  "issuer": "GABC...XYZ"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `code` | string | Yes | Asset code (e.g., `"USDC"`, `"XLM"`) or `"native"` for native XLM |
| `issuer` | string | No | Asset issuer address (omitted for native XLM) |

---

## Event Types

### Transfer Events

Represents a token transfer from one address to another.

**Type**: `"transfer"`

**Fields**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | string | Yes | Source address |
| `to` | string | Yes | Destination address |
| `amount` | string | Yes | Amount in stroops (smallest unit, 1/10^7) |

**Example**:

```json
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "0.3.0",
  "ledger_sequence": 60200000,
  "tx_hash": "abc123...",
  "type": "transfer",
  "from": "GABC...XYZ",
  "to": "GDEF...UVW",
  "amount": "1000000000",
  "asset": {
    "code": "USDC",
    "issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
  }
}
```

**Amount Conversion**:
- `amount` is in stroops (smallest unit)
- To get decimal value: `parseFloat(amount) / 10000000`
- Example: `"1000000000"` stroops = 100.0 USDC

---

### Mint Events

Represents new tokens being created.

**Type**: `"mint"`

**Fields**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `to` | string | Yes | Recipient address |
| `amount` | string | Yes | Amount minted in stroops |

**Example**:

```json
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "0.3.0",
  "ledger_sequence": 60200001,
  "tx_hash": "def456...",
  "type": "mint",
  "to": "GDEF...UVW",
  "amount": "5000000000",
  "asset": {
    "code": "USDC",
    "issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
  }
}
```

---

### Burn Events

Represents tokens being destroyed.

**Type**: `"burn"`

**Fields**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | string | Yes | Address burning tokens |
| `amount` | string | Yes | Amount burned in stroops |

**Example**:

```json
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "0.3.0",
  "ledger_sequence": 60200002,
  "tx_hash": "ghi789...",
  "type": "burn",
  "from": "GDEF...UVW",
  "amount": "2000000000",
  "asset": {
    "code": "USDC",
    "issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
  }
}
```

---

### Clawback Events

Represents tokens being forcibly reclaimed by the issuer.

**Type**: `"clawback"`

**Fields**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | string | Yes | Address losing tokens |
| `amount` | string | Yes | Amount clawed back in stroops |

**Example**:

```json
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "0.3.0",
  "ledger_sequence": 60200003,
  "tx_hash": "jkl012...",
  "type": "clawback",
  "from": "GXYZ...ABC",
  "amount": "500000000",
  "asset": {
    "code": "USDC",
    "issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
  }
}
```

---

### Fee Events

Represents fees collected during token operations.

**Type**: `"fee"`

**Fields**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | string | Yes | Address paying fee |
| `amount` | string | Yes | Fee amount in stroops |

**Example**:

```json
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "0.3.0",
  "ledger_sequence": 60200004,
  "tx_hash": "mno345...",
  "type": "fee",
  "from": "GABC...XYZ",
  "amount": "1000",
  "asset": {
    "code": "native"
  }
}
```

---

## Usage Examples

### Filtering by Schema Version

When querying data that may span multiple schema versions:

```bash
# DuckDB: Filter to only v1 events
duckdb analytics.db -c "
  SELECT * FROM transfers
  WHERE _schema = 'nebu.token_transfer.v1'
"

# jq: Filter to only v1 events
cat events.jsonl | jq 'select(._schema == "nebu.token_transfer.v1")'
```

### Handling Schema Migrations

If you have data from multiple schema versions:

```sql
-- Create separate tables for each version
CREATE TABLE transfers_v1 AS
  SELECT * FROM read_json('/dev/stdin')
  WHERE _schema = 'nebu.token_transfer.v1';

CREATE TABLE transfers_v2 AS
  SELECT * FROM read_json('/dev/stdin')
  WHERE _schema = 'nebu.token_transfer.v2';

-- Create unified view with version-specific transformations
CREATE VIEW transfers_unified AS
  SELECT
    ledger_sequence,
    tx_hash,
    "from",  -- v1 field name
    "to",
    amount
  FROM transfers_v1
UNION ALL
  SELECT
    ledger_sequence,
    tx_hash,
    from_address as "from",  -- v2 field name (hypothetical)
    to_address as "to",
    amount
  FROM transfers_v2;
```

### Checking nebu Version

Track which version of nebu produced your data:

```bash
# Count events by nebu version
duckdb analytics.db -c "
  SELECT _nebu_version, COUNT(*) as event_count
  FROM transfers
  GROUP BY _nebu_version
"
```

---

## Migration Guide

### From Pre-v0.3.0 (No Schema Version)

If you have JSON data from nebu before v0.3.0 (which lacks `_schema` and `_nebu_version` fields):

```sql
-- Add schema version to old data
CREATE TABLE transfers_migrated AS
  SELECT
    'nebu.token_transfer.v1' as _schema,
    'unknown' as _nebu_version,
    *
  FROM transfers_old;
```

### Future: v1 → v2 Migration

When v2 is released (with breaking changes), a migration guide will be provided here.

---

## See Also

- [Token Transfer Processor README](./README.md) - Usage guide
- [nebu Documentation](../../../docs/) - Framework documentation
- [Unix Philosophy Review](../../../docs/UNIX_PHILOSOPHY_REVIEW.md) - Why we version schemas
