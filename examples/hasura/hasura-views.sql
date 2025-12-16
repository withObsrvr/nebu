-- ============================================================================
-- Hasura Views for nebu postgres-sink
-- ============================================================================
-- This file creates optimized views for all nebu processors, making JSONB
-- data clean and queryable via Hasura GraphQL.
--
-- Usage:
--   psql -U <your_username> -d postgres < hasura-views.sql
-- ============================================================================

-- ============================================================================
-- TOKEN TRANSFERS
-- ============================================================================
-- Note: All views use 'toid' (Total Order ID) as the primary identifier.
-- TOIDs are SEP-35 compliant unique identifiers that combine ledger sequence,
-- transaction index, and operation index into a single sortable integer.

-- All token transfer events (transfers, mints, burns, fees)
CREATE OR REPLACE VIEW token_transfer_events AS
SELECT
  id as toid,
  event_type,
  -- Transfer fields
  CASE WHEN data->'transfer' IS NOT NULL THEN data->'transfer'->>'from' END as from_address,
  CASE WHEN data->'transfer' IS NOT NULL THEN data->'transfer'->>'to' END as to_address,
  -- Mint fields
  CASE WHEN data->'mint' IS NOT NULL THEN data->'mint'->>'to' END as mint_to,
  -- Burn fields
  CASE WHEN data->'burn' IS NOT NULL THEN data->'burn'->>'from' END as burn_from,
  -- Fee fields
  CASE WHEN data->'fee' IS NOT NULL THEN data->'fee'->>'from' END as fee_from,
  -- Common fields
  COALESCE(
    (data->'transfer'->>'amount')::numeric,
    (data->'mint'->>'amount')::numeric,
    (data->'burn'->>'amount')::numeric,
    (data->'fee'->>'amount')::numeric
  ) as amount,
  COALESCE(
    data->'transfer'->>'assetCode',
    data->'mint'->>'assetCode',
    data->'burn'->>'assetCode',
    data->'fee'->>'assetCode'
  ) as asset_code,
  COALESCE(
    data->'transfer'->>'assetIssuer',
    data->'mint'->>'assetIssuer',
    data->'burn'->>'assetIssuer',
    data->'fee'->>'assetIssuer'
  ) as asset_issuer,
  -- Metadata
  data->'meta'->>'contractAddress' as contract_address,
  (data->'meta'->>'ledgerSequence')::int as ledger,
  data->'meta'->>'txHash' as tx_hash,
  (data->'meta'->>'transactionIndex')::int as tx_index,
  (data->'meta'->>'operationIndex')::int as op_index,
  (data->'meta'->>'inSuccessfulTx')::boolean as successful,
  to_timestamp((data->'meta'->>'closedAtUnix')::bigint) as timestamp,
  created_at
FROM test_events
WHERE event_type IN ('transfer', 'mint', 'burn', 'fee');

-- Transfers only (most common query)
CREATE OR REPLACE VIEW token_transfers AS
SELECT
  id as toid,
  data->'transfer'->>'from' as from_address,
  data->'transfer'->>'to' as to_address,
  (data->'transfer'->>'amount')::numeric as amount,
  data->'transfer'->>'assetCode' as asset_code,
  data->'transfer'->>'assetIssuer' as asset_issuer,
  data->'meta'->>'contractAddress' as contract_address,
  (data->'meta'->>'ledgerSequence')::int as ledger,
  data->'meta'->>'txHash' as tx_hash,
  (data->'meta'->>'inSuccessfulTx')::boolean as successful,
  to_timestamp((data->'meta'->>'closedAtUnix')::bigint) as timestamp,
  created_at
FROM test_events
WHERE event_type = 'transfer';

-- USDC transfers specifically
CREATE OR REPLACE VIEW usdc_transfers AS
SELECT
  toid,
  from_address,
  to_address,
  amount,
  contract_address,
  ledger,
  tx_hash,
  successful,
  timestamp,
  created_at
FROM token_transfers
WHERE asset_code = 'USDC';

-- Large transfers (>100 XLM equivalent, assuming 10M stroops)
CREATE OR REPLACE VIEW large_transfers AS
SELECT
  toid,
  from_address,
  to_address,
  amount,
  asset_code,
  contract_address,
  tx_hash,
  timestamp
FROM token_transfers
WHERE amount > 1000000000; -- 100 XLM in stroops

-- ============================================================================
-- CONTRACT EVENTS
-- ============================================================================

CREATE OR REPLACE VIEW contract_events_decoded AS
SELECT
  id as toid,
  event_type,
  data->>'contractId' as contract_id,
  data->>'type' as event_category, -- CONTRACT, SYSTEM, DIAGNOSTIC
  (data->>'ledgerSequence')::int as ledger,
  data->>'transactionHash' as tx_hash,
  (data->>'transactionIndex')::int as tx_index,
  (data->>'operationIndex')::int as op_index,
  (data->>'eventIndex')::int as event_index,
  (data->>'inSuccessfulTx')::boolean as successful,
  to_timestamp((data->>'timestamp')::bigint) as timestamp,
  data->'topicDecoded' as topics,
  data->'dataDecoded' as event_data,
  (data->'diagnosticEvents'->0) as first_diagnostic_event,
  created_at
FROM contract_events_v2;

-- Contract events by type
CREATE OR REPLACE VIEW contract_transfers AS
SELECT * FROM contract_events_decoded WHERE event_type = 'transfer';

CREATE OR REPLACE VIEW contract_swaps AS
SELECT * FROM contract_events_decoded WHERE event_type = 'swap';

CREATE OR REPLACE VIEW contract_deposits AS
SELECT * FROM contract_events_decoded WHERE event_type = 'deposit';

-- ============================================================================
-- CONTRACT INVOCATIONS
-- ============================================================================

CREATE OR REPLACE VIEW contract_invocations_summary AS
SELECT
  id as toid,
  event_type as function_name,
  data->>'contractId' as contract_id,
  (data->>'successful')::boolean as successful,
  (data->'meta'->>'ledgerSequence')::int as ledger,
  data->'meta'->>'txHash' as tx_hash,
  (data->'meta'->>'transactionIndex')::int as tx_index,
  (data->'meta'->>'operationIndex')::int as op_index,
  to_timestamp((data->'meta'->>'closedAtUnix')::bigint) as timestamp,
  jsonb_array_length(data->'arguments') as arg_count,
  jsonb_array_length(data->'stateChanges') as state_change_count,
  jsonb_array_length(data->'diagnosticEvents') as diagnostic_event_count,
  created_at
FROM full_invocations;

-- Failed invocations only
CREATE OR REPLACE VIEW failed_invocations AS
SELECT * FROM contract_invocations_summary WHERE successful = false;

-- Successful invocations only
CREATE OR REPLACE VIEW successful_invocations AS
SELECT * FROM contract_invocations_summary WHERE successful = true;

-- ============================================================================
-- CARBON SINK (Custom Business Logic)
-- ============================================================================

CREATE OR REPLACE VIEW carbon_offsets AS
SELECT
  id as toid,
  event_type,
  data->>'funder' as funder,
  data->>'recipient' as recipient,
  (data->>'amount')::numeric as amount,
  data->>'project_id' as project_id,
  data->>'memo' as memo,
  data->>'email' as email,
  data->>'tx_hash' as tx_hash,
  (data->>'ledger')::int as ledger,
  to_timestamp((data->>'timestamp')::bigint) as timestamp,
  (data->>'valid_funder')::boolean as valid_funder,
  (data->>'valid_recipient')::boolean as valid_recipient,
  (data->>'valid_amount')::boolean as valid_amount,
  created_at as stored_at
FROM carbon_sink_fixed
WHERE (data->>'valid_funder')::boolean = true
  AND (data->>'valid_recipient')::boolean = true
  AND (data->>'valid_amount')::boolean = true;

-- Carbon offset statistics by project
CREATE OR REPLACE VIEW carbon_project_stats AS
SELECT
  project_id,
  COUNT(*) as offset_count,
  SUM(amount) as total_amount,
  AVG(amount) as avg_amount,
  MIN(amount) as min_amount,
  MAX(amount) as max_amount,
  COUNT(DISTINCT funder) as unique_funders,
  COUNT(DISTINCT recipient) as unique_recipients,
  MIN(timestamp) as first_offset,
  MAX(timestamp) as last_offset
FROM carbon_offsets
GROUP BY project_id;

-- Carbon offset statistics by funder
CREATE OR REPLACE VIEW carbon_funder_stats AS
SELECT
  funder,
  COUNT(*) as offset_count,
  SUM(amount) as total_amount,
  COUNT(DISTINCT project_id) as projects_supported,
  MIN(timestamp) as first_offset,
  MAX(timestamp) as last_offset
FROM carbon_offsets
GROUP BY funder
ORDER BY total_amount DESC;

-- ============================================================================
-- AGGREGATIONS & ANALYTICS
-- ============================================================================

-- Hourly activity summary
CREATE OR REPLACE VIEW hourly_activity AS
SELECT
  date_trunc('hour', timestamp) as hour,
  COUNT(*) as event_count,
  COUNT(DISTINCT from_address) as unique_senders,
  COUNT(DISTINCT to_address) as unique_receivers,
  SUM(amount) as total_volume
FROM token_transfers
GROUP BY date_trunc('hour', timestamp)
ORDER BY hour DESC;

-- Daily USDC volume
CREATE OR REPLACE VIEW daily_usdc_volume AS
SELECT
  date_trunc('day', timestamp) as day,
  COUNT(*) as transfer_count,
  SUM(amount) as total_volume,
  AVG(amount) as avg_amount,
  COUNT(DISTINCT from_address) as unique_senders,
  COUNT(DISTINCT to_address) as unique_receivers
FROM usdc_transfers
WHERE successful = true
GROUP BY date_trunc('day', timestamp)
ORDER BY day DESC;

-- Contract activity leaderboard
CREATE OR REPLACE VIEW contract_activity AS
SELECT
  contract_address,
  COUNT(*) as total_events,
  COUNT(DISTINCT asset_code) as unique_assets,
  SUM(amount) as total_volume,
  MIN(timestamp) as first_seen,
  MAX(timestamp) as last_seen
FROM token_transfers
WHERE successful = true
GROUP BY contract_address
ORDER BY total_events DESC;

-- Top token movers (by volume)
CREATE OR REPLACE VIEW top_movers AS
SELECT
  asset_code,
  COUNT(*) as transfer_count,
  SUM(amount) as total_volume,
  COUNT(DISTINCT from_address) as unique_senders,
  COUNT(DISTINCT to_address) as unique_receivers,
  AVG(amount) as avg_transfer
FROM token_transfers
WHERE successful = true
  AND timestamp > NOW() - INTERVAL '24 hours'
GROUP BY asset_code
ORDER BY total_volume DESC
LIMIT 100;

-- Recent activity feed (last 1000 events)
CREATE OR REPLACE VIEW recent_activity AS
SELECT
  toid,
  'transfer' as activity_type,
  from_address as actor,
  to_address as target,
  amount,
  asset_code,
  tx_hash,
  timestamp
FROM token_transfers
WHERE successful = true
ORDER BY timestamp DESC
LIMIT 1000;

-- ============================================================================
-- INDEXES FOR PERFORMANCE
-- ============================================================================

-- Token transfers indexes
CREATE INDEX IF NOT EXISTS idx_test_events_from ON test_events ((data->'transfer'->>'from'));
CREATE INDEX IF NOT EXISTS idx_test_events_to ON test_events ((data->'transfer'->>'to'));
CREATE INDEX IF NOT EXISTS idx_test_events_asset ON test_events ((data->'transfer'->>'assetCode'));
CREATE INDEX IF NOT EXISTS idx_test_events_contract ON test_events ((data->'meta'->>'contractAddress'));
CREATE INDEX IF NOT EXISTS idx_test_events_ledger ON test_events (((data->'meta'->>'ledgerSequence')::int));
CREATE INDEX IF NOT EXISTS idx_test_events_timestamp ON test_events (created_at DESC);

-- Contract events indexes
CREATE INDEX IF NOT EXISTS idx_contract_events_contract_id ON contract_events_v2 ((data->>'contractId'));
CREATE INDEX IF NOT EXISTS idx_contract_events_ledger ON contract_events_v2 (((data->>'ledgerSequence')::int));
CREATE INDEX IF NOT EXISTS idx_contract_events_timestamp ON contract_events_v2 (((data->>'timestamp')::bigint));

-- Contract invocations indexes
CREATE INDEX IF NOT EXISTS idx_invocations_contract ON full_invocations ((data->>'contractId'));
CREATE INDEX IF NOT EXISTS idx_invocations_function ON full_invocations ((data->>'functionName'));
CREATE INDEX IF NOT EXISTS idx_invocations_successful ON full_invocations (((data->>'successful')::boolean));
CREATE INDEX IF NOT EXISTS idx_invocations_ledger ON full_invocations (((data->'meta'->>'ledgerSequence')::int));

-- Carbon sink indexes
CREATE INDEX IF NOT EXISTS idx_carbon_funder ON carbon_sink_fixed ((data->>'funder'));
CREATE INDEX IF NOT EXISTS idx_carbon_recipient ON carbon_sink_fixed ((data->>'recipient'));
CREATE INDEX IF NOT EXISTS idx_carbon_project ON carbon_sink_fixed ((data->>'project_id'));
CREATE INDEX IF NOT EXISTS idx_carbon_timestamp ON carbon_sink_fixed (((data->>'timestamp')::bigint));

-- ============================================================================
-- HELPER FUNCTIONS
-- ============================================================================

-- Function to decode Stellar address from ScVal
CREATE OR REPLACE FUNCTION decode_stellar_address(scval jsonb)
RETURNS text AS $$
BEGIN
  IF scval->>'addressValue' IS NOT NULL THEN
    RETURN scval->>'addressValue';
  ELSIF scval->>'stringValue' IS NOT NULL THEN
    RETURN scval->>'stringValue';
  ELSE
    RETURN NULL;
  END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Function to decode i128 amount from ScVal
-- NOTE: This is a simplified implementation that works for positive i128 values
-- that fit within PostgreSQL's numeric type. It does NOT handle:
-- - Negative i128 values (would require two's complement conversion)
-- - Values outside numeric's range
-- For production use with full i128 support, consider using a specialized
-- extension or handling i128 values as strings in your application layer.
CREATE OR REPLACE FUNCTION decode_i128_amount(scval jsonb)
RETURNS numeric AS $$
DECLARE
  hex_value text;
BEGIN
  hex_value := scval->>'i128Value';
  IF hex_value IS NULL THEN
    RETURN NULL;
  END IF;

  -- Remove '0x' prefix if present
  hex_value := REPLACE(hex_value, '0x', '');

  -- Convert hex to numeric (works for positive values within numeric range)
  -- This is a simplified approach - for full i128 support including negative
  -- values, you would need to parse the hex manually and handle two's complement
  RETURN ('x' || hex_value)::bit(128)::numeric;
EXCEPTION WHEN OTHERS THEN
  -- Return NULL if conversion fails (e.g., negative numbers, overflow)
  RETURN NULL;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ============================================================================
-- MATERIALIZED VIEWS FOR HEAVY QUERIES
-- ============================================================================
-- Use these for expensive aggregations that don't need real-time data

-- Daily statistics (refresh once per hour)
CREATE MATERIALIZED VIEW IF NOT EXISTS daily_stats AS
SELECT
  date_trunc('day', timestamp) as day,
  COUNT(*) as total_transfers,
  SUM(amount) as total_volume,
  COUNT(DISTINCT from_address) as unique_senders,
  COUNT(DISTINCT to_address) as unique_receivers,
  COUNT(DISTINCT asset_code) as unique_assets
FROM token_transfers
WHERE successful = true
GROUP BY date_trunc('day', timestamp)
ORDER BY day DESC;

-- Create index on materialized view
CREATE UNIQUE INDEX IF NOT EXISTS idx_daily_stats_day ON daily_stats (day);

-- Refresh command (run this periodically):
-- REFRESH MATERIALIZED VIEW CONCURRENTLY daily_stats;

-- ============================================================================
-- PERMISSIONS SETUP (Example)
-- ============================================================================
-- Uncomment and modify for your use case

-- Create read-only role for Hasura anonymous access
-- CREATE ROLE hasura_anonymous;
-- GRANT CONNECT ON DATABASE postgres TO hasura_anonymous;
-- GRANT USAGE ON SCHEMA public TO hasura_anonymous;
-- GRANT SELECT ON ALL TABLES IN SCHEMA public TO hasura_anonymous;
-- GRANT SELECT ON ALL VIEWS IN SCHEMA public TO hasura_anonymous;

-- Create authenticated user role
-- CREATE ROLE hasura_user;
-- GRANT hasura_anonymous TO hasura_user;
-- -- hasura_user has all anonymous permissions plus additional access

COMMENT ON VIEW token_transfer_events IS 'All token transfer events (transfers, mints, burns, fees) with decoded fields';
COMMENT ON VIEW token_transfers IS 'Transfer events only (most common query)';
COMMENT ON VIEW usdc_transfers IS 'USDC transfers only';
COMMENT ON VIEW contract_events_decoded IS 'Contract events with decoded topics and data';
COMMENT ON VIEW contract_invocations_summary IS 'Contract invocations with metadata and counts';
COMMENT ON VIEW carbon_offsets IS 'Validated carbon offset events with business fields';
COMMENT ON VIEW carbon_project_stats IS 'Aggregated statistics by project';
COMMENT ON VIEW carbon_funder_stats IS 'Aggregated statistics by funder';
