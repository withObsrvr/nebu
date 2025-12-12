# DuckDB Sink Processor

A nebu sink processor that writes token transfer events to a DuckDB database for local analytics.

## What It Does

This sink processor consumes token transfer events and stores them in a local DuckDB database. DuckDB is an embedded analytical database (like SQLite but optimized for analytics) that makes it easy to query and analyze event data with SQL.

## Type

**Sink** - Consumes events and writes to DuckDB

## Features

- ✅ Automatic table creation
- ✅ SQL queryable storage
- ✅ Efficient columnar storage
- ✅ Local file-based (no server required)
- ✅ Perfect for local analytics and exploration

## Usage

### As a Library

```go
package main

import (
    "context"
    duckdb_sink "github.com/withObsrvr/nebu/examples/processors/duckdb-sink"
)

func main() {
    // Create sink
    sink, err := duckdb_sink.NewSink("events.db")
    if err != nil {
        panic(err)
    }
    defer sink.Close()

    // Consume events (as map[string]interface{})
    event := map[string]interface{}{
        "type": "transfer",
        "ledger_sequence": 60200000,
        "tx_hash": "abc123...",
        "from": "GABC...",
        "to": "GDEF...",
        "amount": "100.0",
        "asset": map[string]interface{}{
            "code": "USDC",
            "issuer": "GA5Z...",
        },
    }

    err = sink.ConsumeEvent(context.Background(), event)
    if err != nil {
        panic(err)
    }

    // Query the data
    results, _ := sink.Query("SELECT * FROM token_transfer_events WHERE asset_code = 'USDC'")
    fmt.Println(results)
}
```

### With nebu CLI (future)

```bash
# Stream events into DuckDB
nebu run origin token-transfer --start 60200000 --end 60200100 | nebu run sink duckdb --db events.db
```

## Querying Data

Once events are stored, you can query them with any DuckDB client:

```bash
# Using DuckDB CLI
duckdb events.db

# Example queries:
SELECT COUNT(*) FROM token_transfer_events;
SELECT asset_code, COUNT(*) as transfers FROM token_transfer_events GROUP BY asset_code;
SELECT * FROM token_transfer_events WHERE event_type = 'transfer' AND asset_code = 'USDC';
SELECT date_trunc('hour', timestamp) as hour, COUNT(*) FROM token_transfer_events GROUP BY hour;
```

## Schema

```sql
CREATE TABLE token_transfer_events (
    id INTEGER PRIMARY KEY,
    event_type VARCHAR NOT NULL,
    ledger_sequence INTEGER NOT NULL,
    tx_hash VARCHAR NOT NULL,
    contract_address VARCHAR,
    from_address VARCHAR,
    to_address VARCHAR,
    amount VARCHAR NOT NULL,
    asset_code VARCHAR,
    asset_issuer VARCHAR,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)
```

## Dependencies

DuckDB requires CGO. Use Nix for easy builds:

```bash
# Enter development environment with all dependencies
nix develop

# Build the sink
go build -o duckdb-sink ./cmd
```

Or install manually:
- Go 1.21+
- DuckDB C library
- CGO-enabled Go compiler
- pkg-config

```bash
# With dependencies installed:
CGO_ENABLED=1 go build -o duckdb-sink ./cmd
```

## Development

```bash
# With Nix (recommended)
nix develop
go build -o duckdb-sink ./cmd

# Run tests
go test ./...

# Build standalone binary
nix build
./result/bin/duckdb-sink --help
```

## Example: Full Pipeline

```go
// Create origin processor
src, _ := source.NewRPCLedgerSource("https://mainnet.sorobanrpc.com")
origin := token_transfer.NewOrigin(network.PublicNetworkPassphrase)

// Create sink
sink, _ := duckdb_sink.NewSink("events.db")
defer sink.Close()

// Run pipeline
rt := runtime.NewRuntime()
go func() {
    // Consume events from origin and write to sink
    for event := range origin.Out() {
        // Convert protobuf to map (simplified example)
        eventMap := convertToMap(event)
        sink.ConsumeEvent(ctx, eventMap)
    }
}()

rt.RunOrigin(ctx, src, origin, 60200000, 60200100)
```

## Why DuckDB?

- **Fast**: Columnar storage optimized for analytics
- **Embedded**: No server to manage, just a file
- **SQL**: Use familiar SQL to query events
- **Portable**: Single file database, easy to share
- **Analytics-ready**: Perfect for exploration and analysis

## Future Enhancements

- Batch inserts for better performance
- Partitioning by date/ledger
- Automatic schema evolution
- Export to Parquet
- Integration with BI tools
