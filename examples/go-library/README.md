# Go Library Examples

These examples show the progression from raw Stellar ingest concepts to Nebu-style code using:
- `pkg/source/rpc`
- `pkg/runtime`
- `pkg/processor`

They are the Go-library counterparts to the example CLI processors in `examples/processors/transaction-stats` and `examples/processors/ledger-change-stats`.

## Included examples

### `transaction-stats`

Aggregate successful/failed transaction and operation counts over a bounded ledger range.

Run:

```bash
go run ./examples/go-library/transaction-stats/main.go
```

### `ledger-change-stats`

Read ledger entry changes from each ledger and print created / updated / deleted counts plus fee / transaction / operation-caused change counts.

Run:

```bash
go run ./examples/go-library/ledger-change-stats/main.go
```

## Relationship to CLI processors

These examples are custom Go programs that embed Nebu as a library.

The corresponding standalone Nebu-style processors live here:
- `examples/processors/transaction-stats`
- `examples/processors/ledger-change-stats`

Those can be run like normal processors:

```bash
transaction-stats --start-ledger 60200000 --end-ledger 60200009
ledger-change-stats --start-ledger 60200000 --end-ledger 60200005
```
