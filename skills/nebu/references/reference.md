# nebu reference

Use this only after loading `SKILL.md` when the task needs exact examples, install steps, or processor categories.

## Installation

Recommended user install:

```bash
go install github.com/withObsrvr/nebu/cmd/nebu@latest
export PATH="$HOME/go/bin:$PATH"
nebu --version
nebu list
nebu install token-transfer
token-transfer --start-ledger 60200000 --end-ledger 60200001
```

Development install from a pinned tag:

```bash
git clone --branch <release-tag> https://github.com/withObsrvr/nebu && cd nebu
make install
export PATH="$HOME/go/bin:$PATH"
nebu list
```

Published Docker image for quick previews:

```bash
docker run --rm withobsrvr/nebu:latest \
  token-transfer --start-ledger 60200000 --end-ledger 60200001
```

## Configuration

```bash
NEBU_RPC_URL          # Stellar RPC endpoint; default is mainnet archive RPC
NEBU_RPC_AUTH         # Authorization header, e.g. 'Api-Key ...' (keep secret in env)
NEBU_NETWORK          # 'mainnet', 'testnet', or full passphrase
NEBU_MODE             # 'rpc' or 'archive' for nebu fetch
NEBU_DATASTORE_TYPE   # 'S3' or 'GCS' for archive mode
NEBU_BUCKET_PATH      # archive bucket/path
NEBU_REGION           # S3 region, e.g. us-east-2
NEBU_BUFFER_SIZE      # archive fetch buffer size
NEBU_NUM_WORKERS      # archive fetch worker count
```

**Trust posture:** nebu inherits the trust of the RPC endpoint or archive source. For mainnet work, use a trusted provider or your own infrastructure. Never hardcode `NEBU_RPC_AUTH` in committed scripts.

## Built-in processors in v0.6.7

Always confirm with `nebu list` against the installed version.

| Name | Type | Purpose |
| --- | --- | --- |
| `token-transfer` | origin | SAC/XLM transfers, mints, burns, clawbacks, fees |
| `contract-events` | origin | All Soroban contract events |
| `contract-invocation` | origin | Contract function calls, cross-contract calls, state changes |
| `transaction-stats` | origin | Aggregate transaction/operation success/failure stats |
| `ledger-change-stats` | origin | Per-ledger created/updated/deleted/change-reason stats |
| `usdc-filter` | transform | Keep USDC token-transfer events |
| `amount-filter` | transform | Filter token transfers by amount/asset ranges |
| `dedup` | transform | Deduplicate by a key field |
| `time-window` | transform | Filter by time or ledger sequence window |
| `json-file-sink` | sink | Write JSONL files |
| `nats-sink` | sink | Publish to NATS / JetStream |
| `postgres-sink` | sink | Insert JSONB rows with TOID-based idempotency |

A current install may also show community processors for account/asset/contract filters, DEX/swap detection, contract state, NFT events, TTL tracking, trade extraction, Kafka/S3/webhook sinks, and protocol-specific Soroswap/Aquarius workflows. Treat community availability as dynamic.

## Output schema

Every event includes versioning metadata. Filter by `_schema` so migrations do not silently change downstream logic.

Token-transfer v1 currently uses flat transfer asset fields; confirm with `token-transfer --describe-json` before depending on exact fields:

```json
{
  "_schema": "nebu.token_transfer.v1",
  "_nebu_version": "v0.6.7",
  "meta": {
    "ledgerSequence": 60200000,
    "closedAtUnix": "1765158311",
    "txHash": "abc...",
    "transactionIndex": 1,
    "contractAddress": "CA..."
  },
  "transfer": {
    "from": "GA...",
    "to": "GB...",
    "assetCode": "USDC",
    "assetIssuer": "GA...",
    "amount": "1000000"
  }
}
```

DuckDB schema guard:

```sql
SELECT * FROM read_json('events.jsonl')
WHERE _schema = 'nebu.token_transfer.v1';
```

## Common workflows

Install and inspect a processor:

```bash
nebu install token-transfer
nebu describe token-transfer
token-transfer --describe-json | jq .
```

Extract a bounded range to a file:

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200010 \
  | json-file-sink --out /tmp/transfers.jsonl
```

Track USDC transfers:

```bash
token-transfer --start-ledger 60200000 --end-ledger 60201000 \
  | jq -c 'select(.transfer.assetCode == "USDC")'
```

Aggregate with DuckDB:

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 \
  | duckdb -c "SELECT COUNT(*) AS transfers FROM read_json('/dev/stdin') WHERE transfer IS NOT NULL"
```

Separate fetching from processing:

```bash
nebu fetch 60200000 60200100 > ledgers.xdr
cat ledgers.xdr | token-transfer | jq -c 'select(.transfer != null)'
```

Fetch historical archive data from public AWS S3 without credentials:

```bash
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "aws-public-blockchain/v1.1/stellar/ledgers/pubnet" \
  --region us-east-2 \
  62080000 62081000 | gzip > historical.xdr.gz
```

Fan out to multiple destinations:

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 \
  | tee >(nats-sink --subject "stellar.transfers" --jetstream) \
  | tee >(json-file-sink --out transfers.jsonl) \
  | jq -r '"Ledger \(.meta.ledgerSequence): \(.transfer.amount)"'
```

Live stream only when explicitly requested:

```bash
token-transfer --start-ledger 60200000 --follow \
  | nats-sink --subject "stellar.transfers" --jetstream
```

## Backfills and resume

For large ranges, recommend a small probe first, then chunked or archive-mode backfills. `nebu fetch --mode archive` is the preferred historical path when public S3/GCS data covers the range. If the installed CLI supports `nebu resume`, inspect `nebu resume --help` before using it for checkpointed runs.

## Stability notes

Agent-safe surfaces:

- `pkg/processor`, `pkg/source`, `registry.yaml` v1, `description.yml` v1
- `nebu list` and `nebu describe <processor>`
- `<processor> --describe-json`
- schema-versioned JSONL with `_schema` and `_nebu_version`

See https://nebu.withobsrvr.com/STABILITY.html or `docs/STABILITY.md` before relying on package-level APIs. Runtime hooks (`docs/HOOKS.md`) support progress, metrics, tracing, and agent gating for custom processors.
