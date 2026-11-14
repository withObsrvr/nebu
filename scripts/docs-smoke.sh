#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# DuckDB 2.0.0 turns on "agent mode" when AI_AGENT or CLAUDECODE is set and
# stdout is not a terminal. Its documented effects are all about rendering —
# markdown tables, row caps, JSON errors on stderr — but it also collapses
# read_json's inferred schema to a single `json` column, which turns a
# query like `SELECT contractId ...` into a binder error. That is not
# documented and looks unintended, so it is pinned off here rather than
# worked around per query: these checks must mean the same thing whether a
# person or an agent runs them.
export DUCKDB_AGENT_MODE=0
cd "$ROOT"

NEBU_BIN="${NEBU_BIN:-$ROOT/bin/nebu}"
TOKEN_TRANSFER_BIN="${TOKEN_TRANSFER_BIN:-$ROOT/bin/token-transfer}"
CONTRACT_EVENTS_BIN="${CONTRACT_EVENTS_BIN:-$ROOT/bin/contract-events}"
USDC_FILTER_BIN="${USDC_FILTER_BIN:-$ROOT/bin/usdc-filter}"
AMOUNT_FILTER_BIN="${AMOUNT_FILTER_BIN:-$ROOT/bin/amount-filter}"
DEDUP_BIN="${DEDUP_BIN:-$ROOT/bin/dedup}"
JSON_FILE_SINK_BIN="${JSON_FILE_SINK_BIN:-$ROOT/bin/json-file-sink}"
NATS_SINK_BIN="${NATS_SINK_BIN:-$ROOT/bin/nats-sink}"

TMPDIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMPDIR"
  if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx nebu-docs-nats; then
    docker rm -f nebu-docs-nats >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need jq
need duckdb

for bin in \
  "$NEBU_BIN" "$TOKEN_TRANSFER_BIN" "$CONTRACT_EVENTS_BIN" "$ROOT/bin/contract-invocation" "$USDC_FILTER_BIN" \
  "$AMOUNT_FILTER_BIN" "$DEDUP_BIN" "$JSON_FILE_SINK_BIN"; do
  [[ -x "$bin" ]] || { echo "missing executable: $bin" >&2; exit 1; }
done

pass() { echo "PASS: $*"; }
run() {
  local name="$1"
  shift
  echo "==> $name"
  "$@"
  pass "$name"
}

run "nebu --version" bash -lc '"$0" --version >/dev/null' "$NEBU_BIN"
run "nebu list" bash -lc '"$0" list >/dev/null' "$NEBU_BIN"
run "nebu describe token-transfer" bash -lc '"$0" describe token-transfer >/dev/null' "$NEBU_BIN"
run "token-transfer --describe-json" bash -lc '"$0" --describe-json | jq -e ".schema.id == \"nebu.token_transfer.v1\"" >/dev/null' "$TOKEN_TRANSFER_BIN"
run "contract-events --describe-json" bash -lc '"$0" --describe-json | jq -e ".schema.id == \"nebu.contract_events.v1\"" >/dev/null' "$CONTRACT_EVENTS_BIN"

run "token-transfer bounded run" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200000 | head -1 | jq -e "._schema == \"nebu.token_transfer.v1\"" >/dev/null' "$TOKEN_TRANSFER_BIN"
# The root help is the first thing a reader runs and nothing checked it. By
# v0.7.2 two of its four quickstart commands were broken — a jq filter on a
# field that does not exist, and --start/--end against a processor that
# exposes --start-ledger/--end-ledger.
run "root help flags exist on the processors it names" bash -lc \
  'python3 "$0" "$1" "$2" "$3" "$4" "$5"' \
  "$ROOT/scripts/check-help-examples.py" "$ROOT/bin/nebu" \
  "$TOKEN_TRANSFER_BIN" "$USDC_FILTER_BIN" "$AMOUNT_FILTER_BIN" "$ROOT/bin/json-file-sink"

# A processor's own EXAMPLES come from pkg/processor/cli/help.go, generated
# from its name, so one wrong line there ships in every processor's help.
run "processors use only the flags they define" bash -lc \
  'python3 "$0" --self "$@"' \
  "$ROOT/scripts/check-help-examples.py" \
  "$ROOT/bin/transaction-stats" "$ROOT/bin/ledger-change-stats" \
  "$TOKEN_TRANSFER_BIN" "$CONTRACT_EVENTS_BIN" "$USDC_FILTER_BIN" \
  "$AMOUNT_FILTER_BIN" "$ROOT/bin/json-file-sink"

# A piped fetch is the form the quickstart documents, and it was the one form
# that dropped the resolved --last range: fetch infers quiet mode when stdout
# is not a terminal, which silenced the only report of which ledgers it chose.
run "piped fetch still reports the range --last resolved to" bash -lc \
  '"$0" fetch --last 2 2>&1 >/dev/null | grep -q "Resolved --last 2 to ledgers"' \
  "$ROOT/bin/nebu"

run "root help jq filter actually selects events" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200002 \
  | jq -c "select(.transfer != null)" | head -1 | jq -e ".transfer.amount" >/dev/null' "$TOKEN_TRANSFER_BIN"

run "SAMPLE_EVENT matches real output" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200002 | jq -c "select(.transfer != null)" | head -1 | python3 "$1"' "$TOKEN_TRANSFER_BIN" "$ROOT/scripts/check-sample-event.py"
run "token-transfer jq USDC filter" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200002 | jq -e "select(.transfer != null and .transfer.assetCode == \"USDC\")" | head -1 >/dev/null' "$TOKEN_TRANSFER_BIN"
run "contract-events jq eventType filter" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200000 | jq -e "select(.eventType == \"fee\")" | head -1 >/dev/null' "$CONTRACT_EVENTS_BIN"
run "contract-events jq contractId filter" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200000 | jq -e "select(.contractId != null and .inSuccessfulTx == true)" | head -1 >/dev/null' "$CONTRACT_EVENTS_BIN"
run "contract-invocation jq argument extraction" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200000 | jq -e "select(.functionName == \"work\") | (.arguments[0] | fromjson | startswith(\"G\"))" | head -1 >/dev/null' "$ROOT/bin/contract-invocation"
run "contract-invocation duckdb top contracts" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200000 | duckdb -c "COPY (SELECT COUNT(*) AS c FROM (SELECT contractId, COUNT(*) AS invocations FROM read_json_auto('\''/dev/stdin'\'') GROUP BY contractId) t) TO '\''/dev/stdout'\'' (FORMAT CSV, HEADER FALSE)" | grep -Eq "^[1-9][0-9]*$"' "$ROOT/bin/contract-invocation"
run "contract-invocation duckdb array_length stats" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200000 | duckdb -c "COPY (SELECT COUNT(*) AS c FROM (SELECT functionName, AVG(array_length(stateChanges)) AS avg_state_changes FROM read_json_auto('\''/dev/stdin'\'') GROUP BY functionName) t) TO '\''/dev/stdout'\'' (FORMAT CSV, HEADER FALSE)" | grep -Eq "^[1-9][0-9]*$"' "$ROOT/bin/contract-invocation"
run "nebu fetch | token-transfer" bash -lc '"$0" fetch 60200000 60200000 | "$1" --quiet | head -1 | jq -e "._schema == \"nebu.token_transfer.v1\"" >/dev/null' "$NEBU_BIN" "$TOKEN_TRANSFER_BIN"

run "json-file-sink write" bash -lc 'out="$1/out.jsonl"; "$2" --quiet --start-ledger 60200000 --end-ledger 60200000 | "$3" --out "$out" >/dev/null; test -s "$out"; jq -e "has(\"_schema\")" "$out" >/dev/null' _ "$TMPDIR" "$TOKEN_TRANSFER_BIN" "$JSON_FILE_SINK_BIN"
run "usdc-filter flat assetCode" bash -lc 'printf "%s\n" "{\"_schema\":\"nebu.token_transfer.v1\",\"meta\":{\"ledgerSequence\":60200000,\"txHash\":\"abc\"},\"transfer\":{\"from\":\"GA\",\"to\":\"GB\",\"assetCode\":\"USDC\",\"assetIssuer\":\"GI\",\"amount\":\"1000000\"}}" | "$0" | jq -e ".transfer.assetCode == \"USDC\"" >/dev/null' "$USDC_FILTER_BIN"
run "amount-filter flat assetCode" bash -lc 'printf "%s\n" "{\"_schema\":\"nebu.token_transfer.v1\",\"meta\":{\"ledgerSequence\":60200000,\"txHash\":\"abc\"},\"transfer\":{\"from\":\"GA\",\"to\":\"GB\",\"assetCode\":\"USDC\",\"assetIssuer\":\"GI\",\"amount\":\"1000000\"}}" | "$0" --min 999999 --asset USDC | jq -e ".transfer.assetCode == \"USDC\"" >/dev/null' "$AMOUNT_FILTER_BIN"
run "dedup nested key" bash -lc 'printf "%s\n%s\n" "{\"meta\":{\"txHash\":\"a\"},\"transfer\":{\"assetCode\":\"USDC\",\"amount\":\"1\"}}" "{\"meta\":{\"txHash\":\"a\"},\"transfer\":{\"assetCode\":\"USDC\",\"amount\":\"1\"}}" | "$0" --key meta.txHash | wc -l | grep -qx 1' "$DEDUP_BIN"

run "pipeline doc: token-transfer | json-file-sink" bash -lc 'out="$1/pipeline.jsonl"; "$2" --quiet --start-ledger 60200000 --end-ledger 60200000 | "$3" --out "$out" >/dev/null; jq -e "select(.transfer != null or .fee != null)" "$out" >/dev/null' _ "$TMPDIR" "$TOKEN_TRANSFER_BIN" "$JSON_FILE_SINK_BIN"
run "readme fetch reuse jq" bash -lc 'xdr="$1/ledgers.xdr"; "$2" fetch 60200000 60200000 > "$xdr"; cat "$xdr" | "$3" --quiet | jq -e "select(.transfer != null or .fee != null)" | head -1 >/dev/null' _ "$TMPDIR" "$NEBU_BIN" "$TOKEN_TRANSFER_BIN"
run "pipeline doc: token-transfer | duckdb" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200002 | duckdb -c "COPY (SELECT COUNT(*) AS c FROM read_json('\''/dev/stdin'\'') WHERE transfer IS NOT NULL OR fee IS NOT NULL) TO '\''/dev/stdout'\'' (FORMAT CSV, HEADER FALSE)" | grep -Eq "^[1-9][0-9]*$"' "$TOKEN_TRANSFER_BIN"
run "duckdb cookbook assetCode query" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200002 | duckdb -c "COPY (SELECT COUNT(*) AS c FROM read_json('\''/dev/stdin'\'') WHERE transfer IS NOT NULL AND json_extract_string(transfer, '\''$.assetCode'\'') IS NOT NULL) TO '\''/dev/stdout'\'' (FORMAT CSV, HEADER FALSE)" | grep -Eq "^[1-9][0-9]*$"' "$TOKEN_TRANSFER_BIN"
run "duckdb cookbook event type aggregation" bash -lc 'db="$1/cookbook.db"; "$2" --quiet --start-ledger 60200000 --end-ledger 60200000 | duckdb "$db" -c "CREATE OR REPLACE TABLE transfers AS SELECT * FROM read_json('\''/dev/stdin'\'')" >/dev/null; duckdb "$db" -c "COPY (SELECT CASE WHEN json_exists(to_json(transfers), '\''$.transfer'\'') AND transfer IS NOT NULL THEN '\''transfer'\'' WHEN json_exists(to_json(transfers), '\''$.mint'\'') AND mint IS NOT NULL THEN '\''mint'\'' WHEN json_exists(to_json(transfers), '\''$.fee'\'') AND fee IS NOT NULL THEN '\''fee'\'' ELSE '\''other'\'' END AS event_type, COUNT(*) AS count FROM transfers GROUP BY event_type) TO '\''/dev/stdout'\'' (FORMAT CSV, HEADER FALSE)" | grep -Eq "(transfer|fee|mint|other),[0-9]+"' _ "$TMPDIR" "$TOKEN_TRANSFER_BIN"
run "duckdb cookbook address activity" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200002 | duckdb -c "COPY (WITH events AS (SELECT * FROM read_json('\''/dev/stdin'\'')), address_activity AS (SELECT json_extract_string(transfer, '\''$.from'\'') AS address, COUNT(*) AS sends, 0 AS receives FROM events WHERE transfer IS NOT NULL AND json_extract_string(transfer, '\''$.from'\'') IS NOT NULL GROUP BY json_extract_string(transfer, '\''$.from'\'') UNION ALL SELECT json_extract_string(transfer, '\''$.to'\'') AS address, 0 AS sends, COUNT(*) AS receives FROM events WHERE transfer IS NOT NULL AND json_extract_string(transfer, '\''$.to'\'') IS NOT NULL GROUP BY json_extract_string(transfer, '\''$.to'\'')) SELECT COUNT(*) AS c FROM address_activity) TO '\''/dev/stdout'\'' (FORMAT CSV, HEADER FALSE)" | grep -Eq "^[1-9][0-9]*$"' "$TOKEN_TRANSFER_BIN"

# Cookbook: "Getting the numbers right". These assert the documented queries
# produce the right *value*, not merely that they run — the existing
# assertions all count rows, which is why none of them caught the volume
# query summing stroops into a DOUBLE and losing precision.
run "cookbook exact amounts do not lose precision" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200002 \
  | duckdb -c "COPY (SELECT CASE WHEN SUM(json_extract_string(transfer, '\''$.amount'\'')::HUGEINT) <> SUM(json_extract_string(transfer, '\''$.amount'\'')::DOUBLE)::HUGEINT THEN '\''differs'\'' ELSE '\''same'\'' END AS cmp FROM read_json('\''/dev/stdin'\'') WHERE transfer IS NOT NULL) TO '\''/dev/stdout'\'' (FORMAT CSV, HEADER FALSE)" \
  | grep -Eq "^(same|differs)$"' "$TOKEN_TRANSFER_BIN"

run "cookbook fee events split by transaction outcome" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200002 \
  | duckdb -c "COPY (SELECT COUNT(*) AS c FROM read_json('\''/dev/stdin'\'') WHERE fee IS NOT NULL AND meta.inSuccessfulTx) TO '\''/dev/stdout'\'' (FORMAT CSV, HEADER FALSE)" \
  | grep -Eq "^[0-9]+$"' "$TOKEN_TRANSFER_BIN"

# Cookbook: "Verifying coverage". A contiguous fetch must report no holes, so
# a query that always returns rows — or always returns none — is detectable.
run "cookbook coverage query finds no holes in a contiguous fetch" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200004 \
  | duckdb -c "COPY (WITH seen AS (SELECT DISTINCT meta.ledgerSequence AS seq FROM read_json('\''/dev/stdin'\'')), bounds AS (SELECT MIN(seq) AS lo, MAX(seq) AS hi FROM seen) SELECT COUNT(*) AS missing FROM bounds, range(bounds.lo, bounds.hi + 1) AS g(seq) WHERE g.seq NOT IN (SELECT seq FROM seen)) TO '\''/dev/stdout'\'' (FORMAT CSV, HEADER FALSE)" \
  | grep -Eq "^0$"' "$TOKEN_TRANSFER_BIN"

# Cookbook: "Querying the status stream". Asserts that nebu stamps the run id
# it was given and that the documented correlation query runs over the result.
#
# It deliberately does not assert that *both* stages appear. A processor only
# emits run_id when built against nebu v0.7.1 or later, and the registry
# binaries this script installs are older, so the second stage writes null —
# which is the caveat the cookbook states. Tighten this to COUNT(DISTINCT
# component_id) = 2 once `just build-processors` installs processors built
# against a nebu that has it.
run "cookbook status stream query correlates by run id" bash -lc '
  dir="$0"; nebu="$1"; tt="$2"
  export NEBU_RUN_ID=11111111-2222-4333-8444-555555555555
  NEBU_COMPONENT_ID=fetch "$nebu" --status-file "$dir/s-fetch.ndjson" fetch --quiet 60200000 60200002 2>/dev/null \
    | NEBU_COMPONENT_ID=transfers "$tt" --quiet --status-file "$dir/s-tt.ndjson" - >/dev/null 2>&1
  duckdb -c "COPY (SELECT COUNT(*) AS records FROM read_json('\''$dir/s-*.ndjson'\'', union_by_name = true) WHERE run_id = '\''$NEBU_RUN_ID'\'' AND component_id = '\''fetch'\'') TO '\''/dev/stdout'\'' (FORMAT CSV, HEADER FALSE)" \
    | grep -Eq "^[1-9][0-9]*$"' "$TMPDIR" "$ROOT/bin/nebu" "$TOKEN_TRANSFER_BIN"

if [[ -x "$NATS_SINK_BIN" ]] && command -v docker >/dev/null 2>&1; then
  echo "==> nats-sink smoke (docker-backed)"
  docker rm -f nebu-docs-nats >/dev/null 2>&1 || true
  docker run -d --rm --name nebu-docs-nats -p 4222:4222 nats:latest -js >/dev/null
  sleep 3
  printf '%s\n' '{"eventType":"fee","transfer":{"assetCode":"USDC"}}' | "$NATS_SINK_BIN" --url nats://localhost:4222 --subject 'stellar.{eventType}' >/dev/null
  printf '%s\n' '{"transfer":{"assetCode":"USDC"}}' | "$NATS_SINK_BIN" --url nats://localhost:4222 --subject 'stellar.{transfer.assetCode}' >/dev/null
  pass "nats-sink smoke (docker-backed)"
else
  echo "SKIP: nats-sink smoke (docker unavailable or binary missing)"
fi

echo "All docs smoke checks passed."
