package processor

import (
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
