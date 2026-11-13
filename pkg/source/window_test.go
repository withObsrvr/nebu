package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLast(t *testing.T) {
	tests := []struct {
		name      string
		window    LedgerWindow
		n         uint32
		wantStart uint32
		wantEnd   uint32
		wantErr   string
	}{
		{
			name:      "ordinary request",
			window:    LedgerWindow{Oldest: 65000000, Latest: 65430000},
			n:         100,
			wantStart: 65429901,
			wantEnd:   65430000,
		},
		{
			// The boundary: exactly the whole window is servable.
			name:      "exactly the window",
			window:    LedgerWindow{Oldest: 100, Latest: 199},
			n:         100,
			wantStart: 100,
			wantEnd:   199,
		},
		{
			name:      "one ledger",
			window:    LedgerWindow{Oldest: 2, Latest: 65430000},
			n:         1,
			wantStart: 65430000,
			wantEnd:   65430000,
		},
		{
			// Deeper than the reported window is allowed, because the
			// reported floor is not reliable: archive-backed RPC endpoints
			// report a window of a few dozen ledgers and serve history
			// millions deep. Requesting the full range and letting the
			// stream fail beats refusing a range the source can serve.
			name:      "deeper than the reported window is still resolved",
			window:    LedgerWindow{Oldest: 65458009, Latest: 65458072},
			n:         1000,
			wantStart: 65457073,
			wantEnd:   65458072,
		},
		{
			// The one real limit: you cannot ask for more ledgers than the
			// chain has, which would underflow the subtraction.
			name:    "more ledgers than the chain has",
			window:  LedgerWindow{Oldest: 65300000, Latest: 65430000},
			n:       99999999,
			wantErr: "the chain only reaches ledger 65430000",
		},
		{
			// Would underflow uint32 if computed as Latest - n + 1 without
			// the window check first.
			name:    "more ledgers than exist at all",
			window:  LedgerWindow{Oldest: 2, Latest: 50},
			n:       4294967295,
			wantErr: "cannot serve",
		},
		{name: "zero", window: LedgerWindow{Oldest: 2, Latest: 100}, n: 0, wantErr: "greater than zero"},
		{name: "empty source", window: LedgerWindow{}, n: 10, wantErr: "no ledgers"},
		{
			// An inverted window is no longer rejected: Oldest is advisory,
			// so only Latest governs the arithmetic.
			name:      "inverted window still counts back from latest",
			window:    LedgerWindow{Oldest: 500, Latest: 100},
			n:         10,
			wantStart: 91,
			wantEnd:   100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := ResolveLast(tt.window, tt.n)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantEnd, end)
			assert.Equal(t, tt.n, end-start+1, "the resolved range must be exactly n ledgers")
		})
	}
}

func TestLedgerWindowString(t *testing.T) {
	assert.Equal(t, "100-200", LedgerWindow{Oldest: 100, Latest: 200}.String())
}

func TestLedgerWindowCoversIsAdvisory(t *testing.T) {
	// Measured against archive-rpc.lightsail.network, which reports a
	// 64-ledger window and serves ledger 60200000 regardless. Covers exists
	// to drive a warning, never a refusal.
	reported := LedgerWindow{Oldest: 65458009, Latest: 65458072}

	assert.True(t, reported.Covers(65458009), "the floor itself is covered")
	assert.True(t, reported.Covers(65458050))
	assert.False(t, reported.Covers(60200000), "below the reported floor")

	// A source that did not report a floor makes no claim either way.
	assert.True(t, LedgerWindow{Latest: 100}.Covers(1))
}
