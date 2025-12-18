-- Migration: Create metadata tables
-- Description: Optional tables for contract and event metadata
-- Schema version: v1.0
-- Created: 2025-12-17

-- =============================================================================
-- Contracts Metadata Table
-- =============================================================================

-- Drop existing tables if schema needs to be recreated
DROP TABLE IF EXISTS event_types CASCADE;
DROP TABLE IF EXISTS contracts CASCADE;
DROP TABLE IF EXISTS indexing_stats CASCADE;
DROP VIEW IF EXISTS recent_events CASCADE;
DROP VIEW IF EXISTS event_counts_by_contract CASCADE;

CREATE TABLE contracts (
  contract_id VARCHAR(56) PRIMARY KEY,
  name VARCHAR(100),
  symbol VARCHAR(20),
  description TEXT,
  website_url VARCHAR(500),

  -- Contract metadata
  deployment_ledger BIGINT,
  deployment_timestamp TIMESTAMP,

  -- Flags
  is_verified BOOLEAN DEFAULT false,
  is_active BOOLEAN DEFAULT true,

  -- Timestamps
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

-- Index for active contracts
CREATE INDEX IF NOT EXISTS idx_contracts_active
  ON contracts(is_active)
  WHERE is_active = true;

-- Index for verified contracts
CREATE INDEX IF NOT EXISTS idx_contracts_verified
  ON contracts(is_verified)
  WHERE is_verified = true;

COMMENT ON TABLE contracts IS
  'Metadata for Stellar contracts. Manually populated or synced from external source.';

-- =============================================================================
-- Event Types Metadata Table (Optional)
-- =============================================================================

CREATE TABLE event_types (
  id SERIAL PRIMARY KEY,
  contract_id VARCHAR(56) NOT NULL REFERENCES contracts(contract_id),
  event_type VARCHAR(50) NOT NULL,

  -- Human-readable information
  display_name VARCHAR(100),
  description TEXT,

  -- Schema information
  topics_schema JSONB,
  data_schema JSONB,

  -- Timestamps
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),

  UNIQUE (contract_id, event_type)
);

-- Index for looking up event types by contract
CREATE INDEX IF NOT EXISTS idx_event_types_contract
  ON event_types(contract_id);

COMMENT ON TABLE event_types IS
  'Metadata for contract event types. Helps with event interpretation and display.';

-- =============================================================================
-- Indexing Statistics Table
-- =============================================================================

CREATE TABLE indexing_stats (
  id SERIAL PRIMARY KEY,

  -- Stats snapshot
  stat_date DATE NOT NULL DEFAULT CURRENT_DATE,

  -- Ledger progress
  latest_ledger_sequence BIGINT NOT NULL,
  latest_ledger_timestamp TIMESTAMP NOT NULL,

  -- Event counts
  total_events BIGINT NOT NULL,
  events_last_24h BIGINT,

  -- Performance metrics
  avg_events_per_ledger NUMERIC(10,2),
  avg_processing_time_ms NUMERIC(10,2),

  -- Metadata
  created_at TIMESTAMP DEFAULT NOW(),

  UNIQUE (stat_date)
);

CREATE INDEX IF NOT EXISTS idx_indexing_stats_date
  ON indexing_stats(stat_date DESC);

COMMENT ON TABLE indexing_stats IS
  'Daily statistics for indexer health monitoring.';

-- =============================================================================
-- Helper Views
-- =============================================================================

-- View: Recent events with contract metadata
CREATE OR REPLACE VIEW recent_events AS
SELECT
  e.id,
  e.ledger_sequence,
  e.ledger_closed_at,
  e.transaction_hash,
  e.event_index,
  e.contract_id,
  c.name AS contract_name,
  c.symbol AS contract_symbol,
  e.event_type,
  et.display_name AS event_display_name,
  e.topics,
  e.data
FROM contract_events e
LEFT JOIN contracts c ON e.contract_id = c.contract_id
LEFT JOIN event_types et ON e.contract_id = et.contract_id AND e.event_type = et.event_type
ORDER BY e.ledger_sequence DESC
LIMIT 1000;

COMMENT ON VIEW recent_events IS
  'Most recent 1000 events with enriched contract metadata.';

-- View: Event counts by contract
CREATE OR REPLACE VIEW event_counts_by_contract AS
SELECT
  e.contract_id,
  c.name AS contract_name,
  c.symbol AS contract_symbol,
  COUNT(*) AS total_events,
  COUNT(DISTINCT e.transaction_hash) AS total_transactions,
  MIN(e.ledger_closed_at) AS first_event_at,
  MAX(e.ledger_closed_at) AS last_event_at
FROM contract_events e
LEFT JOIN contracts c ON e.contract_id = c.contract_id
GROUP BY e.contract_id, c.name, c.symbol
ORDER BY total_events DESC;

COMMENT ON VIEW event_counts_by_contract IS
  'Aggregated event statistics per contract.';

-- =============================================================================
-- Sample Data (Optional)
-- =============================================================================

-- You can add known contracts here for your deployment
-- Example:
-- INSERT INTO contracts (contract_id, name, symbol, description) VALUES
--   ('CXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX', 'Example Contract', 'EXM', 'Example contract description')
-- ON CONFLICT (contract_id) DO NOTHING;
