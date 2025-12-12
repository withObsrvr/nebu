# Cycle 5: Processor Ecosystem Expansion

**Appetite**: 1 week (5 days)
**Status**: Planning

## Problem

We have excellent processor infrastructure (CLI wrappers, install command, full composability), but only 2 example processors:
- **Origin**: token-transfer (the only origin processor)
- **Transform**: usdc-filter (single asset filter)
- **Sink**: json-file-sink (basic file writing)

Users need:
1. More **transform** examples showing different patterns (filtering by amount, time windows, deduplication)
2. A **DuckDB sink** for proper database persistence (not just piping to duckdb CLI)
3. **Integration tests** to ensure pipeline reliability
4. **Developer guide** showing how to build custom processors

Without these, users must write processors from scratch with no examples beyond basic filtering.

## Appetite

**5 days** - Build ecosystem examples, not production features. Each processor should be ~50-100 LOC showing a pattern.

## Solution

### 1. Transform Processors (2 days)

Build 3 transform processors demonstrating key patterns:

**`amount-filter`** - Filter by numeric ranges
```bash
# Only transfers over 1M stroops
token-transfer | amount-filter --min 1000000

# Whales: over 100M stroops
token-transfer | amount-filter --min 100000000 --asset USDC
```

**`time-window`** - Filter by time range
```bash
# Only events from last hour
token-transfer | time-window --last 1h

# Specific date range
token-transfer | time-window --start 2024-01-01 --end 2024-01-31
```

**`dedup`** - Remove duplicate events
```bash
# Deduplicate by tx_hash
token-transfer | dedup --key tx_hash

# Deduplicate by multiple fields
token-transfer | dedup --key tx_hash,ledger_sequence
```

### 2. DuckDB Sink (1 day)

Build a proper DuckDB sink that writes to database files:

```bash
# Write to DuckDB database
token-transfer | usdc-filter | duckdb-sink --db transfers.duckdb --table usdc_transfers

# Then query
duckdb transfers.duckdb "SELECT COUNT(*) FROM usdc_transfers"
```

Features:
- Auto-create table from first event schema
- Append mode (add to existing table)
- Replace mode (drop and recreate)
- Batch inserts for performance

### 3. Integration Tests (1 day)

Add `tests/integration` with real pipeline tests:

```bash
# Test full pipeline
tests/integration/test_pipeline.sh
tests/integration/test_transforms.sh
tests/integration/test_sinks.sh
```

Tests verify:
- Origin → Transform → Sink pipelines work end-to-end
- Quiet mode produces no stderr output
- File/stdin input modes work correctly
- Error handling is proper

### 4. Developer Guide (1 day)

Create `docs/BUILDING_PROCESSORS.md` with:
- Step-by-step guide for each processor type
- Common patterns (filtering, aggregation, enrichment)
- Testing strategies
- Packaging and distribution

## Scope Line

```
COULD HAVE ─────────────
  - CLI auto-completion
  - Processor marketplace/registry
  - Performance benchmarks

NICE TO HAVE ───────────
  - More origin processors (nft-transfers, liquidity-pools)
  - Webhook sink processor
  - Aggregation transforms (group-by, window functions)

MUST HAVE ══════════════
  - 3 transform processors (amount-filter, time-window, dedup)
  - DuckDB sink processor
  - Integration tests
  - Developer guide
```

## Rabbit Holes

**DON'T:**
- Build a full ORM or schema management system
- Add complex configuration systems
- Build a processor registry/marketplace
- Add distributed processing features
- Implement complex time-series windowing

**DO:**
- Keep processors simple (50-100 LOC each)
- Use the CLI wrappers we already built
- Focus on demonstrating patterns
- Write tests that actually run real pipelines

## No-Gos

- No web UI or management console
- No processor versioning system
- No dependency management beyond go.mod
- No cloud deployment features
- No authentication/authorization

## Done Looks Like

### Transform Processors Built

```bash
# Amount filter
$ token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  amount-filter --min 10000000 | \
  wc -l
42  # Only large transfers

# Time window
$ token-transfer --start-ledger 60200000 --end-ledger 60300000 | \
  time-window --last 24h | \
  wc -l
1847  # Events from last 24 hours

# Dedup
$ cat events-with-dupes.jsonl | dedup --key tx_hash | wc -l
1000  # Removed duplicates
```

### DuckDB Sink Working

```bash
# Write to database
$ token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  usdc-filter | \
  duckdb-sink --db transfers.duckdb --table usdc_transfers

Writing to transfers.duckdb...
Created table usdc_transfers
Inserted 51 events

# Query the database
$ duckdb transfers.duckdb -c "
  SELECT
    asset.code as asset,
    COUNT(*) as transfers,
    SUM(CAST(amount AS BIGINT)) as volume
  FROM usdc_transfers
  GROUP BY asset.code
"
┌───────┬───────────┬─────────────┐
│ asset │ transfers │   volume    │
├───────┼───────────┼─────────────┤
│ USDC  │    51     │ 532482184   │
└───────┴───────────┴─────────────┘
```

### Integration Tests Pass

```bash
$ make test-integration
Running integration tests...
✓ test_origin_to_stdout
✓ test_origin_to_transform_to_sink
✓ test_stdin_input
✓ test_file_input
✓ test_quiet_mode
✓ test_error_handling
All tests passed!
```

### Developer Guide Complete

```bash
$ cat docs/BUILDING_PROCESSORS.md

# Building Custom Processors

## Transform Processor in 5 Minutes

1. Create directory: examples/processors/my-filter/cmd
2. Copy template...
3. Implement filterFunc...
4. Build and test...

[Detailed guide with examples for Origin, Transform, and Sink]
```

## Use Cases Enabled

### 1. High-Value Transfer Monitoring
```bash
# Find whale movements (>100M USDC)
$ token-transfer --start-ledger 60200000 --end-ledger 60300000 | \
  usdc-filter | \
  amount-filter --min 100000000 | \
  duckdb-sink --db whales.duckdb --table movements
```

### 2. Time-Based Analysis
```bash
# Compare trading volume by hour
$ token-transfer --start-ledger 60200000 --end-ledger 60300000 | \
  time-window --last 24h | \
  duckdb-sink --db recent.duckdb --table hourly

$ duckdb recent.duckdb -c "
  SELECT
    DATE_TRUNC('hour', from_unixtime(ledger_sequence * 5)) as hour,
    COUNT(*) as transfers
  FROM hourly
  GROUP BY hour
  ORDER BY hour
"
```

### 3. Clean Data Pipelines
```bash
# Remove duplicates before loading to database
$ cat raw-events.jsonl | \
  dedup --key tx_hash | \
  duckdb-sink --db clean.duckdb --table events
```

### 4. Build Custom Processors
```bash
# Follow the guide to build your own
$ cat docs/BUILDING_PROCESSORS.md
# 30 minutes later...
$ my-custom-filter --help
```

## Implementation Order

**Day 1**: amount-filter transform
- Parse amount from event
- Filter by min/max range
- Optional asset filtering
- Tests with token-transfer

**Day 2**: time-window transform + dedup transform
- time-window: Parse ledger_sequence, filter by time
- dedup: Track seen keys, filter duplicates
- Tests for both

**Day 3**: duckdb-sink
- Create DuckDB connection
- Auto-create table from JSON schema
- Batch insert events
- Handle errors gracefully
- Tests with real database

**Day 4**: Integration tests
- Test fixtures (sample ledgers)
- Pipeline tests (origin → transform → sink)
- Error case tests
- Quiet mode verification

**Day 5**: Developer guide + polish
- Write comprehensive guide
- Add examples to each processor
- Update main README
- Test all documentation examples

## Success Metrics

After Cycle 5:
- ✅ 3 new transform processors with different patterns
- ✅ DuckDB sink for database persistence
- ✅ Integration test suite (>10 tests)
- ✅ Developer guide with step-by-step instructions
- ✅ All examples in docs actually work

Users can now:
- Build custom processors in <30 minutes
- Use practical transforms out of the box
- Persist data to DuckDB databases
- Trust that pipelines work (integration tests)

## Architecture Impact

**Before Cycle 5**:
```
Infrastructure: ✅ (CLI wrappers, install, composability)
Processors: ⚠️ (only 2-3 examples)
Testing: ❌ (manual testing only)
Documentation: ⚠️ (README only)
```

**After Cycle 5**:
```
Infrastructure: ✅ (CLI wrappers, install, composability)
Processors: ✅ (6+ processors showing all patterns)
Testing: ✅ (automated integration tests)
Documentation: ✅ (developer guide + examples)
```

**Ready for users to build their own processors!**

## Risk Assessment

**Low Risk**:
- All infrastructure already works
- Just building examples using existing wrappers
- Each processor is independent

**Potential Issues**:
- DuckDB library might have quirks - timebox to 1 day
- Integration tests might be flaky - keep them simple
- Documentation might grow too large - stay focused

**Mitigation**:
- Start with simplest processor first (amount-filter)
- Write integration tests as we build processors
- Keep guide to 1 page per processor type

## Ready to Start?

This cycle builds on the solid foundation from Cycles 1-4. We're not changing infrastructure, just adding examples that demonstrate the power of the system.

**Ship goal**: Users can build custom processors in 30 minutes by following the guide.
