-- Migration: Create contract_events table
-- Simple JSONB schema for storing contract-events output via postgres-sink

CREATE TABLE IF NOT EXISTS contract_events (
  id BIGINT PRIMARY KEY,
  event_type TEXT,
  data JSONB NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_contract_events_data_gin
  ON contract_events USING GIN (data);

CREATE INDEX IF NOT EXISTS idx_contract_events_event_type
  ON contract_events(event_type)
  WHERE event_type IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_contract_events_created_at
  ON contract_events(created_at DESC);
