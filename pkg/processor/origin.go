package processor

import (
	"context"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Origin processors consume LedgerCloseMeta from Stellar and emit typed events.
//
// Origin processors are the entry point for data into a nebu pipeline.
// They typically wrap Stellar SDK processors (like token_transfer.EventsProcessor)
// and convert raw ledger data into structured protobuf events.
//
// # Error handling
//
// ProcessLedger does not return an error. A pipeline that runs for
// hours over millions of ledgers must not die because one ledger had a
// malformed field. Per-ledger errors should be reported via
// [ReportWarning] and the method should return normally; the pipeline
// will continue with the next ledger. Unrecoverable errors (e.g., the
// processor has lost a required resource) should be reported via
// [ReportFatal] and the method should return immediately; the runtime
// will halt the pipeline.
//
// Implementations should also respect context cancellation, returning
// as soon as possible when ctx.Done() is observed.
//
// Example implementation:
//
//	type TokenTransferOrigin struct {
//	    name       string
//	    eventsProc *token_transfer.EventsProcessor
//	    emitter    *Emitter[*token_transfer.TokenTransferEvent]
//	}
//
//	func (o *TokenTransferOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
//	    events, err := o.eventsProc.EventsFromLedger(ledger)
//	    if err != nil {
//	        processor.ReportWarning(ctx, o.name, fmt.Errorf("ledger %d: %w", ledger.LedgerSequence(), err))
//	        return
//	    }
//	    for _, ev := range events {
//	        select {
//	        case <-ctx.Done():
//	            return
//	        default:
//	            o.emitter.Emit(ev)
//	        }
//	    }
//	}
type Origin interface {
	Processor

	// ProcessLedger processes a single ledger and emits events via the
	// processor's internal Emitter. The implementation must respect
	// context cancellation and report errors through the Reporter
	// attached to ctx (see [ReportWarning], [ReportFatal]).
	ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta)
}
