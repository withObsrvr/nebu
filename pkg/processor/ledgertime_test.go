package processor

import (
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
)

// ledgerWithScpExt builds a LedgerCloseMeta whose ScpValue carries the given
// whole-second close time and ext union.
func ledgerWithScpExt(closeTimeSeconds xdr.TimePoint, ext xdr.StellarValueExt) xdr.LedgerCloseMeta {
	return xdr.LedgerCloseMeta{
		V: 0,
		V0: &xdr.LedgerCloseMetaV0{
			LedgerHeader: xdr.LedgerHeaderHistoryEntry{
				Header: xdr.LedgerHeader{
					LedgerSeq: xdr.Uint32(60200000),
					ScpValue: xdr.StellarValue{
						CloseTime: closeTimeSeconds,
						Ext:       ext,
					},
				},
			},
		},
	}
}

// TestLedgerCloseTimeMs covers every StellarValueType discriminant. Arms 3 and
// 4 both carry CloseTimeMs; a regression that drops arm 4 would pass a test
// that only exercised arm 3, so all five are asserted here.
func TestLedgerCloseTimeMs(t *testing.T) {
	const seconds xdr.TimePoint = 1765158311

	tests := []struct {
		name string
		ext  xdr.StellarValueExt
		want int64
	}{
		{
			name: "arm 0 BASIC promotes whole seconds",
			ext:  xdr.StellarValueExt{V: xdr.StellarValueTypeStellarValueBasic},
			want: 1765158311000,
		},
		{
			name: "arm 1 SIGNED promotes whole seconds",
			ext: xdr.StellarValueExt{
				V:                xdr.StellarValueTypeStellarValueSigned,
				LcValueSignature: &xdr.LedgerCloseValueSignature{},
			},
			want: 1765158311000,
		},
		{
			name: "arm 2 EMPTY_TX_SET promotes whole seconds",
			ext: xdr.StellarValueExt{
				V:             xdr.StellarValueTypeStellarValueEmptyTxSet,
				ProposedValue: &xdr.StellarValueProposedValue{},
			},
			want: 1765158311000,
		},
		{
			name: "arm 3 SIGNED_MS uses the millisecond value",
			ext: xdr.StellarValueExt{
				V: xdr.StellarValueTypeStellarValueSignedMs,
				SignedMsValue: &xdr.StellarValueSignedMsValue{
					CloseTimeMs: xdr.TimePointMs(1765158311123),
				},
			},
			want: 1765158311123,
		},
		{
			// The arm an implementation that only handles SIGNED_MS silently
			// gets wrong, and that a fixture from a busy ledger never covers.
			name: "arm 4 EMPTY_TX_SET_MS uses the millisecond value",
			ext: xdr.StellarValueExt{
				V: xdr.StellarValueTypeStellarValueEmptyTxSetMs,
				ProposedMsValue: &xdr.StellarValueProposedMsValue{
					CloseTimeMs: xdr.TimePointMs(1765158311456),
				},
			},
			want: 1765158311456,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LedgerCloseTimeMs(ledgerWithScpExt(seconds, tt.ext))
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestLedgerCloseTimeMsDistinguishesSameSecondLedgers is the behaviour the
// helper exists for: two Protocol 30 ledgers closing inside one whole second
// must be distinguishable, which is what a second-resolution close time
// cannot do.
func TestLedgerCloseTimeMsDistinguishesSameSecondLedgers(t *testing.T) {
	const sameSecond xdr.TimePoint = 1765158311

	first := LedgerCloseTimeMs(ledgerWithScpExt(sameSecond, xdr.StellarValueExt{
		V:             xdr.StellarValueTypeStellarValueSignedMs,
		SignedMsValue: &xdr.StellarValueSignedMsValue{CloseTimeMs: xdr.TimePointMs(1765158311200)},
	}))
	second := LedgerCloseTimeMs(ledgerWithScpExt(sameSecond, xdr.StellarValueExt{
		V:             xdr.StellarValueTypeStellarValueSignedMs,
		SignedMsValue: &xdr.StellarValueSignedMsValue{CloseTimeMs: xdr.TimePointMs(1765158311800)},
	}))

	assert.NotEqual(t, first, second, "same-second ledgers must be distinguishable")
	assert.Equal(t, int64(600), second-first)
	assert.Equal(t, time.Unix(int64(sameSecond), 0).Unix(), time.UnixMilli(first).Unix(),
		"both still fall inside the original whole second")
}

// TestLedgerCloseTimeMsRFC3339 documents the DATA-3 conversion in the doc
// comment, so the documented one-liner cannot rot.
func TestLedgerCloseTimeMsRFC3339(t *testing.T) {
	// 1765158311 is the real close time of pubnet ledger 60200000
	// (2025-12-08), plus 123ms. Keep the year as 2025.
	ledger := ledgerWithScpExt(0, xdr.StellarValueExt{
		V:             xdr.StellarValueTypeStellarValueSignedMs,
		SignedMsValue: &xdr.StellarValueSignedMsValue{CloseTimeMs: xdr.TimePointMs(1765158311123)},
	})

	got := time.UnixMilli(LedgerCloseTimeMs(ledger)).UTC().Format(time.RFC3339Nano)
	assert.Equal(t, "2025-12-08T01:45:11.123Z", got)
}
