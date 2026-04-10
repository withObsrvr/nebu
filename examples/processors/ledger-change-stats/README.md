# ledger-change-stats

Example Nebu origin processor that emits one JSON event per ledger summarizing created / updated / deleted ledger entry changes plus fee / transaction / operation-caused change counts.

## Run

```bash
go run ./examples/processors/ledger-change-stats/cmd/ledger-change-stats --start-ledger 60200000 --end-ledger 60200005
```

Or from piped XDR:

```bash
nebu fetch 60200000 60200005 | go run ./examples/processors/ledger-change-stats/cmd/ledger-change-stats -
```
