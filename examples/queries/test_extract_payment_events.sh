#!/usr/bin/env bash
# Test the extract_payment_events.sql query with mock data

echo "Testing extract_payment_events.sql with mock data..."
echo ""

# Create mock payment event that matches contract-events schema
echo '{"_schema":"nebu.contract-events.v1","_nebu_version":"dev","timestamp":"1765158311", "ledgerSequence":60200000, "transactionHash":"165fee8954a31cc99e75e66f10dbf2b53e9905db743cba6e3c87d516ad002590", "contractId":"CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA", "eventType":"payment", "topicDecoded":[{"symbolValue":"payment"}], "dataDecoded":{"entries":{"payment_id":{"hex":"abc123"},"token":{"address":"CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC"},"amount":1000000000,"from":{"address":"GAMQQFFSOW7VBIKMDFGGNNJ6XYNQ26MJZAXKKDWAN2PYAJG5WYBSF72B"},"merchant":{"address":"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},"royalty_amount":50000000}}, "inSuccessfulTx":true, "operationIndex":0, "networkPassphrase":"Public Global Stellar Network ; September 2015"}
{"_schema":"nebu.contract-events.v1","_nebu_version":"dev","timestamp":"1765158316", "ledgerSequence":60200001, "transactionHash":"265fee8954a31cc99e75e66f10dbf2b53e9905db743cba6e3c87d516ad002591", "contractId":"CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA", "eventType":"payment", "topicDecoded":[{"symbolValue":"payment"}], "dataDecoded":{"entries":{"payment_id":{"hex":"def456"},"token":{"address":"CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC"},"amount":500000000,"from":{"address":"GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H"},"merchant":{"address":"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},"royalty_amount":25000000}}, "inSuccessfulTx":true, "operationIndex":1, "networkPassphrase":"Public Global Stellar Network ; September 2015"}' | \
  duckdb -c "$(cat examples/queries/extract_payment_events.sql)"

echo ""
echo "✓ Query executed successfully!"
echo ""
echo "To use with real data:"
echo "  contract-events --start-ledger 60200000 --end-ledger 60300000 | \\"
echo "    duckdb -c \"\$(cat examples/queries/extract_payment_events.sql)\" -json"
