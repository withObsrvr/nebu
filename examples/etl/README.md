# Shell ETL Examples

These examples show how to use nebu as the extract/streaming part of a simple shell-based ETL workflow.

Nebu is not an ETL orchestrator, but it works well inside:

- shell scripts
- cron jobs
- CI jobs
- Makefiles / justfiles
- Airflow / Dagster / Prefect tasks

## Example: USDC transfers to DuckDB

Script:

- [`usdc_to_duckdb.sh`](./usdc_to_duckdb.sh)

What it does:

1. **Extract** token transfer events from a ledger range
2. **Transform** by filtering for USDC transfers
3. **Load** into:
   - a JSONL archive file
   - a DuckDB database table

## Requirements

- `nebu` installed
- `token-transfer` installed (`nebu install token-transfer`)
- `jq`
- `duckdb`

## Run

```bash
./examples/etl/usdc_to_duckdb.sh 60200000 60200100
```

Or with custom output paths:

```bash
OUT_DIR=./out \
DB_PATH=./out/nebu.duckdb \
./examples/etl/usdc_to_duckdb.sh 60200000 60200100
```

## Output

The script writes:

- `OUT_DIR/usdc-transfers.jsonl`
- `DB_PATH` with a `usdc_transfers` table

Then it prints a small summary query from DuckDB.

## Notes

This is intentionally simple and shell-native. For bigger workflows, use this script as the task body inside your scheduler/orchestrator.
