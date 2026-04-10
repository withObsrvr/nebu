-- Filter USDC Transfers from Specific Contract
--
-- This query shows how to filter token-transfer events by both asset and contract.
--
-- Usage:
--   token-transfer --start-ledger 60200000 --end-ledger 60300000 | \
--     duckdb -c "$(cat examples/queries/usdc_by_contract.sql)" -json

SELECT
  -- Extract metadata
  json_extract_string(meta, '$.ledgerSequence') as ledger,
  json_extract_string(meta, '$.txHash') as tx_hash,
  json_extract_string(meta, '$.contractAddress') as contract,

  -- Extract transfer details
  json_extract_string(transfer, '$.from') as from_address,
  json_extract_string(transfer, '$.to') as to_address,
  CAST(json_extract_string(transfer, '$.amount') AS BIGINT) as amount,

  -- Extract asset info
  json_extract_string(transfer, '$.assetCode') as asset_code,
  json_extract_string(transfer, '$.assetIssuer') as issuer

FROM read_json_auto('/dev/stdin', format='newline_delimited')
WHERE transfer IS NOT NULL
  AND json_extract_string(transfer, '$.assetCode') = 'USDC'
  AND json_extract_string(meta, '$.contractAddress') = 'CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA';  -- Replace with your contract
