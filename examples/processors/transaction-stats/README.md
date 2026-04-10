# transaction-stats

Example Nebu origin processor that aggregates successful/failed transaction and operation counts across the processed ledger range.

## Run

```bash
go run ./examples/processors/transaction-stats/cmd/transaction-stats --start-ledger 60200000 --end-ledger 60200009
```

Or from piped XDR:

```bash
nebu fetch 60200000 60200009 | go run ./examples/processors/transaction-stats/cmd/transaction-stats -
```
