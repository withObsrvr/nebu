-- Migration: Create contract_events table
-- Description: Main table for storing Stellar contract invocation events
-- Schema version: v1.0
-- Created: 2025-12-17

-- =============================================================================
-- Main Events Table
-- =============================================================================

-- Drop existing table if schema needs to be recreated
-- This ensures clean migration even if table exists with wrong schema
DROP TABLE IF EXISTS contract_events CASCADE;

CREATE TABLE contract_events (
  -- Primary identification
  id BIGSERIAL PRIMARY KEY,

  -- Ledger metadata
  ledger_sequence BIGINT NOT NULL,
  ledger_closed_at TIMESTAMP NOT NULL,
  transaction_hash VARCHAR(64) NOT NULL,

  -- Event identification
  event_index INTEGER NOT NULL,
  contract_id VARCHAR(56) NOT NULL,

  -- Event data
  event_type VARCHAR(50),
  topics JSONB,
  data JSONB,

  -- Indexing metadata
  created_at TIMESTAMP DEFAULT NOW(),

  -- Ensure uniqueness per event
  UNIQUE (ledger_sequence, transaction_hash, event_index)
);

-- =============================================================================
-- Indexes for Query Performance
-- =============================================================================

-- Query by contract
CREATE INDEX IF NOT EXISTS idx_contract_events_contract_id
  ON contract_events(contract_id);

-- Query by ledger sequence (for range queries)
CREATE INDEX IF NOT EXISTS idx_contract_events_ledger_sequence
  ON contract_events(ledger_sequence DESC);

-- Query by timestamp (for time-based queries)
CREATE INDEX IF NOT EXISTS idx_contract_events_timestamp
  ON contract_events(ledger_closed_at DESC);

-- Query by transaction
CREATE INDEX IF NOT EXISTS idx_contract_events_transaction
  ON contract_events(transaction_hash);

-- Query by event type
CREATE INDEX IF NOT EXISTS idx_contract_events_type
  ON contract_events(event_type)
  WHERE event_type IS NOT NULL;

-- GIN index for JSONB queries (topics and data)
CREATE INDEX IF NOT EXISTS idx_contract_events_topics_gin
  ON contract_events USING GIN (topics);

CREATE INDEX IF NOT EXISTS idx_contract_events_data_gin
  ON contract_events USING GIN (data);

-- Composite index for contract + time range queries
CREATE INDEX IF NOT EXISTS idx_contract_events_contract_time
  ON contract_events(contract_id, ledger_closed_at DESC);

-- =============================================================================
-- Comments for Documentation
-- =============================================================================

COMMENT ON TABLE contract_events IS
  'Contract invocation events from Stellar blockchain. Each row represents a single contract event.';

COMMENT ON COLUMN contract_events.ledger_sequence IS
  'Stellar ledger sequence number when event occurred';

COMMENT ON COLUMN contract_events.ledger_closed_at IS
  'Timestamp when ledger closed (UTC)';

COMMENT ON COLUMN contract_events.transaction_hash IS
  'Stellar transaction hash that triggered the event';

COMMENT ON COLUMN contract_events.event_index IS
  'Index of event within transaction (0-based)';

COMMENT ON COLUMN contract_events.contract_id IS
  'Stellar contract address (C...)';

COMMENT ON COLUMN contract_events.topics IS
  'Event topics as JSONB array';

COMMENT ON COLUMN contract_events.data IS
  'Event data as JSONB object';

-- =============================================================================
-- Table Statistics (for query planner)
-- =============================================================================

-- Analyze table to update statistics
ANALYZE contract_events;
