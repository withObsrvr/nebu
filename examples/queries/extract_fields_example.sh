#!/usr/bin/env bash

# Simple example of extracting business fields from contract invocations
# This demonstrates the concept of a nebu transformer

set -e

# Extract and parse fields from "work" function invocations
jq -c '
  # Generate TOID (Transaction Operation ID)
  {
    toid: ((.meta.ledgerSequence * 4294967296) + (.meta.transactionIndex * 4096) + .meta.operationIndex),

    # Core fields
    ledger: .meta.ledgerSequence,
    timestamp: .meta.closedAtUnix,
    tx_hash: .meta.txHash[0:16],
    contract_id: .contractId[0:16],
    function_name: .functionName,
    invoking_account: .invokingAccount[0:12],
    successful: .successful,
    in_successful_tx: .meta.inSuccessfulTx,

    # Extracted business fields
    # Parse arguments - they are JSON strings that need to be decoded
    arg0_raw: .arguments[0],
    arg1_raw: .arguments[1],
    arg2_raw: .arguments[2],

    # For work function:
    # arg0 = account address
    # arg1 = hash (hex string)
    # arg2 = number
    account: (try (.arguments[0] | fromjson) catch null),
    hash: (try (.arguments[1] | fromjson) catch null),
    value: (try (.arguments[2] | fromjson) catch null),

    # Number of arguments
    num_args: (.arguments | length),
    num_events: (.diagnosticEvents | length),
    num_state_changes: (.stateChanges | length)
  }
'
