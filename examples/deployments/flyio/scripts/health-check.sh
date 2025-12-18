#!/bin/bash
# =============================================================================
# Health Check Script for nebu Indexer
# =============================================================================
# This script verifies the indexer is running correctly
# Used by Docker HEALTHCHECK and Fly.io health checks

set -euo pipefail

# Check if contract-events process is running
if ! pgrep -f "contract-events" > /dev/null 2>&1; then
    echo "ERROR: contract-events process not running"
    exit 1
fi

# Check if postgres-sink process is running
if ! pgrep -f "postgres-sink" > /dev/null 2>&1; then
    echo "ERROR: postgres-sink process not running"
    exit 1
fi

# Optional: Check database connectivity
if [ -n "${DATABASE_URL:-}" ]; then
    if ! psql "$DATABASE_URL" -c "SELECT 1" > /dev/null 2>&1; then
        echo "ERROR: Cannot connect to database"
        exit 1
    fi
fi

# Optional: Check recent activity (verify events are being written)
# Uncomment to enable:
#
# if [ -n "${DATABASE_URL:-}" ]; then
#     # Check if any events were written in the last 5 minutes
#     RECENT_COUNT=$(psql "$DATABASE_URL" -t -c \
#         "SELECT COUNT(*) FROM contract_events WHERE created_at > NOW() - INTERVAL '5 minutes'" \
#         2>/dev/null | tr -d ' ')
#
#     if [ "$RECENT_COUNT" = "0" ]; then
#         echo "WARNING: No events written in last 5 minutes"
#         # Don't fail - might be low activity period
#     fi
# fi

echo "Health check passed"
exit 0
