-- Extract Payment Events from Specific Contract
--
-- This is the same as extract_payment_events.sql but filtered to a specific contract.
-- Update the CONTRACT_ADDRESS variable below to filter for your contract.
--
-- Usage:
--   contract-events --start-ledger 60200000 --end-ledger 60300000 | \
--     duckdb -c "$(cat examples/queries/extract_payment_events_filtered.sql)" -json

SELECT
  -- Generate unique composite ID: ledger-txhash[:8]-opindex
  format('{}-{}-{}',
    ledgerSequence,
    substring(transactionHash, 1, 8),
    COALESCE(operationIndex, 0)
  ) as id,

  -- Contract and event metadata
  contractId as contract_id,
  eventType as event_type,

  -- Extract payment_id from nested dataDecoded structure
  '0x' || json_extract_string(to_json(dataDecoded), '$.entries.payment_id.hex') as payment_id,

  -- Extract token contract address
  json_extract_string(to_json(dataDecoded), '$.entries.token.address') as token_id,

  -- Extract payment amount as unsigned bigint
  CAST(json_extract(to_json(dataDecoded), '$.entries.amount') AS UBIGINT) as amount,

  -- Extract sender address
  json_extract_string(to_json(dataDecoded), '$.entries.from.address') as from_id,

  -- Extract merchant address
  json_extract_string(to_json(dataDecoded), '$.entries.merchant.address') as merchant_id,

  -- Extract royalty amount
  CAST(json_extract(to_json(dataDecoded), '$.entries.royalty_amount') AS UBIGINT) as royalty_amount,

  -- Include transaction metadata
  transactionHash as tx_hash,
  ledgerSequence as block_height,
  CAST(timestamp AS BIGINT) as block_timestamp_unix,
  inSuccessfulTx as successful,
  networkPassphrase as network

FROM read_json_auto('/dev/stdin', format='newline_delimited')
WHERE eventType = 'payment'
  AND contractId = 'CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA'  -- Replace with your contract
  AND inSuccessfulTx = true;  -- Only include events from successful transactions
