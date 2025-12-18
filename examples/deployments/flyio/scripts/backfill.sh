#!/bin/bash
set -euo pipefail

# =============================================================================
# Historical Backfill Script for nebu Indexer
# =============================================================================
# This script backfills historical contract events from a starting ledger
# to the current ledger. It processes in chunks to handle large ranges.
#
# Usage:
#   backfill.sh [START_LEDGER] [END_LEDGER]
#
# Examples:
#   backfill.sh 60000000          # Backfill from ledger 60M to current
#   backfill.sh 60000000 61000000 # Backfill specific range

echo "========================================"
echo "nebu Historical Backfill"
echo "========================================"

# =============================================================================
# Configuration
# =============================================================================

# Required environment variables
: "${DATABASE_URL:?ERROR: DATABASE_URL not set}"
: "${RPC_URL:?ERROR: RPC_URL not set}"

# Backfill parameters
START_LEDGER="${1:-${START_LEDGER:-}}"
END_LEDGER="${2:-latest}"
CHUNK_SIZE="${CHUNK_SIZE:-10000}"  # Process 10k ledgers at a time
BATCH_SIZE="${BATCH_SIZE:-5000}"   # Larger batches for backfill performance

# Validate start ledger
if [ -z "$START_LEDGER" ] || [ "$START_LEDGER" = "latest" ]; then
    echo "ERROR: START_LEDGER must be a specific ledger number for backfill"
    echo "Usage: $0 <start_ledger> [end_ledger]"
    echo "Example: $0 60000000"
    exit 1
fi

echo "Backfill Configuration:"
echo "  Start Ledger: $START_LEDGER"
echo "  End Ledger: $END_LEDGER"
echo "  Chunk Size: $CHUNK_SIZE ledgers"
echo "  Batch Size: $BATCH_SIZE events"
echo "========================================"

# =============================================================================
# Get Current Ledger
# =============================================================================

if [ "$END_LEDGER" = "latest" ]; then
    echo "Fetching current ledger from Stellar RPC..."
    CURRENT_LEDGER=$(curl -s -X POST "$RPC_URL" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"getLatestLedger","params":[]}' \
        | jq -r '.result.sequence')

    if [ -z "$CURRENT_LEDGER" ] || [ "$CURRENT_LEDGER" = "null" ]; then
        echo "ERROR: Failed to fetch current ledger from RPC"
        exit 1
    fi

    END_LEDGER=$CURRENT_LEDGER
    echo "✓ Current ledger: $END_LEDGER"
fi

# =============================================================================
# Calculate Progress
# =============================================================================

TOTAL_LEDGERS=$((END_LEDGER - START_LEDGER))
TOTAL_CHUNKS=$(( (TOTAL_LEDGERS + CHUNK_SIZE - 1) / CHUNK_SIZE ))

echo "========================================"
echo "Backfill Plan:"
echo "  Total Ledgers: $TOTAL_LEDGERS"
echo "  Total Chunks: $TOTAL_CHUNKS"
echo "  Estimated Time: ~$((TOTAL_CHUNKS * 2)) minutes (assuming 2min/chunk)"
echo "========================================"

# Confirm before proceeding
if [ "${SKIP_CONFIRMATION:-}" != "true" ]; then
    echo ""
    read -p "Proceed with backfill? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Backfill cancelled"
        exit 0
    fi
fi

# =============================================================================
# Process Chunks
# =============================================================================

CHUNK_NUM=0
CURRENT_START=$START_LEDGER

while [ $CURRENT_START -lt $END_LEDGER ]; do
    CHUNK_NUM=$((CHUNK_NUM + 1))
    CHUNK_END=$((CURRENT_START + CHUNK_SIZE))

    if [ $CHUNK_END -gt $END_LEDGER ]; then
        CHUNK_END=$END_LEDGER
    fi

    CHUNK_LEDGERS=$((CHUNK_END - CURRENT_START))
    PROGRESS=$((CHUNK_NUM * 100 / TOTAL_CHUNKS))

    echo ""
    echo "========================================"
    echo "Chunk $CHUNK_NUM/$TOTAL_CHUNKS ($PROGRESS%)"
    echo "Ledgers: $CURRENT_START → $CHUNK_END ($CHUNK_LEDGERS ledgers)"
    echo "========================================"

    # Run the backfill for this chunk
    START_TIME=$(date +%s)

    contract-events \
        --start-ledger "$CURRENT_START" \
        --end-ledger "$CHUNK_END" \
        --rpc-url "$RPC_URL" \
        | postgres-sink \
            --table contract_events \
            --database-url "$DATABASE_URL" \
            --batch-size "$BATCH_SIZE" \
            --upsert-key "ledger_sequence,event_index" \
        || {
            echo "ERROR: Chunk $CHUNK_NUM failed (ledgers $CURRENT_START-$CHUNK_END)"
            echo "You can resume from ledger $CURRENT_START"
            exit 1
        }

    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))

    echo "✓ Chunk $CHUNK_NUM complete in ${DURATION}s"

    # Save checkpoint
    echo "$CHUNK_END" > /data/backfill_checkpoint.txt

    # Move to next chunk
    CURRENT_START=$CHUNK_END
done

# =============================================================================
# Verify Backfill
# =============================================================================

echo ""
echo "========================================"
echo "Backfill Complete!"
echo "========================================"

# Count total events
echo "Verifying results..."
EVENT_COUNT=$(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM contract_events" | tr -d ' ')
echo "✓ Total events in database: $EVENT_COUNT"

# Show ledger range
LEDGER_RANGE=$(psql "$DATABASE_URL" -t -c \
    "SELECT MIN(ledger_sequence), MAX(ledger_sequence) FROM contract_events" \
    | tr -d ' ')
echo "✓ Ledger range: $LEDGER_RANGE"

echo ""
echo "Backfill successful!"
echo "You can now start the live indexer with --start-ledger latest"
echo "========================================"

# Mark backfill as complete
touch /data/backfill_complete
