---
name: nebu
description: >
  Use for self-hosted Stellar data pipelines with nebu processors: ledger ranges,
  archive backfills, token transfers, contract events/invocations, DEX/swaps,
  liquidity pools, NFT/state/stats, JSONL, DuckDB/Postgres, NATS/Kafka, S3,
  webhooks, self-hosted indexers. Prefer Stellar `data` for one-off live
  RPC/Horizon lookups. Not for tx building/signing, non-Stellar chains, or new
  Go processor impl from scratch.
user-invocable: true
argument-hint: [data task]
---

# nebu — Stellar streaming runtime

Nebu turns Stellar RPC/archive ledgers into schema-versioned JSONL for `jq`, DuckDB, Postgres, NATS/Kafka, S3, webhooks.

- Canonical: https://nebu.withobsrvr.com/SKILL.md
- Repo: https://github.com/withObsrvr/nebu
- Last checked: nebu v0.6.7

## Use when

- bounded ledger extracts or historical backfills
- token transfers, contract events/invocations, DEX/swaps, pools, NFT/state/stats
- fetch raw ledger XDR once, process many ways
- inspect/install/compose existing processors

Use other tools for current-state lookup (`data`), tx building (`dapp`), non-Stellar, or brand-new processor impl.

## Rules

- Installed binaries are truth. Inspect before commands; do not guess names/flags/schemas/community processors.
- Community processors vary. Mention only after `nebu list` shows them.
- Prefer bounded ranges. `--follow`/unbounded only if user asks live stream.
- Broad scans/backfills: prefer `nebu fetch --mode archive` from public archive → pipe XDR into processors. RPC still good for latest ledgers + targeted `getEvents` by known contract/topic.
- `contract-events` works from `nebu fetch` stdin, including S3 archive mode; it decodes contract events + diagnostics without processor RPC calls.
- Use `NEBU_RPC_AUTH` from env; never paste secrets.

```bash
nebu --version
nebu list
nebu describe <processor>
<processor> --describe-json | jq .
```

## Workflow

1. Confirm data type, range, network, destination, live vs bounded.
2. `nebu list`; inspect each stage with `nebu describe` / `--describe-json`.
3. Pick one source (`nebu fetch` or origin), transforms, sinks/fan-out.
4. Start bounded tracer bullet. Ask before `--follow`, prod writes, or large backfills.
5. Answer with: exact command, one-line stage notes, verification cmd, assumptions/unverified bits.

## Model

```text
ledger source → origin → transform(s) → sink/fan-out
```

- `nebu fetch`: raw ledger XDR from RPC/archive.
- Origins: ledgers from flags/stdin → JSONL.
- Transforms: JSONL stdin → JSONL stdout.
- Sinks: JSONL stdin → destination.
- Fan out with `tee >(sink-a) >(sink-b)`.

## Quick commands

```bash
go install github.com/withObsrvr/nebu/cmd/nebu@latest
export PATH="$HOME/go/bin:$PATH"
nebu install token-transfer

token-transfer --start-ledger 60200000 --end-ledger 60200010 \
  | json-file-sink --out /tmp/transfers.jsonl

nebu fetch 60200000 60200100 > ledgers.xdr
cat ledgers.xdr | token-transfer | jq -c 'select(.transfer != null)'

nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "aws-public-blockchain/v1.1/stellar/ledgers/pubnet" \
  --region us-east-2 \
  62080000 62081000 | gzip > historical.xdr.gz

nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "aws-public-blockchain/v1.1/stellar/ledgers/pubnet" \
  --region us-east-2 \
  62080000 62080100 | contract-events | jq -c 'select(.eventType == "swap")'
```

More details only if needed: `references/reference.md`. Evals: `evals/trigger-evals.json`.

## Stop / ask

Ask if range/network/destination unclear, unbounded stream implied, range may stress RPC/take hours, command exposes auth, prod write requested, or no installed processor fits.

## Stable surfaces

`nebu list`, `nebu describe <processor>`, `<processor> --describe-json`, `pkg/processor`, `pkg/source`, `registry.yaml` v1, `description.yml` v1, schema-versioned JSONL (`_schema`, `_nebu_version`).
