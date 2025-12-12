# Shape: Schema Versioning in JSON Output

**Status**: Ready for Betting
**Appetite**: 1 day
**Created**: 2025-12-12

---

## Problem

nebu outputs JSON events with no schema version identifier. When the output format changes (new fields, renamed fields, structure changes), downstream consumers break silently.

**Current situation:**
```json
{"type":"transfer","ledger_sequence":60200000,"tx_hash":"abc...","from":"GA...","to":"GB...","amount":"1000000"}
```

**What breaks:**
- Users pipe to DuckDB, create tables based on current schema
- nebu updates and adds/removes fields in a new version
- Old DuckDB queries break or produce wrong results
- No way to detect schema mismatch

**Real example:**
```bash
# User creates a DuckDB table from nebu v0.3.0 output
nebu run origin token-transfer --start 60200000 --end 60200100 | \
  duckdb analytics.db -c "CREATE TABLE transfers AS SELECT * FROM read_json('/dev/stdin')"

# Upgrade to nebu v0.4.0 which renames "from" → "from_address"
# Old DuckDB queries using "from" field now fail
duckdb analytics.db -c "SELECT \"from\", COUNT(*) FROM transfers GROUP BY \"from\""
# Error: column "from" not found
```

---

## Appetite

**1 day** - This is a small, focused change to add metadata fields to JSON output.

---

## Solution

### Fat-Marker Sketch

**Add version metadata to every JSON event:**

```json
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "0.3.0",
  "type": "transfer",
  "ledger_sequence": 60200000,
  "tx_hash": "abc...",
  "from": "GA...",
  "to": "GB...",
  "amount": "1000000",
  "asset": {"code": "USDC", "issuer": "GA..."}
}
```

**Schema versioning strategy:**

1. **Schema identifier** (`_schema`) - Format: `nebu.<processor>.<version>`
   - `nebu.token_transfer.v1` - Original format
   - `nebu.token_transfer.v2` - After breaking changes (field renames, removals)
   - Non-breaking changes (new fields) don't bump version

2. **nebu version** (`_nebu_version`) - Which nebu CLI version produced this
   - Helps debug: "This output was from nebu 0.3.0"
   - Useful when schema is same but behavior changed

**Breaking vs non-breaking changes:**

```
Breaking (bumps schema version):
- Rename field: "from" → "from_address"
- Remove field: delete "contract_address"
- Change type: "amount" string → number
- Change structure: flatten nested objects

Non-breaking (keeps schema version):
- Add new field: add "timestamp"
- Add new event type: add "clawback" type
```

**Implementation approach:**

```go
// pkg/processor/cli/origin.go
func simplifyTokenTransferEvent(ev *token_transfer.TokenTransferEvent) map[string]interface{} {
    event := map[string]interface{}{
        "_schema":        "nebu.token_transfer.v1",
        "_nebu_version":  version.Version,  // From build ldflags

        // Actual event data
        "ledger_sequence": meta.LedgerSequence,
        "tx_hash":         meta.TxHash,
        "type":            "transfer",
        // ... rest of fields
    }
    return event
}
```

**Documentation:**

Create `examples/processors/token-transfer/SCHEMA.md`:
```markdown
# Token Transfer JSON Schema

## Current Version: v1

Schema identifier: `nebu.token_transfer.v1`

### Event Structure

All events include:
- `_schema` (string): Schema version identifier
- `_nebu_version` (string): nebu CLI version that produced this event
- `type` (string): Event type: "transfer" | "mint" | "burn" | "clawback" | "fee"
- `ledger_sequence` (number): Ledger number
- `tx_hash` (string): Transaction hash

### Type-Specific Fields

#### Transfer Events
- `from` (string): Source address
- `to` (string): Destination address
- `amount` (string): Amount in stroops
- `asset` (object): Asset information
  - `code` (string): Asset code or "native"
  - `issuer` (string, optional): Asset issuer address

...
```

---

## Scope Line

### MUST HAVE ═══════════════════
- ✅ Add `_schema` field to all JSON events
- ✅ Add `_nebu_version` field to all JSON events
- ✅ Use semantic versioning for schema: `nebu.<processor>.v<N>`
- ✅ Document current schema in `examples/processors/token-transfer/SCHEMA.md`
- ✅ Update CLI to inject version from build ldflags
- ✅ Start at `v1` for all existing outputs

### NICE TO HAVE ───────────────
- ✅ Add schema changelog to SCHEMA.md
- ✅ Include schema version in processor registry.yaml
- ⚠️ Document breaking vs non-breaking change policy

### COULD HAVE ─────────────────
- ❌ Schema validation in nebu CLI (validate output matches declared schema)
- ❌ Schema migration tools (convert v1 → v2)
- ❌ Machine-readable schema (JSON Schema, Protobuf descriptors)
- ❌ Schema compatibility testing

---

## Rabbit Holes

**Don't do these:**

1. **❌ Don't build schema validation** - Just emit the version identifier. Don't validate output against schema definitions.

2. **❌ Don't create schema migration tools** - Users can handle migrations themselves with SQL/jq. This is a future cycle if needed.

3. **❌ Don't generate JSON Schema definitions** - The protobuf files are the source of truth. Human-readable SCHEMA.md is sufficient.

4. **❌ Don't version every processor separately** - All token-transfer output is `v1` to start. Only version when we have multiple processors with different schemas.

5. **❌ Don't add runtime schema selection** - No `--schema-version v1` flag. You get the current version. Old versions are for reading old data, not generating.

---

## No-Gos

**Explicitly out of scope:**

- **JSON Schema validation** - Not validating output against schema definitions
- **Schema migration tooling** - No `nebu migrate v1-to-v2` command
- **Multiple output schemas** - Can't request old schema versions via CLI flag
- **Protobuf schema exports** - Not exporting .proto files as JSON Schema
- **Compatibility testing** - Not auto-testing that v2 is backward compatible

---

## Done Looks Like

### Success Example 1: Versioned Output

```bash
nebu run origin token-transfer --start-ledger 60200000 --end-ledger 60200001 | jq

# Output:
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "0.3.0",
  "type": "transfer",
  "ledger_sequence": 60200000,
  "tx_hash": "abc123...",
  "from": "GABC...",
  "to": "GXYZ...",
  "amount": "1000000",
  "asset": {
    "code": "USDC",
    "issuer": "GABC..."
  }
}
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "0.3.0",
  "type": "mint",
  "ledger_sequence": 60200001,
  "tx_hash": "def456...",
  "to": "GXYZ...",
  "amount": "5000000",
  "asset": {
    "code": "native"
  }
}
```

### Success Example 2: DuckDB Schema Detection

```bash
# Create table with schema-aware query
nebu run origin token-transfer --start 60200000 --end 60200100 | \
  duckdb analytics.db -c "
    CREATE TABLE transfers AS
    SELECT
      _schema,
      _nebu_version,
      ledger_sequence,
      type,
      \"from\",
      \"to\",
      amount,
      json_extract_string(asset, '$.code') as asset_code
    FROM read_json('/dev/stdin')
    WHERE _schema = 'nebu.token_transfer.v1'
  "

# Later: Filter by schema version
duckdb analytics.db -c "
  SELECT _schema, COUNT(*) FROM transfers GROUP BY _schema
"
# Output:
# _schema                    | count
# ---------------------------+-------
# nebu.token_transfer.v1     | 432
```

### Success Example 3: Schema Documentation

```bash
cat examples/processors/token-transfer/SCHEMA.md

# Token Transfer JSON Schema

## Current Version: v1

Schema identifier: `nebu.token_transfer.v1`

First released: nebu v0.3.0

### Changelog

**v1** (2025-12-12)
- Initial schema version
- Supports: transfer, mint, burn, clawback, fee events
- Fields: ledger_sequence, tx_hash, type, from, to, amount, asset

### Event Structure
...
```

### Success Example 4: Version in Build

```bash
# Check nebu version
nebu --version
# Output: nebu version 0.3.0

# Events include matching version
nebu run origin token-transfer --start 60200000 --end 60200000 | jq '._nebu_version'
# Output: "0.3.0"
```

### Success Example 5: Future Breaking Change

```bash
# In nebu v0.4.0, we rename "from" → "from_address"
# New events have v2 schema:

{
  "_schema": "nebu.token_transfer.v2",
  "_nebu_version": "0.4.0",
  "type": "transfer",
  "ledger_sequence": 60300000,
  "from_address": "GABC...",  # <-- RENAMED
  "to_address": "GXYZ...",    # <-- RENAMED
  "amount": "1000000",
  "asset": {"code": "USDC", "issuer": "GABC..."}
}

# Users can filter by schema version in DuckDB:
SELECT * FROM transfers WHERE _schema = 'nebu.token_transfer.v1'  -- Old data
SELECT * FROM transfers WHERE _schema = 'nebu.token_transfer.v2'  -- New data
```

---

## Implementation Checklist

When implementing this shape, you should:

- [ ] Add version constants to `pkg/version/version.go`
- [ ] Inject version via ldflags in Makefile: `-ldflags "-X pkg/version.Version=$(VERSION)"`
- [ ] Update `simplifyTokenTransferEvent()` to add `_schema` and `_nebu_version` fields
- [ ] Create `examples/processors/token-transfer/SCHEMA.md` documentation
- [ ] Document schema versioning policy in SCHEMA.md (breaking vs non-breaking)
- [ ] Update `registry.yaml` to include schema version for token-transfer processor
- [ ] Update README with schema versioning explanation
- [ ] Add example DuckDB queries that filter by schema version
- [ ] Test that version is correctly injected and appears in output

---

## Notes

- This is a **1-day appetite** - very focused change
- Schema version starts at `v1` for all existing output
- Only bump to `v2` when making breaking changes (field renames, removals, type changes)
- Adding new fields is non-breaking and doesn't require version bump
- The `_` prefix on `_schema` and `_nebu_version` prevents collision with event fields
- This solves the "silent breakage" problem identified in Unix Philosophy review
