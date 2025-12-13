# Contract Events Processor

Extract all contract events from Stellar ledgers.

## Overview

The contract-events processor is an **origin processor** that extracts ALL contract events from Stellar ledgers, including:

- Token transfers (native, issued assets, Soroban tokens)
- Contract fee events
- Swaps, deposits, withdrawals
- Any custom contract event

Unlike the `token-transfer` processor which focuses on SEP-41 token events, `contract-events` captures all contract activity on the network.

## Features

- ✅ Extracts all contract events from ledgers
- ✅ Decodes ScVal topics and data to JSON
- ✅ Auto-detects common event types (transfer, mint, swap, etc.)
- ✅ Includes diagnostic events for debugging
- ✅ Supports both operation-level and transaction-level events
- ✅ V3/V4 ledger format compatibility via Stellar SDK
- ✅ Tracks successful and failed transactions
- ✅ Full network passphrase support (mainnet/testnet)

## Installation

```bash
# Install globally
go install github.com/withObsrvr/nebu/examples/processors/contract-events/cmd/contract-events@latest

# Or use nebu install
nebu install contract-events
```

## Usage

### Basic Usage

```bash
# Process ledger range
contract-events --start-ledger 60200000 --end-ledger 60200100

# Stream continuously (unbounded)
contract-events --start-ledger 60269740

# Read from stdin
nebu fetch 60200000 60200100 | contract-events
```

### With nebu CLI

```bash
# Using nebu run (once added to registry)
nebu run origin contract-events --start-ledger 60269740 --end-ledger 60269742

# Pipe to transforms
nebu run origin contract-events --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.event_type == "swap")' | \
  json-file-sink --out swaps.jsonl
```

### Filter by Event Type

```bash
# Filter for transfer events only
contract-events --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.event_type == "transfer")'

# Filter for specific contract
contract-events --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.contract_id == "CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA")'

# Filter for successful transactions only
contract-events --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.in_successful_tx == true)'
```

## Output Schema

Each event is output as a JSON object with the following fields:

```json
{
  "_schema": "nebu.contract-events.v1",
  "_nebu_version": "dev",
  "timestamp": 1765560602,
  "ledger_sequence": 60269740,
  "transaction_hash": "20287f293c7a3cacf2e471bf8495963c52b3dfda695ec51d256abe9e04024b91",
  "contract_id": "CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA",
  "type": "contract",
  "event_type": "transfer",
  "topic_decoded": [
    "transfer",
    "GAEEMKYQGU6XGQP74NVVJI7JHSY6DRPRGMKRZM3XSLGTA3VJJGVZFACH",
    "GDQPT65VSMG7PRBUV2GHPANXGNMOAHMUSDMNIIWE5QIVERCOX36WFYXS",
    "USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
  ],
  "data_decoded": "0x0000000000000000000000000000000000000000000000000000000066ead216",
  "in_successful_tx": true,
  "event_index": 2,
  "operation_index": 0,
  "network_passphrase": "Public Global Stellar Network ; September 2015"
}
```

### Schema Fields

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | int64 | Ledger close time (Unix timestamp) |
| `ledger_sequence` | uint32 | Ledger sequence number |
| `transaction_hash` | string | Transaction hash |
| `contract_id` | string | Contract ID (strkey encoded) |
| `type` | string | Event type: "contract", "system", or "diagnostic" |
| `event_type` | string | Detected event type from topics (e.g., "transfer", "swap") |
| `topic_decoded` | array | Decoded topic values as JSON |
| `data_decoded` | any | Decoded event data as JSON |
| `in_successful_tx` | bool | Whether the transaction succeeded |
| `event_index` | int | Event index within the transaction |
| `operation_index` | int | Operation index (-1 for transaction-level events) |
| `diagnostic_events` | array | Diagnostic events (if any) |
| `network_passphrase` | string | Network identifier |

### Detected Event Types

The processor auto-detects common event types from the first topic:

- `transfer` - Asset transfer between accounts
- `mint` - New token issuance
- `burn` - Token destruction
- `swap` - Token swap/trade
- `deposit` - Liquidity deposit
- `withdraw` - Liquidity withdrawal
- `stake` - Staking operation
- `unstake` - Unstaking operation
- `claim` - Reward claim
- `approval` - Token approval
- `fee` - Fee collection
- And more...

If the event type isn't recognized, the first symbol topic is used as the event type.

## Input Modes

### 1. RPC Mode (Default)

Fetch ledgers from Stellar RPC:

```bash
contract-events --start-ledger 60200000 --end-ledger 60200100
```

**Flags:**
- `--rpc-url` - RPC endpoint (default: mainnet)
- `--network` - Network passphrase ("mainnet" or "testnet")
- `--start-ledger` - Start ledger sequence (required)
- `--end-ledger` - End ledger sequence (0 for unbounded)

### 2. stdin Mode

Read XDR ledgers from stdin:

```bash
cat ledgers.xdr | contract-events
```

### 3. File Mode

Read XDR ledgers from file:

```bash
contract-events ledgers.xdr
```

## Environment Variables

- `NEBU_RPC_URL` - Override RPC endpoint
- `NEBU_NETWORK` - Override network ("mainnet", "testnet", or full passphrase)
- `NEBU_RPC_AUTH` - Authentication header (e.g., "Api-Key YOUR_KEY")

## Examples

### Find All Swap Events

```bash
contract-events --start-ledger 60200000 --end-ledger 60200100 | \
  jq 'select(.event_type == "swap")' | \
  jq '{contract: .contract_id, topics: .topic_decoded, data: .data_decoded}'
```

### Track Contract Activity

```bash
# Monitor a specific contract
contract-events --start-ledger 60269740 | \
  jq 'select(.contract_id == "CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA")'
```

### Save to Database

```bash
contract-events --start-ledger 60200000 --end-ledger 60200100 | \
  your-database-sink
```

## Comparison with token-transfer

| Feature | contract-events | token-transfer |
|---------|----------------|----------------|
| Event Coverage | All contract events | SEP-41 token events only |
| Event Types | transfer, mint, swap, fee, custom, etc. | transfer, mint, burn, clawback, fee |
| Output Format | Generic contract event | Token-specific event |
| Use Case | General contract monitoring | Token-focused indexing |
| Performance | Captures everything | Optimized for tokens |

**When to use contract-events:**
- Monitoring all contract activity
- DEX/swap tracking
- Custom contract events
- General-purpose indexing

**When to use token-transfer:**
- Token-focused indexing
- SEP-41 compliant transfers only
- Optimized token event processing

## Performance

The processor:
- Streams ledgers efficiently with buffered channels
- Only processes Soroban transactions (skips non-contract txs)
- Decodes ScVals lazily
- Supports unbounded streaming for real-time monitoring

## Development

```bash
# Build
go build -o bin/contract-events ./cmd/contract-events

# Run tests
go test ./...

# Install locally
go install ./cmd/contract-events
```

## License

Apache 2.0

## Maintainer

OBSRVR - https://withobsrvr.com
