package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRPCLedgerSource_Stream(t *testing.T) {
	// This is an integration test that hits real Stellar RPC
	// Use a small range of recent ledgers (within RPC's retention window)
	src, err := NewLedgerSource("https://archive-rpc.lightsail.network")
	require.NoError(t, err)
	defer src.Close()

	ch := make(chan xdr.LedgerCloseMeta, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stream 5 recent ledgers
	// Note: RPC typically retains ~24 hours of ledgers
	// Using a recent-ish range that should be available
	startLedger := uint32(60200000)
	endLedger := uint32(60200004)

	go func() {
		err := src.Stream(ctx, startLedger, endLedger, ch)
		if err != nil && err != context.Canceled {
			t.Logf("Stream error: %v", err)
		}
	}()

	// Collect ledgers
	var ledgers []xdr.LedgerCloseMeta
	for ledger := range ch {
		ledgers = append(ledgers, ledger)
	}

	// Should have received exactly 5 ledgers
	assert.Equal(t, 5, len(ledgers), "expected 5 ledgers")

	// Verify ledger sequences are correct
	for i, ledger := range ledgers {
		expectedSeq := startLedger + uint32(i)
		actualSeq := ledger.LedgerSequence()
		assert.Equal(t, expectedSeq, actualSeq, "ledger %d has wrong sequence", i)
	}
}

func TestRPCLedgerSource_InvalidRange(t *testing.T) {
	src, err := NewLedgerSource("https://archive-rpc.lightsail.network")
	require.NoError(t, err)
	defer src.Close()

	ctx := context.Background()

	tests := []struct {
		name  string
		start uint32
		end   uint32
	}{
		{"zero start", 0, 100},
		{"zero end", 100, 0},
		{"both zero", 0, 0},
		{"start > end", 100, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh channel for each test to avoid close panics
			ch := make(chan xdr.LedgerCloseMeta, 10)
			err := src.Stream(ctx, tt.start, tt.end, ch)
			assert.Error(t, err, "expected error for invalid range")
		})
	}
}

func TestRPCLedgerSource_Cancellation(t *testing.T) {
	src, err := NewLedgerSource("https://archive-rpc.lightsail.network")
	require.NoError(t, err)
	defer src.Close()

	ch := make(chan xdr.LedgerCloseMeta, 10)
	ctx, cancel := context.WithCancel(context.Background())

	// Start streaming a larger range of recent ledgers
	go func() {
		_ = src.Stream(ctx, 60200000, 60200037, ch)
	}()

	// Read a few ledgers then cancel
	count := 0
	for range ch {
		count++
		if count == 3 {
			cancel()
			break
		}
	}

	// Drain remaining
	for range ch {
		count++
	}

	// Should have stopped early (not all 38 ledgers)
	assert.Less(t, count, 38, "should have stopped before completing full range")
}

func TestNewLedgerSource_EmptyURL(t *testing.T) {
	_, err := NewLedgerSource("")
	assert.Error(t, err, "should reject empty URL")
}
