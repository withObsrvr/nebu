package processor

import (
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// LedgerCloseTimeMs returns the ledger's close time in milliseconds since the
// Unix epoch, in UTC.
//
// Use this rather than the SDK's [xdr.LedgerCloseMeta.ClosedAt] or
// LedgerCloseTime whenever the value is emitted, stored, ordered, or used to
// bucket events. Those two return whole seconds, and from Protocol 30 onward
// whole seconds are ambiguous: CAP-88 gives close times millisecond
// resolution, so consecutive ledgers can close within the same second. Code
// that windows or deduplicates on a second-resolution close time does not
// fail — it silently produces the wrong answer.
//
// To emit the value as the RFC 3339 UTC timestamp that OBSRVR data contracts
// require, convert at the edge:
//
//	time.UnixMilli(processor.LedgerCloseTimeMs(ledger)).UTC().Format(time.RFC3339Nano)
//
// # Why this is not a one-liner
//
// The millisecond value lives in the StellarValueExt union, and **two** of its
// arms carry it:
//
//	StellarValueType                 Arm                 CloseTimeMs
//	0  BASIC                         (void)              no
//	1  SIGNED                        LcValueSignature    no
//	2  EMPTY_TX_SET                  ProposedValue       no
//	3  SIGNED_MS                     SignedMsValue       yes
//	4  EMPTY_TX_SET_MS               ProposedMsValue     yes
//
// Reading only arm 3 — the natural first guess, since GetSignedMsValue is the
// obvious accessor — compiles, reviews cleanly, and works on every ledger that
// contains transactions. It silently falls back to second resolution on every
// *empty* Protocol 30 ledger, which is arm 4. That is a failure mode no
// fixture captured from a busy ledger will ever reveal, which is exactly why
// this read belongs in one tested place rather than in each processor.
//
// For ledgers before Protocol 30 (arms 0 through 2) there is no millisecond
// value to read, and the whole-second close time is promoted. Those ledgers
// are unambiguous — their close times are seconds apart — so the promotion
// loses nothing.
func LedgerCloseTimeMs(ledger xdr.LedgerCloseMeta) int64 {
	scpValue := ledger.LedgerHeaderHistoryEntry().Header.ScpValue

	if signed, ok := scpValue.Ext.GetSignedMsValue(); ok {
		return int64(signed.CloseTimeMs)
	}
	if proposed, ok := scpValue.Ext.GetProposedMsValue(); ok {
		return int64(proposed.CloseTimeMs)
	}

	// Pre-Protocol-30: only whole seconds exist. Promote rather than
	// returning zero, so callers get a usable timestamp for historical
	// ledgers without special-casing the protocol version.
	return int64(scpValue.CloseTime) * 1000
}

// closeTimeLayout renders a close time as RFC 3339 in UTC with exactly three
// decimal places. RFC3339Nano is the obvious choice and the wrong one: it
// drops trailing zeros, so a ledger that closed on a whole second renders
// without a fractional part and one that closed at .916 renders with three
// digits. The field then has a varying shape, which is awkward to parse
// strictly and invites a consumer to treat the short form as second
// resolution.
const closeTimeLayout = "2006-01-02T15:04:05.000Z07:00"

// LedgerCloseTimeRFC3339 returns the ledger's close time as RFC 3339 in UTC
// with millisecond precision, e.g. "2026-11-14T13:15:44.916Z".
//
// This is the form DATA-3 of the OBSRVR Engineering Standard asks events to
// carry, and it is the rendering every processor should use, so that one
// timestamp does not arrive as "...44.916Z" from one processor and
// "...44.916000Z" or "...44Z" from another. It reads through
// [LedgerCloseTimeMs], so it is correct on both of the Protocol 30 union arms
// and promotes a pre-Protocol-30 whole second.
//
// It renders milliseconds and no finer. CAP-88 defines close times in
// milliseconds, so there is nothing below that to lose.
func LedgerCloseTimeRFC3339(ledger xdr.LedgerCloseMeta) string {
	return time.UnixMilli(LedgerCloseTimeMs(ledger)).UTC().Format(closeTimeLayout)
}

// FormatCloseTimeRFC3339 renders a millisecond Unix timestamp the same way, for
// a caller that already holds the value — a processor that read it once and
// passes it down, or one whose timestamp comes from somewhere other than a
// LedgerCloseMeta.
func FormatCloseTimeRFC3339(closeTimeMs int64) string {
	return time.UnixMilli(closeTimeMs).UTC().Format(closeTimeLayout)
}
