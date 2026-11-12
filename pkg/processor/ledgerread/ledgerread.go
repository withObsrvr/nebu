// Package ledgerread iterates the contents of a ledger using the Stellar
// SDK's ingest readers, reporting read failures through the nebu
// [processor.Reporter] attached to the context.
//
// It exists because every origin processor needs the same loop — create a
// reader, read until io.EOF, close — and the loop has one failure mode that
// is easy to get wrong: treating a read error as end-of-ledger. A processor
// that writes
//
//	for {
//	    tx, err := reader.Read()
//	    if err != nil {
//	        break
//	    }
//	    ...
//	}
//
// silently truncates the ledger on a decode failure and emits no warning at
// all, so the run looks clean. Keeping the loop here means that distinction
// is made once, in a tested place, the way [processor.LedgerCloseTimeMs] does
// for close times.
//
// This package is deliberately separate from [processor] itself: it imports
// the SDK's ingest package, which pulls in the cloud-storage and history
// archive dependencies. Transforms and sinks that only need the processor
// interfaces should keep importing [processor] directly and pay nothing for
// those.
package ledgerread

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
)

// Transactions yields every transaction in ledger, in ledger order.
//
// Failures are reported through the [processor.Reporter] on ctx as warnings
// and never returned: an origin's ProcessLedger has no error to return, so a
// reader that failed to open yields nothing, and an individual transaction
// that cannot be read is skipped with a warning naming it.
//
// Skipping rather than abandoning the ledger is deliberate. The SDK's reader
// advances its index before returning a read error, so the transactions after
// a bad one are still readable, and losing one transaction is a much smaller
// hole in the output than losing every transaction that followed it. Whether
// a run with holes is acceptable at all is a question for the execution
// policy, not for this loop — every skip is reported, so the policy can see
// it.
//
// That makes this the wrong function for building an index. A caller that
// collects transactions into a map and later looks one up by key cannot see
// the skip: the lookup simply misses, and a miss usually falls through to a
// default, so a partial read becomes wrong data rather than less data. Use
// [AllTransactions] for that shape — it reports whether the ledger was read
// to the end, so the caller can decline to use a partial index.
//
// Iteration stops as soon as ctx is cancelled, and the reader is closed on
// every exit path, including an early break by the caller.
func Transactions(
	ctx context.Context,
	processorName, networkPassphrase string,
	ledger xdr.LedgerCloseMeta,
) iter.Seq[ingest.LedgerTransaction] {
	return func(yield func(ingest.LedgerTransaction) bool) {
		sequence := ledger.LedgerSequence()

		reader, err := ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(networkPassphrase, ledger)
		if err != nil {
			processor.ReportWarning(ctx, processorName,
				fmt.Errorf("ledger %d: open transaction reader: %w", sequence, err))
			return
		}
		defer closeReader(ctx, processorName, sequence, "transaction", reader.Close)

		for {
			if ctx.Err() != nil {
				return
			}

			tx, err := reader.Read()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				processor.ReportWarning(ctx, processorName,
					fmt.Errorf("ledger %d: read transaction: %w", sequence, err))
				continue
			}

			if !yield(tx) {
				return
			}
		}
	}
}

// AllTransactions reads every transaction in ledger and reports whether it
// reached the end.
//
// Use it instead of [Transactions] whenever the transactions become an index
// that something later consults by key — a transaction-hash to success map,
// most commonly. [Transactions] skips an unreadable transaction and carries
// on, which is right when each transaction is emitted on its own and wrong
// here: the skip is invisible at lookup time, and a missing key usually
// resolves to a zero value or an explicit default, so the output ends up
// confidently mislabelled rather than merely short.
//
// ok is false when the reader could not be opened or a transaction could not
// be read, and in both cases the problem has already been reported as a
// warning through the [processor.Reporter] on ctx. A caller that gets false
// should decline to use the result rather than use part of it:
//
//	txs, ok := ledgerread.AllTransactions(ctx, o.Name(), o.passphrase, ledger)
//	if !ok {
//	    return // already reported; an incomplete index is worse than none
//	}
//
// The transactions read before the failure are still returned, for a caller
// that genuinely wants them — a count, say, where a short count is obviously
// short. They are returned as a slice rather than an iterator because the
// completeness of the whole ledger is the answer being asked for, and that is
// not known until the end.
func AllTransactions(
	ctx context.Context,
	processorName, networkPassphrase string,
	ledger xdr.LedgerCloseMeta,
) ([]ingest.LedgerTransaction, bool) {
	sequence := ledger.LedgerSequence()

	reader, err := ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(networkPassphrase, ledger)
	if err != nil {
		processor.ReportWarning(ctx, processorName,
			fmt.Errorf("ledger %d: open transaction reader: %w", sequence, err))
		return nil, false
	}
	defer closeReader(ctx, processorName, sequence, "transaction", reader.Close)

	var transactions []ingest.LedgerTransaction
	for {
		if err := ctx.Err(); err != nil {
			// A cancelled read is not a complete one, and saying otherwise
			// would let a shutdown produce a confidently wrong index.
			return transactions, false
		}

		tx, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return transactions, true
		}
		if err != nil {
			processor.ReportWarning(ctx, processorName,
				fmt.Errorf("ledger %d: read transaction: %w", sequence, err))
			return transactions, false
		}

		transactions = append(transactions, tx)
	}
}

// Changes yields every ledger-entry change in ledger, in the SDK's order:
// fee changes, then transaction meta changes, then post-apply fee changes,
// then protocol upgrade changes.
//
// Failures are reported as warnings through the [processor.Reporter] on ctx,
// as in [Transactions]. Unlike Transactions, a read failure ends the
// iteration rather than skipping ahead: the change stream is a state machine
// over the transactions, so a failure mid-stream can drop a whole category of
// changes for a transaction while the surrounding changes still arrive. A
// partial change set is a more misleading result than a short one, so this
// stops and says so.
//
// Iteration stops as soon as ctx is cancelled, and the reader is closed on
// every exit path, including an early break by the caller.
func Changes(
	ctx context.Context,
	processorName, networkPassphrase string,
	ledger xdr.LedgerCloseMeta,
) iter.Seq[ingest.Change] {
	return func(yield func(ingest.Change) bool) {
		sequence := ledger.LedgerSequence()

		reader, err := ingest.NewLedgerChangeReaderFromLedgerCloseMeta(networkPassphrase, ledger)
		if err != nil {
			processor.ReportWarning(ctx, processorName,
				fmt.Errorf("ledger %d: open change reader: %w", sequence, err))
			return
		}
		defer closeReader(ctx, processorName, sequence, "change", reader.Close)

		for {
			if ctx.Err() != nil {
				return
			}

			change, err := reader.Read()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				processor.ReportWarning(ctx, processorName,
					fmt.Errorf("ledger %d: read change: %w", sequence, err))
				return
			}

			if !yield(change) {
				return
			}
		}
	}
}

// closeReader reports a close failure rather than discarding it. A reader
// that fails to release is not a reason to fail the ledger, but it is also
// not nothing: swallowing it hides a resource leak behind a clean run.
func closeReader(ctx context.Context, processorName string, sequence uint32, kind string, close func() error) {
	if err := close(); err != nil {
		processor.ReportWarning(ctx, processorName,
			fmt.Errorf("ledger %d: close %s reader: %w", sequence, kind, err))
	}
}
