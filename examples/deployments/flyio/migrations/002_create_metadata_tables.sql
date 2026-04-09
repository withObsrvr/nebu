-- Migration: Create metadata tables and helper views
-- Description: Optional metadata and extracted views for contract-events JSONB storage
-- Schema version: v2.0

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
  deployment_ledger BIGINT,
  deployment_timestamp TIMESTAMP,
  is_verified BOOLEAN DEFAULT false,
  is_active BOOLEAN DEFAULT true,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_contracts_active
  ON contracts(is_active)
  WHERE is_active = true;

CREATE INDEX IF NOT EXISTS idx_contracts_verified
  ON contracts(is_verified)
  WHERE is_verified = true;

CREATE TABLE event_types (
  id SERIAL PRIMARY KEY,
  contract_id VARCHAR(56) NOT NULL REFERENCES contracts(contract_id),
  event_type VARCHAR(50) NOT NULL,
  display_name VARCHAR(100),
  description TEXT,
  topics_schema JSONB,
  data_schema JSONB,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE (contract_id, event_type)
);

CREATE INDEX IF NOT EXISTS idx_event_types_contract
  ON event_types(contract_id);

CREATE TABLE indexing_stats (
  id SERIAL PRIMARY KEY,
  stat_date DATE NOT NULL DEFAULT CURRENT_DATE,
  latest_ledger_sequence BIGINT NOT NULL,
  latest_ledger_timestamp TIMESTAMP NOT NULL,
  total_events BIGINT NOT NULL,
  events_last_24h BIGINT,
  avg_events_per_ledger NUMERIC(10,2),
  avg_processing_time_ms NUMERIC(10,2),
  created_at TIMESTAMP DEFAULT NOW(),
  UNIQUE (stat_date)
);

CREATE INDEX IF NOT EXISTS idx_indexing_stats_date
  ON indexing_stats(stat_date DESC);

CREATE OR REPLACE VIEW recent_events AS
SELECT
  e.id,
  (e.data->>'ledgerSequence')::BIGINT AS ledger_sequence,
  to_timestamp((e.data->>'timestamp')::BIGINT) AS ledger_closed_at,
  e.data->>'transactionHash' AS transaction_hash,
  (e.data->>'eventIndex')::INTEGER AS event_index,
  e.data->>'contractId' AS contract_id,
  c.name AS contract_name,
  c.symbol AS contract_symbol,
  e.event_type,
  et.display_name AS event_display_name,
  e.data->'topicDecoded' AS topics,
  e.data->'dataDecoded' AS data
FROM contract_events e
LEFT JOIN contracts c ON e.data->>'contractId' = c.contract_id
LEFT JOIN event_types et ON e.data->>'contractId' = et.contract_id AND e.event_type = et.event_type
ORDER BY (e.data->>'ledgerSequence')::BIGINT DESC
LIMIT 1000;

CREATE OR REPLACE VIEW event_counts_by_contract AS
SELECT
  e.data->>'contractId' AS contract_id,
  c.name AS contract_name,
  c.symbol AS contract_symbol,
  COUNT(*) AS total_events,
  COUNT(DISTINCT e.data->>'transactionHash') AS total_transactions,
  MIN(to_timestamp((e.data->>'timestamp')::BIGINT)) AS first_event_at,
  MAX(to_timestamp((e.data->>'timestamp')::BIGINT)) AS last_event_at
FROM contract_events e
LEFT JOIN contracts c ON e.data->>'contractId' = c.contract_id
GROUP BY e.data->>'contractId', c.name, c.symbol
ORDER BY total_events DESC;
