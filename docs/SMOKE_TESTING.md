# Smoke testing

Use a bounded RPC end-to-end smoke test to verify that the built application can
contact a live Stellar RPC server, fetch recent ledgers, decode their XDR, and
emit schema-versioned JSONL through a processor.

This is separate from `make test`, which runs the automated Go test suite, and
`make docs-smoke`, which validates documented CLI examples.

## Prerequisites

- `curl`
- `jq`
- Network access to a Stellar RPC endpoint
- A built nebu CLI and token-transfer processor

Build the required binaries:

```bash
make build-cli
make build-processors
```

## Bounded live RPC end-to-end test

The following test queries the current mainnet ledger, selects a bounded range
of three recent ledgers, and runs the complete RPC-to-JSONL path. It uses a
temporary directory and does not write to any external sink.

```bash
set -euo pipefail

RPC_URL="${NEBU_RPC_URL:-https://rpc.lightsail.network}"
RPC_HEADERS=()
if [[ -n "${NEBU_RPC_AUTH:-}" ]]; then
  RPC_HEADERS=(-H "Authorization: $NEBU_RPC_AUTH")
fi

TMPDIR="$(mktemp -d)"
EVENTS="$TMPDIR/token-transfers.jsonl"
trap 'rm -rf "$TMPDIR"' EXIT

LATEST="$({
  curl --fail --silent --show-error \
    -X POST \
    -H 'Content-Type: application/json' \
    "${RPC_HEADERS[@]}" \
    --data '{"jsonrpc":"2.0","id":1,"method":"getLatestLedger"}' \
    "$RPC_URL"
} | jq -er '.result.sequence')"
START=$((LATEST - 2))

echo "Testing bounded mainnet range $START..$LATEST"

./bin/nebu fetch \
  --quiet \
  --rpc-url "$RPC_URL" \
  --network mainnet \
  "$START" "$LATEST" \
  | ./bin/token-transfer --quiet \
  > "$EVENTS"

test -s "$EVENTS"
jq -e '
  ._schema == "nebu.token_transfer.v1" and
  ._nebu_version != null
' "$EVENTS" >/dev/null

echo "PASS: produced $(wc -l < "$EVENTS") schema-versioned JSONL events"
```

A passing run proves that:

1. The RPC endpoint responds to JSON-RPC requests.
2. `nebu fetch` retrieves the complete bounded ledger range.
3. The fetched ledger XDR can be decoded by a processor.
4. The processor emits valid, schema-versioned JSONL.
5. Every process in the pipeline exits successfully.

The exact event count varies with live ledger activity. The test therefore
checks for non-empty, valid output rather than a fixed count.

## Troubleshooting

- **The latest-ledger query fails:** verify `RPC_URL`, network access, and any
  required RPC authentication.
- **Ledger fetch fails:** retry with a fresh `LATEST` value; RPC servers retain
  only a limited recent ledger window.
- **The output file is empty:** increase the bounded range modestly, for example
  by setting `START=$((LATEST - 9))`. Do not use `--follow` for this test.
- **Schema validation fails:** inspect the first event with
  `head -n 1 "$EVENTS" | jq .` and confirm that the processor binary matches the
  checked-out nebu version.

For authenticated RPC endpoints, provide credentials through the environment,
such as `NEBU_RPC_AUTH`; never place tokens directly in documentation or shell
history.
