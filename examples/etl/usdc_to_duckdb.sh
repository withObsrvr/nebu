#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 START_LEDGER END_LEDGER" >&2
  echo "example: $0 60200000 60200100" >&2
  exit 1
fi

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command not found: $1" >&2
    exit 1
  fi
}

need token-transfer
need jq
need duckdb

START_LEDGER="$1"
END_LEDGER="$2"
OUT_DIR="${OUT_DIR:-./out}"
DB_PATH="${DB_PATH:-$OUT_DIR/nebu.duckdb}"
JSONL_PATH="${JSONL_PATH:-$OUT_DIR/usdc-transfers.jsonl}"
TABLE_NAME="${TABLE_NAME:-usdc_transfers}"

mkdir -p "$OUT_DIR"
rm -f "$JSONL_PATH"

echo "Extracting USDC transfers from ledgers ${START_LEDGER}..${END_LEDGER}"
echo "JSONL output: $JSONL_PATH"
echo "DuckDB:       $DB_PATH"
echo

# Extract + transform to a reusable JSONL archive.
token-transfer --start-ledger "$START_LEDGER" --end-ledger "$END_LEDGER" | \
  jq -c 'select(.transfer != null and .transfer.assetCode == "USDC")' | \
  tee "$JSONL_PATH" > /dev/null

# Load into DuckDB.
duckdb "$DB_PATH" <<SQL
CREATE TABLE IF NOT EXISTS ${TABLE_NAME} AS
SELECT *
FROM read_json('${JSONL_PATH}')
WHERE 1 = 0;

INSERT INTO ${TABLE_NAME}
SELECT *
FROM read_json('${JSONL_PATH}');
SQL

echo "Load complete."
echo

echo "Summary:"
duckdb "$DB_PATH" <<SQL
SELECT
  COUNT(*) AS rows_loaded,
  MIN(meta.ledgerSequence) AS first_ledger,
  MAX(meta.ledgerSequence) AS last_ledger
FROM ${TABLE_NAME};
SQL

echo
echo "Top senders:"
duckdb "$DB_PATH" <<SQL
SELECT
  json_extract_string(transfer, '$.from') AS sender,
  COUNT(*) AS transfers
FROM ${TABLE_NAME}
GROUP BY 1
ORDER BY 2 DESC
LIMIT 10;
SQL
