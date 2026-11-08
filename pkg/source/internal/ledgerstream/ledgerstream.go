// Package ledgerstream adapts the Stellar SDK's ledgerbackend.LedgerStream to
// the nebu source.LedgerSource contract.
//
// It exists so that every nebu ledger source shares one streaming
// implementation. The sources previously each carried their own
// PrepareRange/GetLedger loop — near-identical copies that had to be fixed
// twice, and that skipped what the SDK's stream does on the way in. For
// buffered storage that was not cosmetic: NewBufferedStorageStream reads the
// datastore manifest to discover the bucket's real layout, which the
// hand-rolled loop never did.
//
// This package is internal: it is nebu's own plumbing, not a contract for
// external implementations of source.LedgerSource.
package ledgerstream

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// Range converts a nebu [start, end] pair into an SDK range.
//
// end == 0 means unbounded: stream from start and keep going.
func Range(start, end uint32) (ledgerbackend.Range, error) {
	if start == 0 {
		return ledgerbackend.Range{}, fmt.Errorf("start ledger must be > 0 (got %d)", start)
	}
	if end > 0 && start > end {
		return ledgerbackend.Range{}, fmt.Errorf("invalid range: start (%d) must be <= end (%d)", start, end)
	}
	if end == 0 {
		return ledgerbackend.UnboundedRange(start), nil
	}
	return ledgerbackend.BoundedRange(start, end), nil
}

// Pump streams every ledger in ledgerRange from stream onto out, in order.
//
// The caller owns closing out; Pump only sends. Pump returns nil when a
// bounded range completes, the context error if the caller cancelled, and a
// wrapped error if the stream or a ledger could not be read.
//
// Decoding each record is what makes it safe to hand to another goroutine:
// RawLedgers yields a slice that is only valid until the next iteration step,
// so the bytes must not be forwarded as-is. Unmarshalling into a fresh value
// per record copies everything the consumer will read.
func Pump(
	ctx context.Context,
	stream ledgerbackend.LedgerStream,
	ledgerRange ledgerbackend.Range,
	out chan<- xdr.LedgerCloseMeta,
) error {
	sequence := ledgerRange.From()

	for raw, err := range stream.RawLedgers(ctx, ledgerRange) {
		if err != nil {
			return fmt.Errorf("failed to get ledger %d: %w", sequence, err)
		}

		var ledger xdr.LedgerCloseMeta
		if err := ledger.UnmarshalBinary(raw); err != nil {
			return fmt.Errorf("failed to decode ledger %d: %w", sequence, err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- ledger:
		}

		sequence++
	}

	// RawLedgers stops yielding on cancellation without reporting it, so the
	// context is the only thing that tells a completed range apart from a
	// caller that gave up.
	return ctx.Err()
}
