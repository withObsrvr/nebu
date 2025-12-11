// Package token_transfer provides an origin processor for Stellar token transfer events.
package token_transfer

import (
	"context"

	"github.com/stellar/go-stellar-sdk/processors/token_transfer"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
)

// Origin is an origin processor that wraps Stellar's token_transfer.EventsProcessor.
// It consumes ledgers and emits TokenTransferEvent protobuf messages.
type Origin struct {
	passphrase string
	eventsProc *token_transfer.EventsProcessor
	emitter    *processor.Emitter[*token_transfer.TokenTransferEvent]
}

// NewOrigin creates a new token transfer origin processor.
// The passphrase should be the network passphrase (e.g., network.PublicNetworkPassphrase).
func NewOrigin(passphrase string) *Origin {
	return &Origin{
		passphrase: passphrase,
		eventsProc: token_transfer.NewEventsProcessor(passphrase),
		emitter:    processor.NewEmitter[*token_transfer.TokenTransferEvent](1024),
	}
}

// Name implements processor.Processor.
func (o *Origin) Name() string {
	return "stellar/token-transfer"
}

// Type implements processor.Processor.
func (o *Origin) Type() processor.Type {
	return processor.TypeOrigin
}

// Out returns the output channel for consuming emitted events.
// This is useful when embedding this processor in a single-process pipeline.
func (o *Origin) Out() <-chan *token_transfer.TokenTransferEvent {
	return o.emitter.Out()
}

// Close closes the emitter, signaling that no more events will be produced.
func (o *Origin) Close() {
	o.emitter.Close()
}

// ProcessLedger implements processor.Origin.
// It extracts token transfer events from the ledger and emits them.
func (o *Origin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
	// Extract events from the ledger
	events, err := o.eventsProc.EventsFromLedger(ledger)
	if err != nil {
		return err
	}

	// Emit each event
	for _, ev := range events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			o.emitter.Emit(ev)
		}
	}

	return nil
}
