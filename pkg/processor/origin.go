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
// Example implementation:
//
//	type TokenTransferOrigin struct {
//	    eventsProc *token_transfer.EventsProcessor
//	    emitter    *Emitter[*token_transfer.TokenTransferEvent]
//	}
//
//	func (o *TokenTransferOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
//	    events, err := o.eventsProc.EventsFromLedger(ledger)
//	    if err != nil {
//	        return err
//	    }
//	    for _, ev := range events {
//	        select {
//	        case <-ctx.Done():
//	            return ctx.Err()
//	        default:
//	            o.emitter.Emit(ev)
//	        }
//	    }
//	    return nil
//	}
type Origin interface {
	Processor

	// ProcessLedger processes a single ledger and emits events.
	// The implementation should respect context cancellation.
	//
	// Errors returned indicate processing failure; the runtime may
	// stop the pipeline on error depending on configuration.
	ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error
}
