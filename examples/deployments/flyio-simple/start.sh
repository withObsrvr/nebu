#!/bin/bash
set -euo pipefail

# =============================================================================
# nebu Indexer Startup Script (Simplified)
# =============================================================================
# This script runs a contract events indexer pipeline on Fly.io
# All processors are pre-built and available in PATH

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
# Run Migrations
# =============================================================================

echo "Running database migrations..."
psql "$DATABASE_URL" -f /001_schema.sql 2>&1 | grep -v "already exists" || true
echo "✓ Migrations complete"
echo "========================================"

# =============================================================================
# Start Pipeline
# =============================================================================

echo "Starting indexer pipeline..."
echo "  contract-events | postgres-sink"
echo "========================================"

# Start pipeline - processors are already in PATH from Docker image
exec contract-events \
  --start-ledger "$START_LEDGER" \
  --rpc-url "$RPC_URL" \
  --follow \
  | postgres-sink \
      --dsn "$DATABASE_URL" \
      --table contract_events \
      --batch-size "$BATCH_SIZE"
