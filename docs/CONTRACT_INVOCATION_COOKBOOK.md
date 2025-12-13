# Contract Invocation Cookbook

This cookbook provides practical examples for querying and analyzing Stellar contract invocations using the `contract-invocation` processor.

## Table of Contents

- [Basic Filtering](#basic-filtering)
- [Function Analysis](#function-analysis)
- [Argument Extraction](#argument-extraction)
- [State Change Analysis](#state-change-analysis)
- [Diagnostic Event Analysis](#diagnostic-event-analysis)
- [DuckDB Analytics](#duckdb-analytics)
- [Advanced Patterns](#advanced-patterns)

## Basic Filtering

### Filter by Contract Address

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.contractId == "CDL74RF5BLYR2YBLCCI7F5FB6TPSCLKEJUBSD2RSVWZ4YHF3VMFAIGWA")'
```

### Filter by Function Name

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.functionName == "harvest")'
```

### Filter by Multiple Functions

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.functionName == "mint" or .functionName == "burn")'
```

### Filter by Success Status

```bash
# Only successful invocations
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.successful == true)'

# Only failed invocations
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.successful == false)'
```

### Filter by Transaction Success

```bash
# Invocations in successful transactions
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.meta.inSuccessfulTx == true)'

# Failed transaction but emitted events
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.meta.inSuccessfulTx == false)'
```

### Combine Multiple Filters

```bash
# Specific contract and function, only successful
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.contractId == "CDL74RF5..." and .functionName == "work" and .successful == true)'
```

## Function Analysis

### Count Invocations by Function

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq -r '.functionName' | sort | uniq -c | sort -rn
```

### List Unique Functions

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq -r '.functionName' | sort -u
```

### Group by Contract and Function

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq -r '"\(.contractId[0:8])\t\(.functionName)"' | sort | uniq -c | sort -rn
```

### Success Rate by Function

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq -r '"\(.functionName)\t\(.successful)"' | \
  awk '{count[$1]++; if($2=="true") success[$1]++}
       END {for(f in count) printf "%s\t%.1f%%\n", f, (success[f]/count[f])*100}' | \
  sort -k2 -rn
```

## Argument Extraction

### Extract First Argument

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.arguments | length > 0) | {function: .functionName, arg0: .arguments[0]}'
```

### Extract All Arguments

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '{function: .functionName, args: .arguments}'
```

### Parse JSON Arguments

```bash
# If arguments are JSON strings, parse them
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.functionName == "work") |
      {function: .functionName,
       arg0: (.arguments[0] | fromjson),
       arg1: (.arguments[1] | fromjson)}'
```

### Extract Specific Argument Fields

```bash
# Extract amount from second argument
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.functionName == "transfer") |
      {from: (.arguments[0] | fromjson),
       to: (.arguments[1] | fromjson),
       amount: (.arguments[2] | fromjson)}'
```

## State Change Analysis

### Find Invocations with State Changes

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.stateChanges | length > 0)'
```

### Count State Changes per Invocation

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '{function: .functionName, numChanges: (.stateChanges | length)}' | \
  jq -s 'group_by(.function) | map({function: .[0].function, avgChanges: (map(.numChanges) | add / length)})'
```

### Extract State Change Details

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.stateChanges | length > 0) |
      {function: .functionName,
       changes: [.stateChanges[] | {key: .key, operation: .operation}]}'
```

### Track Specific Key Changes

```bash
# Monitor changes to specific storage key
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '.stateChanges[] | select(.key | contains("Block"))'
```

### Compare Before/After State

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '.stateChanges[] |
      select(.operation == "update") |
      {key: .key, old: (.oldValue | fromjson), new: (.newValue | fromjson)}'
```

## Diagnostic Event Analysis

### Count Events per Invocation

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '{function: .functionName, numEvents: (.diagnosticEvents | length)}'
```

### Filter by Event Type

```bash
# Event types: 0=system, 1=contract, 2=diagnostic
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '.diagnosticEvents[] | select(.eventType == 1)'
```

### Extract Event Topics

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '.diagnosticEvents[] |
      {contract: .contractId[0:12], topics: [.topics[] | fromjson]}'
```

### Find Cross-Contract Calls

```bash
# Look for fn_call events
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '.diagnosticEvents[] |
      select(.topics[0] | contains("fn_call")) |
      {from: .contractId[0:8], function: .topics[2]}'
```

### Analyze Event Data

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '.diagnosticEvents[] |
      select(.data != "null") |
      {contract: .contractId[0:8], data: (.data | fromjson)}'
```

## DuckDB Analytics

### Top Contracts by Invocation Count

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    SELECT contractId, COUNT(*) as invocations
    FROM read_json_auto('/dev/stdin')
    GROUP BY contractId
    ORDER BY invocations DESC
    LIMIT 10
  "
```

### Function Popularity by Contract

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    SELECT
      contractId,
      functionName,
      COUNT(*) as count,
      SUM(CASE WHEN successful THEN 1 ELSE 0 END) as successes,
      ROUND(100.0 * SUM(CASE WHEN successful THEN 1 ELSE 0 END) / COUNT(*), 2) as success_rate
    FROM read_json_auto('/dev/stdin')
    GROUP BY contractId, functionName
    ORDER BY count DESC
    LIMIT 20
  "
```

### Invocations Over Time

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60300000 | \
  duckdb -c "
    SELECT
      DATE_TRUNC('hour', to_timestamp(CAST(meta.closedAtUnix AS BIGINT))) as hour,
      COUNT(*) as invocations,
      COUNT(DISTINCT contractId) as unique_contracts
    FROM read_json_auto('/dev/stdin')
    GROUP BY hour
    ORDER BY hour
  "
```

### Most Active Invoking Accounts

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    SELECT
      invokingAccount,
      COUNT(*) as invocations,
      COUNT(DISTINCT contractId) as unique_contracts,
      COUNT(DISTINCT functionName) as unique_functions
    FROM read_json_auto('/dev/stdin')
    GROUP BY invokingAccount
    ORDER BY invocations DESC
    LIMIT 10
  "
```

### State Change Statistics

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    SELECT
      functionName,
      AVG(list_length(stateChanges)) as avg_state_changes,
      MAX(list_length(stateChanges)) as max_state_changes,
      COUNT(*) as invocations
    FROM read_json_auto('/dev/stdin')
    GROUP BY functionName
    HAVING avg_state_changes > 0
    ORDER BY avg_state_changes DESC
  "
```

### Event Analysis by Function

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    SELECT
      functionName,
      AVG(list_length(diagnosticEvents)) as avg_events,
      COUNT(*) as invocations
    FROM read_json_auto('/dev/stdin')
    GROUP BY functionName
    ORDER BY avg_events DESC
  "
```

### Export to CSV for Analysis

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    COPY (
      SELECT
        meta.ledgerSequence as ledger,
        meta.txHash as tx_hash,
        contractId as contract,
        functionName as function,
        successful,
        meta.inSuccessfulTx as tx_success,
        list_length(arguments) as num_args,
        list_length(diagnosticEvents) as num_events,
        list_length(stateChanges) as num_state_changes
      FROM read_json_auto('/dev/stdin')
    ) TO 'invocations.csv' (HEADER, DELIMITER ',')
  "
```

### Create Persistent Database

```bash
# Stream data into DuckDB database file
contract-invocation --start-ledger 60200000 --end-ledger 60300000 | \
  duckdb invocations.db -c "
    CREATE TABLE IF NOT EXISTS invocations AS
    SELECT * FROM read_json_auto('/dev/stdin')
  "

# Query the database later
duckdb invocations.db -c "
  SELECT functionName, COUNT(*)
  FROM invocations
  GROUP BY functionName
"
```

## Advanced Patterns

### Find Failed Invocations with Diagnostic Details

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.successful == false and (.diagnosticEvents | length > 0)) |
      {
        tx: .meta.txHash[0:8],
        function: .functionName,
        contract: .contractId[0:12],
        errorEvents: [.diagnosticEvents[] | select(.topics[0] | contains("error"))]
      }'
```

### Track Contract Upgrades

```bash
# Look for contract instance changes in state changes
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '.stateChanges[] |
      select(.key == "\"LedgerKeyContractInstance\"") |
      {contract: .contractId, ledger: .meta.ledgerSequence, operation: .operation}'
```

### Extract Account Interactions

```bash
# Extract all accounts from arguments (addresses)
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '.arguments[] | fromjson | select(type == "string" and (startswith("G") or startswith("M")))'
```

### Monitor Specific Value Transfers

```bash
# Find large amounts in arguments (assuming arg contains amount)
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.arguments | length > 2) |
      {
        function: .functionName,
        amount: (try (.arguments[2] | fromjson) catch null)
      } |
      select(.amount != null and (.amount | tonumber? // 0) > 1000000)'
```

### Generate TOID (Transaction Operation ID)

```bash
# Stellar's TOID format: (ledger << 32) | (tx << 12) | op
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq '{
    toid: ((.meta.ledgerSequence * 4294967296) +
           (.meta.transactionIndex * 4096) +
           .meta.operationIndex),
    function: .functionName
  }'
```

### Create Time-Series Data

```bash
contract-invocation --start-ledger 60200000 --end-ledger 60300000 | \
  jq -r '[.meta.closedAtUnix, .contractId, .functionName, (.successful | tostring)] | @csv' > timeseries.csv
```

### Extract Business Data Pattern

```bash
# Custom extraction for specific contract schema
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.contractId == "CASJKXV..." and .functionName == "sink_carbon") |
      {
        ledger: .meta.ledgerSequence,
        timestamp: .meta.closedAtUnix,
        funder: (.arguments[0] | fromjson),
        recipient: (.arguments[1] | fromjson),
        amount: (.arguments[2] | fromjson),
        project_id: (.arguments[3] | fromjson),
        memo: (try (.arguments[4] | fromjson) catch "")
      }'
```

## Performance Tips

### Use Head for Large Ranges

```bash
# Process only first N invocations
contract-invocation --start-ledger 60200000 --end-ledger 60300000 | head -1000 | jq '...'
```

### Save Intermediate Results

```bash
# Save raw data, analyze multiple times
contract-invocation --start-ledger 60200000 --end-ledger 60200100 > invocations.jsonl

# Run different analyses
cat invocations.jsonl | jq 'select(.functionName == "work")'
cat invocations.jsonl | duckdb -c "SELECT COUNT(*) FROM read_json_auto('/dev/stdin')"
```

### Use Quiet Mode

```bash
# Suppress progress output for cleaner piping
contract-invocation -q --start-ledger 60200000 --end-ledger 60200100 | jq '...'
```

## Common Use Cases

### Audit Trail

```bash
# Extract complete audit trail for specific contract
contract-invocation --start-ledger 60200000 --end-ledger 60300000 | \
  jq 'select(.contractId == "CASJKXV...") |
      {
        timestamp: .meta.closedAtUnix,
        tx: .meta.txHash,
        function: .functionName,
        invoker: .invokingAccount,
        successful: .successful,
        state_changes: (.stateChanges | length)
      }' > audit.jsonl
```

### Cost Analysis

```bash
# Analyze which functions cause most state changes (proxy for cost)
contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  duckdb -c "
    SELECT
      functionName,
      AVG(list_length(stateChanges)) as avg_storage_ops,
      AVG(list_length(diagnosticEvents)) as avg_events,
      COUNT(*) as invocations
    FROM read_json_auto('/dev/stdin')
    GROUP BY functionName
    ORDER BY avg_storage_ops DESC
  "
```

### User Activity

```bash
# Track user's contract interactions
USER="GCOPVDPFET6CUT7H3U54QD4QGJCQ3PMTHY5HCXRHHC4YKCK3J7TVD5K2"

contract-invocation --start-ledger 60200000 --end-ledger 60200100 | \
  jq --arg user "$USER" 'select(.invokingAccount == $user) |
      {
        time: .meta.closedAtUnix,
        contract: .contractId[0:12],
        function: .functionName,
        success: .successful
      }'
```
