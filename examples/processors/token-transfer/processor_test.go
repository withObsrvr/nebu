package token_transfer

import (
	"context"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/withObsrvr/nebu/pkg/runtime"
	"github.com/withObsrvr/nebu/pkg/source"
)

func TestOrigin_ProcessLedger(t *testing.T) {
	// Integration test - processes real ledgers from RPC
	src, err := source.NewRPCLedgerSource("https://archive-rpc.lightsail.network")
	require.NoError(t, err)
	defer src.Close()

	origin := NewOrigin(network.PublicNetworkPassphrase)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Collect events in background
	var events []string
	done := make(chan struct{})

	go func() {
		for ev := range origin.Out() {
			// Categorize by event type
			switch {
			case ev.GetTransfer() != nil:
				events = append(events, "transfer")
			case ev.GetMint() != nil:
				events = append(events, "mint")
			case ev.GetBurn() != nil:
				events = append(events, "burn")
			case ev.GetClawback() != nil:
				events = append(events, "clawback")
			case ev.GetFee() != nil:
				events = append(events, "fee")
			}
		}
		close(done)
	}()

	// Run origin processor on a small range
	rt := runtime.NewRuntime()
	err = rt.RunOrigin(ctx, src, origin, 60200000, 60200002)
	require.NoError(t, err)

	// Close emitter and wait for collection to finish
	origin.Close()
	<-done

	// Should have collected some events (exact count varies)
	assert.Greater(t, len(events), 0, "should have processed some token transfer events")

	// Log summary
	t.Logf("Processed %d events from ledgers 60200000-60200002", len(events))
}

func TestOrigin_Name(t *testing.T) {
	origin := NewOrigin(network.PublicNetworkPassphrase)
	assert.Equal(t, "stellar/token-transfer", origin.Name())
}

func TestOrigin_Type(t *testing.T) {
	origin := NewOrigin(network.PublicNetworkPassphrase)
	assert.Equal(t, "origin", origin.Type().String())
}
