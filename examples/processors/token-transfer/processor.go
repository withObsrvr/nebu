// Package token_transfer provides an origin processor for Stellar token transfer events.
package token_transfer

import (
	"context"

	"github.com/stellar/go-stellar-sdk/asset"
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/processors/token_transfer"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"

	ttpb "github.com/withObsrvr/nebu/examples/processors/token-transfer/proto"
)

// Origin is an origin processor that wraps Stellar's token_transfer.EventsProcessor.
// It consumes ledgers and emits our own TokenTransferEvent protobuf messages.
type Origin struct {
	passphrase string
	eventsProc *token_transfer.EventsProcessor
	emitter    *processor.Emitter[*ttpb.TokenTransferEvent]
}

// NewOrigin creates a new token transfer origin processor.
// The passphrase should be the network passphrase (e.g., network.PublicNetworkPassphrase).
func NewOrigin(passphrase string) *Origin {
	return &Origin{
		passphrase: passphrase,
		eventsProc: token_transfer.NewEventsProcessor(passphrase),
		emitter:    processor.NewEmitter[*ttpb.TokenTransferEvent](1024),
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
func (o *Origin) Out() <-chan *ttpb.TokenTransferEvent {
	return o.emitter.Out()
}

// Close closes the emitter, signaling that no more events will be produced.
func (o *Origin) Close() {
	o.emitter.Close()
}

// extractAssetInfo extracts asset code and issuer from Stellar SDK Asset
func extractAssetInfo(a *asset.Asset) (assetCode, assetIssuer string) {
	if a == nil {
		return "", ""
	}

	// Check if it's a native asset (XLM)
	if a.GetNative() {
		return "XLM", ""
	}

	// Otherwise it's an issued asset
	if issuedAsset := a.GetIssuedAsset(); issuedAsset != nil {
		return issuedAsset.GetAssetCode(), issuedAsset.GetIssuer()
	}

	return "", ""
}

// convertEvent converts a Stellar SDK TokenTransferEvent to our proto event,
// adding the InSuccessfulTx field.
func convertEvent(sdkEvent *token_transfer.TokenTransferEvent, inSuccessfulTx bool) *ttpb.TokenTransferEvent {
	if sdkEvent == nil || sdkEvent.Meta == nil {
		return nil
	}

	// Convert operation index (handle optional field)
	opIndex := uint32(0)
	if sdkEvent.Meta.OperationIndex != nil {
		opIndex = *sdkEvent.Meta.OperationIndex
	}

	// Convert metadata
	meta := &ttpb.EventMeta{
		LedgerSequence:   sdkEvent.Meta.LedgerSequence,
		ClosedAtUnix:     sdkEvent.Meta.ClosedAt.AsTime().Unix(),
		TxHash:           sdkEvent.Meta.TxHash,
		TransactionIndex: sdkEvent.Meta.TransactionIndex,
		OperationIndex:   opIndex,
		ContractAddress:  sdkEvent.Meta.ContractAddress,
		InSuccessfulTx:   inSuccessfulTx,
	}

	// Create our proto event
	pbEvent := &ttpb.TokenTransferEvent{
		Meta: meta,
	}

	// Convert event type
	switch ev := sdkEvent.Event.(type) {
	case *token_transfer.TokenTransferEvent_Transfer:
		assetCode, assetIssuer := extractAssetInfo(ev.Transfer.Asset)
		pbEvent.Event = &ttpb.TokenTransferEvent_Transfer{
			Transfer: &ttpb.Transfer{
				From:        ev.Transfer.From,
				To:          ev.Transfer.To,
				AssetCode:   assetCode,
				AssetIssuer: assetIssuer,
				Amount:      ev.Transfer.Amount,
			},
		}
	case *token_transfer.TokenTransferEvent_Mint:
		assetCode, assetIssuer := extractAssetInfo(ev.Mint.Asset)
		pbEvent.Event = &ttpb.TokenTransferEvent_Mint{
			Mint: &ttpb.Mint{
				To:          ev.Mint.To,
				AssetCode:   assetCode,
				AssetIssuer: assetIssuer,
				Amount:      ev.Mint.Amount,
			},
		}
	case *token_transfer.TokenTransferEvent_Burn:
		assetCode, assetIssuer := extractAssetInfo(ev.Burn.Asset)
		pbEvent.Event = &ttpb.TokenTransferEvent_Burn{
			Burn: &ttpb.Burn{
				From:        ev.Burn.From,
				AssetCode:   assetCode,
				AssetIssuer: assetIssuer,
				Amount:      ev.Burn.Amount,
			},
		}
	case *token_transfer.TokenTransferEvent_Clawback:
		assetCode, assetIssuer := extractAssetInfo(ev.Clawback.Asset)
		pbEvent.Event = &ttpb.TokenTransferEvent_Clawback{
			Clawback: &ttpb.Clawback{
				From:        ev.Clawback.From,
				AssetCode:   assetCode,
				AssetIssuer: assetIssuer,
				Amount:      ev.Clawback.Amount,
			},
		}
	case *token_transfer.TokenTransferEvent_Fee:
		assetCode, assetIssuer := extractAssetInfo(ev.Fee.Asset)
		pbEvent.Event = &ttpb.TokenTransferEvent_Fee{
			Fee: &ttpb.Fee{
				From:        ev.Fee.From,
				AssetCode:   assetCode,
				AssetIssuer: assetIssuer,
				Amount:      ev.Fee.Amount,
			},
		}
	}

	return pbEvent
}

// ProcessLedger implements processor.Origin.
// It extracts token transfer events from the ledger and emits them.
func (o *Origin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
	// Build a map of transaction hash -> success status
	txSuccessMap := make(map[string]bool)
	reader, err := ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(o.passphrase, ledger)
	if err != nil {
		return err
	}
	defer reader.Close()

	for {
		tx, err := reader.Read()
		if err != nil {
			break // End of transactions
		}
		txSuccessMap[tx.Result.TransactionHash.HexString()] = tx.Result.Successful()
	}

	// Extract events from the ledger using Stellar SDK
	sdkEvents, err := o.eventsProc.EventsFromLedger(ledger)
	if err != nil {
		return err
	}

	// Convert SDK events to our proto events with InSuccessfulTx field
	for _, sdkEvent := range sdkEvents {
		successful := true // Default to true
		if sdkEvent.Meta != nil {
			if found, ok := txSuccessMap[sdkEvent.Meta.TxHash]; ok {
				successful = found
			}
		}

		pbEvent := convertEvent(sdkEvent, successful)
		if pbEvent == nil {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			o.emitter.Emit(pbEvent)
		}
	}

	return nil
}
