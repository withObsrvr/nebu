#!/bin/bash
# jq-filter: Transform processor that filters events using jq
# Reads protobuf messages from stdin, applies jq filter, writes to stdout
#
# Usage:
#   contract-events | jq-filter.sh '.data.amount > 1000000' | postgres-sink
#
# Environment variables:
#   JQ_FILTER - The jq filter expression to apply (required)

set -euo pipefail

JQ_FILTER="${JQ_FILTER:-${1:-.}}"

if [ "$JQ_FILTER" = "." ]; then
    # No filter, pass through
    cat
else
    # Apply jq filter to each line
    # Note: This assumes line-delimited JSON input
    # For actual protobuf, you'd need a converter, but this works for simple cases
    while IFS= read -r line; do
        echo "$line" | jq -c "select($JQ_FILTER)" || true
    done
fi
