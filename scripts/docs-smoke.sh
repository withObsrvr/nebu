#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
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
  "$NEBU_BIN" "$TOKEN_TRANSFER_BIN" "$CONTRACT_EVENTS_BIN" "$USDC_FILTER_BIN" \
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
run "token-transfer jq USDC filter" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200002 | jq -e "select(.transfer != null and .transfer.assetCode == \"USDC\")" | head -1 >/dev/null' "$TOKEN_TRANSFER_BIN"
run "contract-events jq eventType filter" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200000 | jq -e "select(.eventType == \"fee\")" | head -1 >/dev/null' "$CONTRACT_EVENTS_BIN"
run "nebu fetch | token-transfer" bash -lc '"$0" fetch 60200000 60200000 | "$1" --quiet | head -1 | jq -e "._schema == \"nebu.token_transfer.v1\"" >/dev/null' "$NEBU_BIN" "$TOKEN_TRANSFER_BIN"

run "json-file-sink write" bash -lc 'out="$1/out.jsonl"; "$2" --quiet --start-ledger 60200000 --end-ledger 60200000 | "$3" --out "$out" >/dev/null; test -s "$out"; jq -e "has(\"_schema\")" "$out" >/dev/null' _ "$TMPDIR" "$TOKEN_TRANSFER_BIN" "$JSON_FILE_SINK_BIN"
run "usdc-filter flat assetCode" bash -lc 'printf "%s\n" "{\"_schema\":\"nebu.token_transfer.v1\",\"meta\":{\"ledgerSequence\":60200000,\"txHash\":\"abc\"},\"transfer\":{\"from\":\"GA\",\"to\":\"GB\",\"assetCode\":\"USDC\",\"assetIssuer\":\"GI\",\"amount\":\"1000000\"}}" | "$0" | jq -e ".transfer.assetCode == \"USDC\"" >/dev/null' "$USDC_FILTER_BIN"
run "amount-filter flat assetCode" bash -lc 'printf "%s\n" "{\"_schema\":\"nebu.token_transfer.v1\",\"meta\":{\"ledgerSequence\":60200000,\"txHash\":\"abc\"},\"transfer\":{\"from\":\"GA\",\"to\":\"GB\",\"assetCode\":\"USDC\",\"assetIssuer\":\"GI\",\"amount\":\"1000000\"}}" | "$0" --min 999999 --asset USDC | jq -e ".transfer.assetCode == \"USDC\"" >/dev/null' "$AMOUNT_FILTER_BIN"
run "dedup nested key" bash -lc 'printf "%s\n%s\n" "{\"meta\":{\"txHash\":\"a\"},\"transfer\":{\"assetCode\":\"USDC\",\"amount\":\"1\"}}" "{\"meta\":{\"txHash\":\"a\"},\"transfer\":{\"assetCode\":\"USDC\",\"amount\":\"1\"}}" | "$0" --key meta.txHash | wc -l | grep -qx 1' "$DEDUP_BIN"

run "pipeline doc: token-transfer | json-file-sink" bash -lc 'out="$1/pipeline.jsonl"; "$2" --quiet --start-ledger 60200000 --end-ledger 60200000 | "$3" --out "$out" >/dev/null; jq -e "select(.transfer != null or .fee != null)" "$out" >/dev/null' _ "$TMPDIR" "$TOKEN_TRANSFER_BIN" "$JSON_FILE_SINK_BIN"
run "pipeline doc: token-transfer | duckdb" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200002 | duckdb -c "COPY (SELECT COUNT(*) AS c FROM read_json(\"/dev/stdin\") WHERE transfer IS NOT NULL OR fee IS NOT NULL) TO \"/dev/stdout\" (FORMAT CSV, HEADER FALSE)" | grep -Eq "^[1-9][0-9]*$"' "$TOKEN_TRANSFER_BIN"
run "duckdb cookbook assetCode query" bash -lc '"$0" --quiet --start-ledger 60200000 --end-ledger 60200002 | duckdb -c "COPY (SELECT COUNT(*) AS c FROM read_json(\"/dev/stdin\") WHERE transfer IS NOT NULL AND json_extract_string(transfer, '\''$.assetCode'\'') IS NOT NULL) TO \"/dev/stdout\" (FORMAT CSV, HEADER FALSE)" | grep -Eq "^[1-9][0-9]*$"' "$TOKEN_TRANSFER_BIN"

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
