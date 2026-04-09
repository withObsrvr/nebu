---
name: nebu-pipeline-composer
description: Compose multi-stage Stellar ledger pipelines from existing nebu processors (origin → transform → sink). Use when the user wants to extract, filter, or store Stellar blockchain data and is willing to use Unix pipes over a custom Go program. Discovers the processor catalog at runtime via `nebu list` and each processor's flags via `--describe-json` — never hardcodes flag names or schema shapes.
---

# Nebu Pipeline Composer

Compose working Stellar ledger extraction pipelines from the processors installed locally. Every pipeline is a shell pipe chain: one origin produces events, zero or more transforms filter/enrich them, one sink writes the final stream somewhere.

## When to use this skill

**Use it when the user asks to:**
- Extract token transfers / contract events / contract invocations / account effects from Stellar ledgers
- Filter a Stellar event stream by asset, amount, account, contract, or time window
- Store Stellar events to a file, Postgres, NATS, S3, Kafka, or a webhook
- Chain two or more of the above into one pipeline
- Backfill a bounded ledger range OR stream continuously from a starting ledger

**Do NOT use it when the user asks to:**
- Write a *new* processor from scratch — hand off to the `nebu-processor-builder` skill
- Interact with non-Stellar chains — nebu is Stellar-specific
- Submit transactions, manage wallets, or sign anything — nebu is read-only extraction
- Build a long-running HTTP API — nebu pipelines are batch/stream jobs, not services

If the request looks like it needs a custom processor (new extraction logic, new sink protocol), say so and stop. Don't improvise by shelling out to awk/jq for complex logic that belongs in Go.

## The three primitives

Every nebu pipeline has the same shape:

```
origin  →  transform₀  →  transform₁  →  …  →  sink
```

- **origin**: connects to Stellar RPC (or reads XDR ledgers from stdin/file) and emits typed events as JSONL on stdout. Examples: `token-transfer`, `contract-events`, `contract-invocation`, `account-effects`.
- **transform**: reads JSONL from stdin, writes filtered/modified JSONL to stdout. Examples: `amount-filter`, `usdc-filter`, `dedup`, `time-window`, `account-filter`, `contract-filter`, `asset-filter`, `rate-limiter`, `aggregator`.
- **sink**: reads JSONL from stdin, writes to an external destination (no stdout). Examples: `json-file-sink`, `postgres-sink`, `nats-sink`, `kafka-sink`, `s3-sink`, `webhook-sink`.

All three types are standalone binaries. The composition is a plain Unix pipe chain.

## Mandatory first step: discover, don't hallucinate

**Before writing any pipeline, run these two commands and read the output.** This skill intentionally does not list processor names, flags, or schema shapes inline — the installed nebu version is the source of truth.

```bash
# Enumerate every installed processor, grouped by type
nebu list

# For each candidate processor, get the exact flags, schema, and version
<processor-name> --describe-json | jq .
```

The `--describe-json` envelope contains:
- `name`, `version`, `type` (origin | transform | sink)
- `description`
- `schema.id` — canonical schema identifier (e.g., `nebu.token_transfer.v1`)
- `schema.output` — full JSON Schema Draft 2020-12 document describing the event shape
- `flags[]` — each flag with `name`, `type`, `default`, `required`

**If you skip this step and hardcode a flag name or schema field, your pipeline will fail on a protocol shift.** The `--describe-json` envelope is part of nebu's stable contract — it won't rot.

## Composition workflow

1. **Clarify the goal.** What asset? What ledger range? Where should the output land? Get concrete before composing.
2. **Pick an origin.** Run `nebu list` and choose the origin whose output schema carries the fields you need.
3. **Inspect its shape.** Run `<origin> --describe-json | jq '.schema.output'` and note which fields are at which paths. Protojson emits camelCase; nested messages live in `$defs`.
4. **Pick transforms.** Each transform filters OR modifies; chain as many as you need. If two filters are independent, order doesn't matter for correctness but may matter for throughput — put the cheapest/most-selective one first.
5. **Pick a sink.** Match the sink to where the data needs to land. For one-off exploration, `json-file-sink` or just redirecting to a file is fine.
6. **Compose and run.** A pipeline is a shell pipe chain. Example shape:

   ```bash
   <origin> --start-ledger N --end-ledger M [--network mainnet] \
     | <transform> [flags] \
     | <transform> [flags] \
     | <sink> [flags]
   ```

7. **Verify.** After the run completes, check the stderr summaries (each stage reports its event counts) and the sink's output file/table.

## Worked example: USDC transfers over 10k XLM, written to JSONL

This is the canonical walkthrough. Adapt it to other assets/filters/sinks.

**Goal:** Capture every USDC transfer of 10 XLM (100,000,000 stroops) or more from ledgers 60200000–60200100 on mainnet, stored as JSONL.

### Step 1: Discover

```bash
nebu list | grep -i -e transfer -e filter -e sink
```

Candidates surface: `token-transfer` (origin), `usdc-filter` (transform), `amount-filter` (transform), `json-file-sink` (sink).

### Step 2: Inspect the origin's schema

```bash
token-transfer --describe-json | jq '.schema'
```

The relevant fields (from the envelope at the installed version): every event has `meta.ledgerSequence`, `meta.txHash`, `meta.inSuccessfulTx`. The polymorphic event payload is one of `transfer`, `mint`, `burn`, `clawback`, or `fee` at the top level. For a `transfer`, the payload has `assetCode`, `assetIssuer`, `from`, `to`, `amount` (stroops as string).

### Step 3: Inspect the filters' flags

```bash
usdc-filter --describe-json | jq '.flags'
amount-filter --describe-json | jq '.flags'
```

Read the actual flag names from the envelope, not from guesses. At the time of writing `amount-filter` exposes `--min`, `--max`, `--asset` — but verify against your local version.

### Step 4: Compose and run

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200100 --network mainnet \
  | usdc-filter \
  | amount-filter --min 100000000 \
  | json-file-sink --out /tmp/usdc-big.jsonl
```

### Step 5: Verify

```bash
wc -l /tmp/usdc-big.jsonl
head -1 /tmp/usdc-big.jsonl | jq '.transfer | {assetCode, assetIssuer, amount}'
```

Every row should have `assetCode == "USDC"` and `amount >= 100000000`. If the count is zero, check each stage's stderr for event counts — one of the filters may be walking a stale schema path (see Pitfall 2 below).

## Common pitfalls

### Pitfall 1: Stdin auto-detect vs explicit RPC mode

Origins support three input modes:
- **RPC mode**: `<origin> --start-ledger N --end-ledger M` — fetches from Stellar RPC.
- **Pipe mode**: `nebu fetch N M | <origin>` — reads XDR ledgers from stdin.
- **File mode**: `<origin> ledgers.xdr` — reads XDR ledgers from a file.

When `--start-ledger` is set, RPC mode wins even if stdin is `/dev/null` (CI/cron case). When `--start-ledger` is absent and stdin is a pipe, pipe mode auto-activates. Don't combine `--start-ledger` and piped stdin — the start-ledger flag takes priority and the piped input is ignored.

### Pitfall 2: Schema path drift between filters

Not every transform is updated every release. If a transform produces zero output on data that clearly matches its advertised behavior, dump one raw event after the origin and diff its shape against the path the transform is walking. The flat proto JSON shape (`transfer.assetCode`) is the current contract as of nebu v0.6.1.

Reproducing this is fast:

```bash
nebu fetch 60200000 60200002 | token-transfer | head -3 | jq .
```

### Pitfall 3: json-file-sink doesn't create the file on zero events

`json-file-sink` opens its output file lazily on the first event. If the upstream filter produces zero events, you get no file at all — not an empty file. If you need an empty-file sentinel, `touch` the path before running the pipeline.

### Pitfall 4: Unbounded streaming runs forever

`--end-ledger 0` (or omitting `--end-ledger`) puts the origin into unbounded streaming mode — it tails the chain indefinitely. That is intentional for long-running sinks but will never return control to an interactive shell. Always set `--end-ledger` for one-shot extraction.

### Pitfall 5: Progress bars disappear when stderr is piped

Origins like `token-transfer` wire a progress hook that prints to stderr *only when stderr is a terminal*. When you redirect stderr to a file or `/dev/null`, or run inside a non-interactive shell, the progress bar suppresses itself. Event-count summary lines still appear at the end.

## Handoffs

- **User wants a new processor** (their extraction/filter/sink logic doesn't exist yet) → stop and invoke `nebu-processor-builder` (planned; for now point them at `skills/nebu-processor-builder/` in the `nebu-processor-registry` repo).
- **User wants a long-running production deployment** → nebu pipelines are the prototype; they graduate to [flowctl](./docs/GRADUATING_TO_FLOWCTL.md) for managed scheduling, checkpointing, and restart.
- **User wants observability beyond stderr** → see [`docs/HOOKS.md`](./docs/HOOKS.md) for the runtime hooks interface (progress, metrics, tracing).
- **User's pipeline fails with "XDR decode error"** — most common cause is piping into an origin that also has `--start-ledger` set. Drop one or the other.

## Output contract

A pipeline invocation should produce:

1. A shell command the user can copy and run
2. The expected stderr output shape (which stages report counts, approximate numbers)
3. A one-line verification command the user can run against the sink output
4. If the pipeline is nontrivial, a short paragraph explaining *why* each stage is there

Do not claim the pipeline works without actually running it or explicitly flagging it as untested.

## Skill metadata

- **Version:** 0.1.0
- **Maintainer:** OBSRVR
- **License:** MIT
- **Tracks nebu contract:** v0.6.1 and forward (via `--describe-json`)
- **Source:** [github.com/withObsrvr/nebu/skills/pipeline-composer](https://github.com/withObsrvr/nebu)
