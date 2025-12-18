#!/bin/bash
# contract-filter: Transform processor that filters events by contract ID
# Reads protobuf messages from stdin, filters by contract_id, writes to stdout
#
# Usage:
#   CONTRACT_IDS="CA123...,CB456..." contract-events | contract-filter.sh | postgres-sink
#
# Environment variables:
#   CONTRACT_IDS - Comma-separated list of contract IDs to include (required)

set -euo pipefail

if [ -z "${CONTRACT_IDS:-}" ]; then
    echo "ERROR: CONTRACT_IDS environment variable required" >&2
    echo "Example: CONTRACT_IDS=\"CA123...,CB456...\"" >&2
    exit 1
fi

# Convert comma-separated list to jq filter
# Example: "CA123,CB456" -> '.contract_id == "CA123" or .contract_id == "CB456"'
IFS=',' read -ra IDS <<< "$CONTRACT_IDS"

# Build jq filter expression
JQ_FILTER=""
for contract_id in "${IDS[@]}"; do
    # Trim whitespace
    contract_id=$(echo "$contract_id" | xargs)

    if [ -z "$JQ_FILTER" ]; then
        JQ_FILTER=".contract_id == \"$contract_id\""
    else
        JQ_FILTER="$JQ_FILTER or .contract_id == \"$contract_id\""
    fi
done

# Apply filter to each line
# Note: This assumes line-delimited JSON input
# For actual protobuf, you'd need a converter, but this works for simple cases
while IFS= read -r line; do
    echo "$line" | jq -c "select($JQ_FILTER)" || true
done
