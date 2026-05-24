---
name: nebu
description: >
  Use this skill when the user wants to compose or run self-hosted Stellar data
  pipelines with existing nebu processors: ledger extraction, token transfers,
  contract events/invocations, DEX/swap activity, liquidity pools, NFT events,
  contract state, transaction/ledger stats, archive backfills, schema-versioned
  JSONL, DuckDB, Postgres, NATS, Kafka, S3, webhooks, or a self-hosted indexer.
  Prefer Stellar `data` for one-off live RPC/Horizon lookups. Do not use for
  building/signing transactions, non-Stellar chains, or implementing new Go
  processors from scratch.
user-invocable: true
argument-hint: [data task]
---

# nebu — Stellar Streaming Runtime

Self-hosted toolkit for Stellar indexers and Unix-composable data pipelines. Nebu turns RPC/archive ledger access into schema-versioned JSONL for tools like `jq`, DuckDB, Postgres, NATS, Kafka, S3, and webhooks.

- **Canonical URL:** https://nebu.withobsrvr.com/SKILL.md
- **Website / quickstart:** https://nebu.withobsrvr.com
- **Repo:** https://github.com/withObsrvr/nebu
- **Last verified against:** nebu v0.6.7 (latest local/remote tag)

## Use nebu when

- extracting bounded Stellar ledger ranges or historical archive backfills
- composing token-transfer, contract-event, invocation, stats, DEX/swap, state, NFT, or liquidity-pool pipelines
- fetching raw ledger XDR once and processing it multiple ways
- installing, inspecting, or composing existing nebu processors

Use another skill/tool for current state lookup (`data`), transaction building (`dapp`), new processor implementation from scratch (nebu processor docs / processor-builder), or non-Stellar chains.

## Gotchas

- Installed binaries are source of truth. **Inspect before proposing or running commands.** Never guess processor names, flags, schemas, fields, or community processor availability.
- Community processors are dynamic; mention them only after `nebu list` confirms they exist locally.
- Prefer bounded ledger ranges. Use `--follow` or unbounded ranges only when the user explicitly asks for live streaming.
- Use `NEBU_RPC_AUTH` from the environment; never paste secrets into commands or committed files.

```bash
nebu --version
nebu list
nebu describe <processor>
<processor> --describe-json | jq .
```

Use `nebu describe` for registry details and `--describe-json` for exact processor flags/schema.

## Pipeline workflow

1. Clarify: data type, ledger range, network, destination, and whether live follow is intentional.
2. Discover processors with `nebu list`.
3. Inspect every intended stage with `nebu describe <name>` and `<name> --describe-json | jq .`.
4. Choose: one origin or `nebu fetch`, zero or more transforms, zero or more sinks/fan-outs.
5. Start with a bounded tracer bullet. Require explicit confirmation for `--follow`, unbounded ranges, production writes, or large backfills.
6. Return the response contract below.

## Pipeline model

```text
ledger source → origin → transform → ... → sink/fan-out
```

- `nebu fetch` emits raw ledger XDR from RPC or archive storage.
- Origins read ledgers from flags or stdin and emit JSONL events.
- Transforms read JSONL on stdin and write JSONL on stdout.
- Sinks read JSONL on stdin and write elsewhere.
- `tee >(sink-a) >(sink-b)` can fan out to multiple destinations.

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
```

For processor lists, env vars, schema examples, and more workflows, read `references/reference.md` only when those details are needed. Trigger eval prompts live in `evals/trigger-evals.json`.

## Safety and stop conditions

Stop and ask if data/range/network/destination is unclear; the user implies unbounded streaming; the range may take hours or stress RPC; a command would expose auth (`NEBU_RPC_AUTH`) or write to production; or no installed processor can do the job.

## Response contract

When proposing a pipeline, return: exact command, one-line stage explanations, one verification command, and flagged assumptions/unverified details.

## Stability

Agent-safe surfaces: `nebu list`, `nebu describe <processor>`, `<processor> --describe-json`, `pkg/processor`, `pkg/source`, `registry.yaml` v1, `description.yml` v1, and schema-versioned JSONL (`_schema`, `_nebu_version`).
