#!/usr/bin/env bash

# Extract business fields from carbon sink contract invocations
# This demonstrates field extraction similar to flowctl's ContractInvocationExtractor

set -e

CONTRACT_ID="${1:-CASJKXVOKEBFC6HRNLLZKMEFJXYS3S5GOXM5DQRD7NDPIOQHCPAOLH7O}"
FUNCTION_NAME="${2:-sink_carbon}"

echo "Extracting fields from contract: $CONTRACT_ID, function: $FUNCTION_NAME" >&2

jq --arg contract "$CONTRACT_ID" \
   --arg function "$FUNCTION_NAME" '
  # Filter to specific contract and function
  select(.contractId == $contract and .functionName == $function) |

  # Generate TOID (Transaction Operation ID) like Stellar
  # Formula: (ledger << 32) | (tx << 12) | op
  {
    toid: ((.meta.ledgerSequence * 4294967296) +
           (.meta.transactionIndex * 4096) +
           .meta.operationIndex),

    # Core metadata
    ledger: .meta.ledgerSequence,
    timestamp: (.meta.closedAtUnix | tonumber | strftime("%Y-%m-%dT%H:%M:%SZ")),
    tx_hash: .meta.txHash,
    contract_id: .contractId,
    function_name: .functionName,
    invoking_account: .invokingAccount,
    schema_name: "carbon_sink_v2",
    successful: .successful,
    in_successful_tx: .meta.inSuccessfulTx,

    # Extract business fields from arguments
    # Assuming schema:
    #   arg[0] = funder (address)
    #   arg[1] = recipient (address)
    #   arg[2] = amount (uint64)
    #   arg[3] = project_id (string)
    #   arg[4] = memo_text (optional string)
    #   arg[5] = email (optional string)

    funder: (try (.arguments[0] | fromjson) catch ""),
    recipient: (try (.arguments[1] | fromjson) catch ""),
    amount: (try (.arguments[2] | fromjson | tonumber) catch 0),
    project_id: (try (.arguments[3] | fromjson) catch ""),
    memo_text: (try (.arguments[4] | fromjson) catch ""),
    email: (try (.arguments[5] | fromjson) catch ""),

    # Validation flags (can filter on these)
    valid_funder: (try (.arguments[0] | fromjson | test("^G[A-Z2-7]{55}$")) catch false),
    valid_recipient: (try (.arguments[1] | fromjson | test("^G[A-Z2-7]{55}$")) catch false),
    valid_amount: (try (.arguments[2] | fromjson | tonumber > 0) catch false),

    # Processing metadata
    processed_at: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
  }
'
