#!/bin/bash
set -euo pipefail

# =============================================================================
# nebu Indexer Startup Script
# =============================================================================
# This script runs a contract events indexer pipeline on Fly.io
# Customize the pipeline for your specific needs

echo "========================================"
echo "Starting nebu Indexer"
echo "========================================"

# =============================================================================
# Configuration
# =============================================================================

# Required environment variables
: "${DATABASE_URL:?ERROR: DATABASE_URL not set}"
: "${RPC_URL:?ERROR: RPC_URL not set}"

# Optional environment variables with defaults
START_LEDGER="${START_LEDGER:-latest}"
BATCH_SIZE="${BATCH_SIZE:-1000}"
LOG_LEVEL="${LOG_LEVEL:-info}"

echo "Configuration:"
echo "  RPC URL: $RPC_URL"
echo "  Start Ledger: $START_LEDGER"
echo "  Batch Size: $BATCH_SIZE"
echo "  Log Level: $LOG_LEVEL"
echo "  Database: ${DATABASE_URL%%@*}@***"  # Hide password in logs
echo "========================================"

# =============================================================================
# Pre-flight Checks
# =============================================================================

echo "Running pre-flight checks..."

# Check if processors exist
if ! command -v contract-events &> /dev/null; then
    echo "ERROR: contract-events processor not found"
    exit 1
fi

if ! command -v postgres-sink &> /dev/null; then
    echo "ERROR: postgres-sink processor not found"
    exit 1
fi

# Test database connection
echo "Testing database connection..."
if ! psql "$DATABASE_URL" -c "SELECT 1" > /dev/null 2>&1; then
    echo "ERROR: Cannot connect to database"
    echo "Please check DATABASE_URL and ensure Postgres is running"
    exit 1
fi

echo "✓ Database connection successful"

# Test Stellar RPC connection
echo "Testing Stellar RPC connection..."
if ! curl -s -X POST "$RPC_URL" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":1,"method":"getLatestLedger","params":[]}' \
    | jq -e '.result.sequence' > /dev/null 2>&1; then
    echo "WARNING: Cannot connect to Stellar RPC"
    echo "Continuing anyway - will retry when pipeline starts"
fi

echo "✓ Pre-flight checks complete"
echo "========================================"

# =============================================================================
# Run Migrations (if needed)
# =============================================================================

if [ -d "/migrations" ] && [ "$(ls -A /migrations/*.sql 2>/dev/null)" ]; then
    echo "Running database migrations..."
    for migration in /migrations/*.sql; do
        echo "  Applying $(basename "$migration")..."
        psql "$DATABASE_URL" -f "$migration" || {
            echo "ERROR: Migration failed: $migration"
            exit 1
        }
    done
    echo "✓ Migrations complete"
    echo "========================================"
fi

# =============================================================================
# Handle Initial Backfill (if needed)
# =============================================================================

# Check if we need to run backfill (first-time setup)
if [ ! -f /data/backfill_complete ] && [ "$START_LEDGER" != "latest" ]; then
    echo "First-time setup detected - running backfill..."
    if [ -f /scripts/backfill.sh ]; then
        /scripts/backfill.sh || {
            echo "ERROR: Backfill failed"
            exit 1
        }
        touch /data/backfill_complete
        echo "✓ Backfill complete"
    else
        echo "WARNING: No backfill script found, skipping"
    fi
    echo "========================================"
fi

# =============================================================================
# Start Indexer Pipeline
# =============================================================================

echo "Starting indexer pipeline..."
echo ""
echo "Pipeline: contract-events → postgres-sink"
echo ""

# Get current ledger if START_LEDGER is "latest"
if [ "$START_LEDGER" = "latest" ]; then
    echo "Fetching latest ledger from Stellar RPC..."
    CURRENT_LEDGER=$(curl -s "$RPC_URL" \
        -X POST \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"getLatestLedger","params":[]}' \
        | jq -r '.result.sequence')

    if [ -z "$CURRENT_LEDGER" ] || [ "$CURRENT_LEDGER" = "null" ]; then
        echo "ERROR: Failed to fetch current ledger, defaulting to 60348494"
        CURRENT_LEDGER=60348494
    fi

    START_LEDGER=$CURRENT_LEDGER
    echo "Starting from ledger: $START_LEDGER"
fi

# =============================================================================
# Build Dynamic Pipeline
# =============================================================================
# Configure pipeline filters via PIPELINE_FILTERS environment variable
# Examples:
#   PIPELINE_FILTERS=""                          # No filters (default)
#   PIPELINE_FILTERS="dedup"                     # Add deduplication
#   PIPELINE_FILTERS="dedup|amount-filter"       # Multiple filters
#   PIPELINE_FILTERS="contract-filter"           # Filter by contract IDs
#   PIPELINE_FILTERS="jq:.data.amount>1000000"   # Custom jq filter
#
# Available filters:
#   - dedup: Remove duplicate events
#   - amount-filter: Filter by amount (set MIN_AMOUNT env var)
#   - contract-filter: Filter by contract IDs (set CONTRACT_IDS env var)
#   - jq:<expression>: Custom jq filter expression

PIPELINE_FILTERS="${PIPELINE_FILTERS:-}"

echo "Building pipeline..."
echo "  Origin: contract-events"

# Build the filter chain
FILTER_CHAIN=""
if [ -n "$PIPELINE_FILTERS" ]; then
    IFS='|' read -ra FILTERS <<< "$PIPELINE_FILTERS"
    for filter in "${FILTERS[@]}"; do
        filter=$(echo "$filter" | xargs)  # Trim whitespace

        if [ "$filter" = "dedup" ]; then
            echo "  Filter: dedup (remove duplicates)"
            FILTER_CHAIN="$FILTER_CHAIN | dedup"

        elif [ "$filter" = "amount-filter" ]; then
            MIN_AMOUNT="${MIN_AMOUNT:-1000000}"
            echo "  Filter: amount-filter (min: $MIN_AMOUNT)"
            FILTER_CHAIN="$FILTER_CHAIN | amount-filter --min-amount $MIN_AMOUNT"

        elif [ "$filter" = "contract-filter" ]; then
            if [ -z "${CONTRACT_IDS:-}" ]; then
                echo "  ERROR: contract-filter requires CONTRACT_IDS environment variable"
                exit 1
            fi
            echo "  Filter: contract-filter (contracts: $CONTRACT_IDS)"
            export CONTRACT_IDS
            FILTER_CHAIN="$FILTER_CHAIN | /scripts/contract-filter.sh"

        elif [[ "$filter" == jq:* ]]; then
            JQ_EXPR="${filter#jq:}"
            echo "  Filter: jq ($JQ_EXPR)"
            export JQ_FILTER="$JQ_EXPR"
            FILTER_CHAIN="$FILTER_CHAIN | /scripts/jq-filter.sh"

        else
            echo "  WARNING: Unknown filter '$filter', skipping"
        fi
    done
fi

echo "  Sink: postgres-sink (table: contract_events)"
echo ""

# Execute the pipeline
# shellcheck disable=SC2086
eval "contract-events \
    --start-ledger \"$START_LEDGER\" \
    --rpc-url \"$RPC_URL\" \
    $FILTER_CHAIN \
    | postgres-sink \
        --dsn \"$DATABASE_URL\" \
        --table contract_events \
        --batch-size \"$BATCH_SIZE\""

# If we reach here, the pipeline exited (shouldn't happen with --follow)
echo "ERROR: Pipeline exited unexpectedly"
exit 1
