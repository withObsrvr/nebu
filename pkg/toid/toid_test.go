package toid_test

import (
	"testing"

	"github.com/withObsrvr/nebu/pkg/toid"
	stellarToid "github.com/stellar/go-stellar-sdk/toid"
)

func TestFromEvent(t *testing.T) {
	tests := []struct {
		name      string
		event     map[string]interface{}
		wantError bool
	}{
		{
			name: "valid event with float64 values",
			event: map[string]interface{}{
				"type": "transfer",
				"meta": map[string]interface{}{
					"ledgerSequence":   float64(60200000),
					"transactionIndex": float64(5),
					"operationIndex":   float64(2),
				},
			},
			wantError: false,
		},
		{
			name: "valid event with int values",
			event: map[string]interface{}{
				"type": "transfer",
				"meta": map[string]interface{}{
					"ledgerSequence":   int32(60200000),
					"transactionIndex": int32(5),
					"operationIndex":   int32(2),
				},
			},
			wantError: false,
		},
		{
			name: "missing meta field",
			event: map[string]interface{}{
				"type": "transfer",
			},
			wantError: true,
		},
		{
			name: "missing ledgerSequence",
			event: map[string]interface{}{
				"meta": map[string]interface{}{
					"transactionIndex": float64(1),
					"operationIndex":   float64(1),
				},
			},
			wantError: true,
		},
		{
			name: "missing transactionIndex",
			event: map[string]interface{}{
				"meta": map[string]interface{}{
					"ledgerSequence": float64(60200000),
					"operationIndex": float64(1),
				},
			},
			wantError: true,
		},
		{
			name: "missing operationIndex",
			event: map[string]interface{}{
				"meta": map[string]interface{}{
					"ledgerSequence":   float64(60200000),
					"transactionIndex": float64(1),
				},
			},
			wantError: true,
		},
		{
			name: "wrong type for ledgerSequence",
			event: map[string]interface{}{
				"meta": map[string]interface{}{
					"ledgerSequence":   "60200000",
					"transactionIndex": float64(1),
					"operationIndex":   float64(1),
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := toid.FromEvent(tt.event)
			if tt.wantError {
				if err == nil {
					t.Errorf("FromEvent() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("FromEvent() unexpected error: %v", err)
			}

			// Verify it matches Stellar SDK directly
			meta := tt.event["meta"].(map[string]interface{})
			var ledger, tx, op int32

			// Extract values based on type
			switch v := meta["ledgerSequence"].(type) {
			case float64:
				ledger = int32(v)
			case int32:
				ledger = v
			case int:
				ledger = int32(v)
			}

			switch v := meta["transactionIndex"].(type) {
			case float64:
				tx = int32(v)
			case int32:
				tx = v
			case int:
				tx = int32(v)
			}

			switch v := meta["operationIndex"].(type) {
			case float64:
				op = int32(v)
			case int32:
				op = v
			case int:
				op = int32(v)
			}

			expected := stellarToid.New(ledger, tx, op).ToInt64()
			if id != expected {
				t.Errorf("TOID mismatch: got %d, want %d", id, expected)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name           string
		ledger         int32
		tx             int32
		op             int32
	}{
		{
			name:   "known TOID from Stellar testnet",
			ledger: 1309069,
			tx:     3,
			op:     0,
		},
		{
			name:   "mainnet ledger 60200000",
			ledger: 60200000,
			tx:     0,
			op:     0,
		},
		{
			name:   "with transaction and operation indices",
			ledger: 60200000,
			tx:     163,
			op:     5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate TOID using Stellar SDK
			toidValue := stellarToid.New(tt.ledger, tt.tx, tt.op).ToInt64()

			// Parse it back using our wrapper
			parsedLedger, parsedTx, parsedOp := toid.Parse(toidValue)

			// Should match original components
			if parsedLedger != tt.ledger {
				t.Errorf("ledger mismatch: got %d, want %d", parsedLedger, tt.ledger)
			}
			if parsedTx != tt.tx {
				t.Errorf("tx mismatch: got %d, want %d", parsedTx, tt.tx)
			}
			if parsedOp != tt.op {
				t.Errorf("op mismatch: got %d, want %d", parsedOp, tt.op)
			}

			// Verify our wrapper matches Stellar SDK exactly
			sdkParsed := stellarToid.Parse(toidValue)
			if parsedLedger != sdkParsed.LedgerSequence {
				t.Errorf("ledger doesn't match Stellar SDK: got %d, SDK says %d", parsedLedger, sdkParsed.LedgerSequence)
			}
			if parsedTx != sdkParsed.TransactionOrder {
				t.Errorf("tx doesn't match Stellar SDK: got %d, SDK says %d", parsedTx, sdkParsed.TransactionOrder)
			}
			if parsedOp != sdkParsed.OperationOrder {
				t.Errorf("op doesn't match Stellar SDK: got %d, SDK says %d", parsedOp, sdkParsed.OperationOrder)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	event := map[string]interface{}{
		"type": "transfer",
		"meta": map[string]interface{}{
			"ledgerSequence":   float64(60200000),
			"transactionIndex": float64(5),
			"operationIndex":   float64(2),
		},
	}

	// Extract TOID from event
	id, err := toid.FromEvent(event)
	if err != nil {
		t.Fatalf("FromEvent failed: %v", err)
	}

	// Parse it back
	ledger, tx, op := toid.Parse(id)

	// Should match original values
	if ledger != 60200000 {
		t.Errorf("ledger mismatch: got %d, want 60200000", ledger)
	}
	if tx != 5 {
		t.Errorf("tx mismatch: got %d, want 5", tx)
	}
	if op != 2 {
		t.Errorf("op mismatch: got %d, want 2", op)
	}

	// Reconstruct TOID and verify it matches
	reconstructed := stellarToid.New(ledger, tx, op).ToInt64()
	if reconstructed != id {
		t.Errorf("round-trip failed: original %d != reconstructed %d", id, reconstructed)
	}
}
