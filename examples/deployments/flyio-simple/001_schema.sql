-- Migration: Create contract_events table
-- Simple schema for storing Stellar contract invocation events

-- Main events table
CREATE TABLE IF NOT EXISTS contract_events (
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

-- Indexes for query performance

-- Query by contract
CREATE INDEX IF NOT EXISTS idx_contract_events_contract_id
  ON contract_events(contract_id);

-- Query by ledger sequence (for range queries)
CREATE INDEX IF NOT EXISTS idx_contract_events_ledger_sequence
  ON contract_events(ledger_sequence DESC);

-- Query by timestamp
CREATE INDEX IF NOT EXISTS idx_contract_events_timestamp
  ON contract_events(ledger_closed_at DESC);

-- Query event type
CREATE INDEX IF NOT EXISTS idx_contract_events_event_type
  ON contract_events(event_type);

-- JSONB search indexes
CREATE INDEX IF NOT EXISTS idx_contract_events_topics_gin
  ON contract_events USING GIN (topics);

CREATE INDEX IF NOT EXISTS idx_contract_events_data_gin
  ON contract_events USING GIN (data);
