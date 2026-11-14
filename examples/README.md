# nebu Examples

This directory contains runnable examples for building Stellar data pipelines with nebu.

## Running Examples

Run the shipped examples directly:

```bash
go run examples/simple_origin/main.go
go run examples/go-library/transaction-stats/main.go
go run examples/go-library/ledger-change-stats/main.go
```

Or via Make for the simple origin example:

```bash
make run-example
```

## Included Examples

### `simple_origin` — Counting Origin Processor

This example demonstrates:
- creating an RPC-backed ledger source
- implementing a basic `processor.Origin`
- wiring source → runtime → processor
- processing a bounded ledger range
- graceful shutdown on Ctrl+C

### `go-library/transaction-stats` — Transaction Statistics

This example demonstrates:
- embedding nebu as a Go library
- using `runtime.RunOrigin` over an RPC-backed range
- reading transactions from `xdr.LedgerCloseMeta`
- computing successful/failed transaction and operation counts

Run with:

```bash
go run examples/go-library/transaction-stats/main.go
```

### `go-library/ledger-change-stats` — Ledger Change Statistics

This example demonstrates:
- embedding nebu as a Go library
- reading ledger entry changes from `xdr.LedgerCloseMeta`
- classifying created / updated / deleted changes
- classifying fee / transaction / operation-caused changes

Run with:

```bash
go run examples/go-library/ledger-change-stats/main.go
```

### `processors/checkpoint-entries` — Ledger Entries From a Checkpoint

A history archive checkpoint is a full state snapshot, published every 64
ledgers. This example streams the entries it holds as JSONL:

```bash
just build-processors
./bin/checkpoint-entries --network testnet --limit 2000
```

It is not a `processor.Origin`: there is no `LedgerCloseMeta` to process, so
it reads the bucket list through `ingest.CheckpointChangeReader` and keeps its
own command rather than going through nebu's origin CLI. Entry types, account
ids, assets, balances, offer ids, pool ids and contract ids come out flat, one
JSON object per entry.

```bash
# Entry mix in the first 200k entries of a pubnet checkpoint
./bin/checkpoint-entries --checkpoint 65470463 --limit 200000 -q | jq -r .entry_type | sort | uniq -c
```

A pubnet checkpoint holds tens of millions of entries. `--limit` reads in
bucket order, which is a sample of the bucket list and **not** a sample of the
network — drop it for network-wide numbers, and expect the read to take a
while.

### Standalone processor forms

The same ideas also exist as example standalone processors:
- `examples/processors/transaction-stats`
- `examples/processors/ledger-change-stats`
- `examples/processors/checkpoint-entries`

## The Stellar ingest SDK examples, in nebu

The Stellar [ingest SDK examples](https://developers.stellar.org/docs/data/indexers/build-your-own/ingest-sdk/examples/index)
are three Go programs. Each has an equivalent here, and the third one is
shaped differently on purpose.

| Stellar SDK example | reads | nebu equivalent |
| --- | --- | --- |
| Transaction Statistics | a bounded ledger range via captive core | [`go-library/transaction-stats`](./go-library/transaction-stats) as a library, [`processors/transaction-stats`](./processors/transaction-stats) as a pipeline stage |
| Ledger Entry Changes | a bounded ledger range via captive core | [`go-library/ledger-change-stats`](./go-library/ledger-change-stats), [`processors/ledger-change-stats`](./processors/ledger-change-stats) |
| Ledger Entry Statistics | a checkpoint snapshot from the history archive | [`processors/checkpoint-entries`](./processors/checkpoint-entries) plus a query |

Two differences are worth knowing before you pick one.

**nebu's equivalents do not need captive core.** The first two SDK examples
run a `stellar-core` binary as a subprocess with a config file and an archive
URL list. nebu's read the same ledgers over RPC or from a datastore, so
`--start-ledger 60200000 --end-ledger 60200100` is the whole setup. Captive
core is still the right backend for a validator-adjacent deployment; it is a
lot of moving parts for an example.

**The third one is a stream, not a tally.** The SDK's Ledger Entry Statistics
example prints four totals — accounts, accounts holding trustlines, claimable
balances, offers. Those four are the only four it can answer; a fifth means
editing Go and reading the checkpoint again. `checkpoint-entries` emits the
entries instead, so the statistics are a query:

```bash
./bin/checkpoint-entries --checkpoint 65470463 --limit 200000 -q > entries.jsonl

duckdb -c "
SELECT
  count(*)                                                                 AS total_entries,
  count_if(entry_type = 'account')                                         AS num_accounts,
  count(DISTINCT CASE WHEN entry_type = 'trustline' THEN account_id END)   AS num_accounts_with_trustlines,
  count_if(entry_type = 'claimable_balance')                               AS num_claimable_balances,
  count_if(entry_type = 'offer')                                           AS num_offers
FROM read_json('entries.jsonl');"
```

```
┌───────────────┬──────────────┬──────────────────────────────┬────────────────────────┬────────────┐
│ total_entries │ num_accounts │ num_accounts_with_trustlines │ num_claimable_balances │ num_offers │
│        200000 │        25977 │                         9790 │                     98 │       3253 │
└───────────────┴──────────────┴──────────────────────────────┴────────────────────────┴────────────┘
```

Those are the SDK example's four numbers, over the first 200,000 entries of
pubnet checkpoint 65470463 — read in under four seconds. Drop `--limit` for
the whole snapshot and they are the network-wide totals that example prints.

**Pin `--checkpoint` when you want the numbers back.** Without it the latest
checkpoint is resolved, which advances every 64 ledgers, so two runs minutes
apart sample different snapshots: the same command gave 26,988 accounts and
then 25,977 while this section was being written. With the checkpoint pinned
the read is deterministic — the buckets and their order are fixed, so the same
`--limit` returns the same entries.

The same file answers questions that example cannot, with no second read:

```bash
duckdb -c "
SELECT asset, count(*) AS trustlines
FROM read_json('entries.jsonl')
WHERE entry_type = 'trustline'
GROUP BY asset ORDER BY trustlines DESC LIMIT 3;"
```

```
┌───────────────────────────────────────────────────────────────┬────────────┐
│ KALE:GBDVX4VELCDSQ54KQJYTNHXAHFLBCA77ZY2USQBM4CSHTTV7DME7KALE │       6147 │
│ USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN │       1012 │
│ XTAR:GAORYJ3KBDGIM7FFSKVUJHJ5NEFWIRDIAGGBJBJS7TY6ECZS53257IG4 │        780 │
└───────────────────────────────────────────────────────────────┴────────────┘
```

One caution carried over from [the DuckDB cookbook](../docs/DUCKDB_COOKBOOK.md):
cast balance sums to `HUGEINT`. Trustline balances in these 200,000 entries
already sum to 356,770,404,264,524,855,041 stroops, which overflows `BIGINT`
and loses precision silently in `DOUBLE`.

Where a tally is the right shape, nebu keeps it: `transaction-stats` and
`ledger-change-stats` emit one aggregate per ledger, because a per-ledger
summary is small and a per-entry stream of a whole checkpoint is not.

### Core shape

Implement `processor.Origin`:

```go
type CountingOrigin struct {
    count int
}

func (o *CountingOrigin) Name() string { return "counting-origin" }
func (o *CountingOrigin) Type() processor.Type { return processor.TypeOrigin }

func (o *CountingOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
    o.count++
    fmt.Printf("Processed ledger %d\n", ledger.LedgerSequence())
}
```

Create the source:

```go
src, err := rpc.NewLedgerSource("https://archive-rpc.lightsail.network")
if err != nil {
    panic(err)
}
defer src.Close()
```

Run it with the runtime:

```go
rt := runtime.NewRuntime()
err = rt.RunOrigin(ctx, src, origin, 60200000, 60200009)
```

### Environment variables

- `NEBU_RPC_URL` — override the default RPC endpoint

## Building Your Own

General guidelines:

1. Implement `processor.Origin`, `processor.Transform`, or `processor.Sink`
2. Respect `ctx.Done()` for cancellation
3. Report per-item problems via the processor reporter helpers instead of returning stream-killing errors
4. Test against a small real ledger range first

Example skeleton:

```go
package main

import (
    "context"

    "github.com/stellar/go-stellar-sdk/xdr"
    "github.com/withObsrvr/nebu/pkg/processor"
    "github.com/withObsrvr/nebu/pkg/runtime"
    "github.com/withObsrvr/nebu/pkg/source/rpc"
)

type MyProcessor struct{}

func (p *MyProcessor) Name() string { return "my-processor" }
func (p *MyProcessor) Type() processor.Type { return processor.TypeOrigin }

func (p *MyProcessor) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
    _ = ctx
    _ = ledger
}

func main() {
    src, err := rpc.NewLedgerSource("https://archive-rpc.lightsail.network")
    if err != nil {
        panic(err)
    }
    defer src.Close()

    proc := &MyProcessor{}
    rt := runtime.NewRuntime()

    if err := rt.RunOrigin(context.Background(), src, proc, 60200000, 60200009); err != nil {
        panic(err)
    }
}
```

## Tips

- Use small ranges first, e.g. 5–10 ledgers
- `pkg/source/rpc/source.go` is the reference RPC implementation
- Always check the error returned by `RunOrigin`

## Questions?

See the main [README](../README.md) for core concepts and API docs.
