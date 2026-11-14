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

// TestLedgerCloseTimeRFC3339 pins the rendering, which is the half of
// Option A that several processors had each solved separately: one used
// "2006-01-02T15:04:05.000Z07:00", another "2006-01-02T15:04:05.000Z", and
// RFC3339Nano is tempting and wrong because it drops trailing zeros.
func TestLedgerCloseTimeRFC3339(t *testing.T) {
	tests := []struct {
		name string
		ext  xdr.StellarValueExt
		// seconds is the whole-second close time, which pre-Protocol-30 arms
		// are promoted from.
		seconds xdr.TimePoint
		want    string
	}{
		{
			name: "a millisecond value renders three digits",
			ext:  xdr.StellarValueExt{V: xdr.StellarValueTypeStellarValueSignedMs, SignedMsValue: &xdr.StellarValueSignedMsValue{CloseTimeMs: xdr.TimePointMs(1794662144916)}},
			want: "2026-11-14T13:15:44.916Z",
		},
		{
			// The case RFC3339Nano gets wrong: it would render
			// "2026-11-14T13:15:44Z" and the field's shape would vary with
			// the value.
			name: "a whole second still renders three digits",
			ext:  xdr.StellarValueExt{V: xdr.StellarValueTypeStellarValueSignedMs, SignedMsValue: &xdr.StellarValueSignedMsValue{CloseTimeMs: xdr.TimePointMs(1794662144000)}},
			want: "2026-11-14T13:15:44.000Z",
		},
		{
			name: "the empty-transaction-set arm renders the same",
			ext:  xdr.StellarValueExt{V: xdr.StellarValueTypeStellarValueEmptyTxSetMs, ProposedMsValue: &xdr.StellarValueProposedMsValue{CloseTimeMs: xdr.TimePointMs(1794662149916)}},
			want: "2026-11-14T13:15:49.916Z",
		},
		{
			// Pre-Protocol-30: the whole second is promoted, so the
			// fractional part is zero rather than absent.
			name:    "a pre-Protocol-30 ledger promotes its whole second",
			ext:     xdr.StellarValueExt{V: xdr.StellarValueTypeStellarValueBasic},
			seconds: 1765158311,
			want:    "2025-12-08T01:45:11.000Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, LedgerCloseTimeRFC3339(ledgerWithScpExt(tt.seconds, tt.ext)))
		})
	}
}

// TestFormatCloseTimeRFC3339 is the same rendering for a caller that already
// holds the millisecond value.
func TestFormatCloseTimeRFC3339(t *testing.T) {
	assert.Equal(t, "2026-11-14T13:15:44.916Z", FormatCloseTimeRFC3339(1794662144916))
	assert.Equal(t, "2026-11-14T13:15:44.000Z", FormatCloseTimeRFC3339(1794662144000))

	// Same second, different millisecond: the two render differently, which
	// is the whole point of the field.
	assert.NotEqual(t, FormatCloseTimeRFC3339(1794662144200), FormatCloseTimeRFC3339(1794662144800))
}
