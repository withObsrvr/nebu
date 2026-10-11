# Checkpoint and resume

`nebu fetch` can persist the last ledger successfully written to its output.
This is opt-in and works independently of the processor or sink downstream:

```bash
nebu fetch --mode archive \
  --checkpoint ./state/backfill.checkpoint.json \
  --checkpoint-interval 1 \
  60200000 60210000 \
  | token-transfer \
  >> transfers.jsonl
```

Run the same command after an interruption. Nebu validates that the checkpoint
belongs to the same requested range and resumes at the ledger after
`last_completed_ledger`. A completed range becomes a successful no-op.

When `--output` is used, a resumed fetch appends to the existing XDR file.
Without `--checkpoint`, fetch behavior is unchanged and `--output` truncates an
existing file.

## Delivery semantics

The default interval is one ledger. On SIGINT or SIGTERM, nebu durably writes
the last ledger accepted by its output before returning. Checkpoints are
replaced atomically, so a partial state file is never treated as valid.

`--checkpoint-interval N` reduces filesystem synchronization overhead, but a
hard crash can replay up to `N-1` ledgers from the last durable checkpoint.
This is at-least-once delivery: downstream consumers should use stable event
keys such as TOIDs when deduplication is required. A Unix pipe has no general
acknowledgement from an arbitrary sink, so nebu does not claim transactional
exactly-once delivery across the entire pipeline.

Checkpoint files are local in v1. For remote execution, place the path on a
durable mounted volume. Object-store checkpoint keys can be added later without
turning nebu into a coordinator or service manager.

## Existing `nebu resume`

`nebu resume` remains a Postgres convenience that derives a starting ledger
from an existing table. New sink-independent workflows should prefer
`nebu fetch --checkpoint`.
