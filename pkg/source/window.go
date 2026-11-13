package source

import (
	"context"
	"fmt"
)

// LedgerWindow is the range of ledgers a source can currently serve.
//
// Both bounds matter. Latest is what "the last N ledgers" counts back from;
// Oldest is what makes the difference between refusing a range the source
// cannot serve and discovering that fact part-way through a backfill.
type LedgerWindow struct {
	// Oldest is the lowest ledger the source still holds.
	Oldest uint32
	// Latest is the highest ledger the source holds.
	Latest uint32
}

// String renders the window for a diagnostic line.
func (w LedgerWindow) String() string {
	return fmt.Sprintf("%d-%d", w.Oldest, w.Latest)
}

// WindowedSource is implemented by a [LedgerSource] that can report which
// ledgers it currently holds.
//
// It is a separate interface rather than a method on LedgerSource because
// LedgerSource is part of the stable surface: adding a method there would
// break every external implementation. Callers type-assert for this one, so
// a source that cannot answer simply does not implement it.
//
// Note that the answer is source-relative and the two sources disagree by
// design. An RPC endpoint reports the network tip and its own retention
// floor; an archive reports the newest and oldest ledgers that have been
// *exported*, which lags the network. "The last 100 ledgers" is therefore a
// different hundred depending on where it is asked, which is why callers are
// expected to report the range they resolved.
type WindowedSource interface {
	LedgerWindow(ctx context.Context) (LedgerWindow, error)
}

// Covers reports whether start is at or above the oldest ledger the source
// said it holds.
//
// Advisory only. An archive-backed RPC endpoint reports a retention window
// covering its last few dozen ledgers and then serves history millions of
// ledgers deeper — archive-rpc.lightsail.network reports a 64-ledger window
// and returns ledger 60200000 without complaint. So a false here means "the
// source did not claim to have this", not "the source cannot serve it", and
// it is worth a warning rather than a refusal.
func (w LedgerWindow) Covers(start uint32) bool {
	return w.Oldest == 0 || start >= w.Oldest
}

// ResolveLast returns the bounded range covering the last n ledgers of w.
//
// It counts back from Latest and does not consult Oldest, because Oldest is
// not a reliable floor (see [LedgerWindow.Covers]). Serviceability is the
// stream's to prove: the resolved range is always the full n ledgers that
// were asked for, so a source that cannot serve it fails the run rather than
// quietly returning a shorter dataset that looks complete.
func ResolveLast(w LedgerWindow, n uint32) (start, end uint32, err error) {
	if n == 0 {
		return 0, 0, fmt.Errorf("ledger count must be greater than zero")
	}
	if w.Latest == 0 {
		return 0, 0, fmt.Errorf("source reported no ledgers")
	}
	if n > w.Latest {
		return 0, 0, fmt.Errorf(
			"cannot serve the last %d ledgers: the chain only reaches ledger %d",
			n, w.Latest)
	}

	return w.Latest - n + 1, w.Latest, nil
}
