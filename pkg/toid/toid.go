// Package toid provides utilities for generating Stellar-compatible Total Order IDs (TOIDs).
// This is a thin wrapper around github.com/stellar/go/toid that extracts TOIDs from nebu event JSON.
//
// TOIDs are used for idempotent event storage in databases. They uniquely identify operations
// within the Stellar network using the formula: (ledger * 2^32) + (tx * 2^12) + op.
//
// See SEP-35: https://github.com/stellar/stellar-protocol/blob/master/ecosystem/sep-0035.md
package toid

import (
	"fmt"

	stellarToid "github.com/stellar/go-stellar-sdk/toid"
)

// FromEvent extracts a TOID from a nebu event.
// Uses the official Stellar SDK implementation (SEP-35 compliant).
//
// Supports two formats:
//   1. Token-transfer format (with "meta" wrapper):
//      event := map[string]interface{}{
//          "meta": map[string]interface{}{
//              "ledgerSequence": 60200000,
//              "transactionIndex": 5,
//              "operationIndex": 2,
//          },
//      }
//
//   2. Contract-events format (top-level fields):
//      event := map[string]interface{}{
//          "ledgerSequence": 60200100,
//          "transactionIndex": 3,
//          "operationIndex": 0,
//      }
//
// Example:
//	id, err := toid.FromEvent(event)
func FromEvent(event map[string]interface{}) (int64, error) {
	// Try to extract from "meta" wrapper first (token-transfer format)
	var source map[string]interface{}
	if meta, ok := event["meta"].(map[string]interface{}); ok {
		source = meta
	} else {
		// Fall back to top-level fields (contract-events format)
		source = event
	}

	ledgerSeq, err := getInt32(source, "ledgerSequence")
	if err != nil {
		return 0, fmt.Errorf("invalid ledgerSequence: %w", err)
	}

	txIndex, err := getInt32(source, "transactionIndex")
	if err != nil {
		return 0, fmt.Errorf("invalid transactionIndex: %w", err)
	}

	opIndex, err := getInt32(source, "operationIndex")
	if err != nil {
		return 0, fmt.Errorf("invalid operationIndex: %w", err)
	}

	// Use official Stellar SDK to calculate TOID
	id := stellarToid.New(ledgerSeq, txIndex, opIndex)
	return id.ToInt64(), nil
}

// Parse decomposes a TOID back into its components using the Stellar SDK.
// Returns (ledgerSequence, transactionIndex, operationIndex).
//
// Example:
//
//	ledger, tx, op := toid.Parse(258620518556708864)
//	// ledger=60200000, tx=3, op=0
func Parse(toidValue int64) (ledgerSeq, txIndex, opIndex int32) {
	id := stellarToid.Parse(toidValue)
	return id.LedgerSequence, id.TransactionOrder, id.OperationOrder
}

// getInt32 extracts an int32 from a map, handling JSON's float64 encoding.
func getInt32(m map[string]interface{}, key string) (int32, error) {
	val, ok := m[key]
	if !ok {
		return 0, fmt.Errorf("missing field '%s'", key)
	}

	switch v := val.(type) {
	case float64:
		return int32(v), nil
	case int32:
		return v, nil
	case int:
		return int32(v), nil
	case int64:
		return int32(v), nil
	default:
		return 0, fmt.Errorf("field '%s' is not a number (got %T)", key, val)
	}
}
