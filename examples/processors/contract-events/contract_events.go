// Package contract_events provides an origin processor that extracts all contract events from Stellar ledgers.
package contract_events

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
)

// ContractEventsOrigin processes ledgers and extracts contract events
type ContractEventsOriginProto struct {
	networkPassphrase string
	out               chan *ContractEvent
}

// NewContractEventsOriginProto creates a new contract events origin processor
func NewContractEventsOriginProto(networkPassphrase string) *ContractEventsOriginProto {
	return &ContractEventsOriginProto{
		networkPassphrase: networkPassphrase,
		out:               make(chan *ContractEvent, 128),
	}
}

// ProcessLedger implements processor.Origin
func (p *ContractEventsOriginProto) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
	txReader, err := ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(p.networkPassphrase, ledger)
	if err != nil {
		return fmt.Errorf("error creating transaction reader: %w", err)
	}
	defer txReader.Close()

	ledgerSeq := ledger.LedgerSequence()
	closeTime := int64(ledger.LedgerHeaderHistoryEntry().Header.ScpValue.CloseTime)

	// Process each transaction
	txIndex := uint32(0)
	for {
		tx, err := txReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading transaction: %w", err)
		}

		// Get transaction events using SDK helper (handles V3/V4 compatibility)
		txEvents, err := tx.GetTransactionEvents()
		if err != nil {
			// Not a Soroban transaction or no events
			txIndex++
			continue
		}

		txHash := tx.Result.TransactionHash.HexString()
		successful := tx.Result.Successful()

		// Get diagnostic events if available
		var diagnosticEvents []xdr.DiagnosticEvent
		diagEvents, err := tx.GetDiagnosticEvents()
		if err == nil {
			diagnosticEvents = diagEvents
		}

		// Process operation-level events
		for opIndex, opEvents := range txEvents.OperationEvents {
			for eventIdx, event := range opEvents {
				// Only process contract events (skip system events)
				if event.Type != xdr.ContractEventTypeContract {
					continue
				}

				contractEvent, err := p.buildContractEvent(
					event,
					ledgerSeq,
					closeTime,
					txHash,
					txIndex,
					int32(opIndex),
					int32(eventIdx),
					successful,
					diagnosticEvents,
				)
				if err != nil {
					// Log but don't fail on individual event errors
					continue
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case p.out <- contractEvent:
					// Event sent
				}
			}
		}

		// Process transaction-level events (V4 only)
		for eventIdx, txEvent := range txEvents.TransactionEvents {
			if txEvent.Event.Type == xdr.ContractEventTypeContract {
				contractEvent, err := p.buildContractEvent(
					txEvent.Event,
					ledgerSeq,
					closeTime,
					txHash,
					txIndex,
					-1, // Transaction-level events have no operation index
					int32(eventIdx),
					successful,
					diagnosticEvents,
				)
				if err != nil {
					continue
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case p.out <- contractEvent:
					// Event sent
				}
			}
		}

		txIndex++
	}

	return nil
}

// buildContractEvent constructs a ContractEvent protobuf from XDR data
func (p *ContractEventsOriginProto) buildContractEvent(
	event xdr.ContractEvent,
	ledgerSeq uint32,
	closeTime int64,
	txHash string,
	txIndex uint32,
	opIndex int32,
	eventIndex int32,
	successful bool,
	diagnosticEvents []xdr.DiagnosticEvent,
) (*ContractEvent, error) {
	// Extract contract ID
	contractID, err := strkey.Encode(strkey.VersionByteContract, event.ContractId[:])
	if err != nil {
		return nil, fmt.Errorf("error encoding contract ID: %w", err)
	}

	// Decode topics
	var topicDecoded []*ScVal
	for _, topic := range event.Body.V0.Topics {
		decoded := convertXdrScValToProto(topic)
		topicDecoded = append(topicDecoded, decoded)
	}

	// Decode event data
	var dataDecoded *ScVal
	eventData := event.Body.V0.Data
	if eventData.Type != xdr.ScValTypeScvVoid {
		dataDecoded = convertXdrScValToProto(eventData)
	} else {
		dataDecoded = &ScVal{Value: &ScVal_VoidValue{VoidValue: &ScVoid{}}}
	}

	// Determine event type
	var eventType ContractEventType
	switch event.Type {
	case xdr.ContractEventTypeContract:
		eventType = ContractEventType_CONTRACT
	case xdr.ContractEventTypeSystem:
		eventType = ContractEventType_SYSTEM
	case xdr.ContractEventTypeDiagnostic:
		eventType = ContractEventType_DIAGNOSTIC
	default:
		eventType = ContractEventType_CONTRACT
	}

	// Detect specific event type from topics
	detectedEventType := detectEventTypeFromTopics(event.Body.V0.Topics)

	// Build contract event protobuf
	contractEvent := &ContractEvent{
		Timestamp:         closeTime,
		LedgerSequence:    ledgerSeq,
		TransactionHash:   txHash,
		TransactionIndex:  txIndex,
		ContractId:        contractID,
		Type:              eventType,
		EventType:         detectedEventType,
		TopicDecoded:      topicDecoded,
		DataDecoded:       dataDecoded,
		InSuccessfulTx:    successful,
		EventIndex:        eventIndex,
		OperationIndex:    opIndex,
		NetworkPassphrase: p.networkPassphrase,
	}

	// Add diagnostic events if available
	if len(diagnosticEvents) > 0 {
		var diagEvents []*DiagnosticEvent
		for _, diagEvent := range diagnosticEvents {
			if diagEvent.Event.Type == xdr.ContractEventTypeContract {
				diagContractID, err := strkey.Encode(strkey.VersionByteContract, diagEvent.Event.ContractId[:])
				if err != nil {
					continue
				}

				// Decode diagnostic event topics and data
				var diagTopics []*ScVal
				for _, topic := range diagEvent.Event.Body.V0.Topics {
					diagTopics = append(diagTopics, convertXdrScValToProto(topic))
				}

				var diagData *ScVal
				if diagEvent.Event.Body.V0.Data.Type != xdr.ScValTypeScvVoid {
					diagData = convertXdrScValToProto(diagEvent.Event.Body.V0.Data)
				} else {
					diagData = &ScVal{Value: &ScVal_VoidValue{VoidValue: &ScVoid{}}}
				}

				diagEventType := ContractEventType_CONTRACT
				if diagEvent.Event.Type == xdr.ContractEventTypeSystem {
					diagEventType = ContractEventType_SYSTEM
				} else if diagEvent.Event.Type == xdr.ContractEventTypeDiagnostic {
					diagEventType = ContractEventType_DIAGNOSTIC
				}

				detectedType := detectEventTypeFromTopics(diagEvent.Event.Body.V0.Topics)

				diagEvents = append(diagEvents, &DiagnosticEvent{
					ContractId:               diagContractID,
					Type:                     diagEventType,
					EventType:                detectedType,
					TopicDecoded:             diagTopics,
					DataDecoded:              diagData,
					InSuccessfulContractCall: diagEvent.InSuccessfulContractCall,
				})
			}
		}
		contractEvent.DiagnosticEvents = diagEvents
	}

	return contractEvent, nil
}

// detectEventTypeFromTopics attempts to determine the event type from topics
func detectEventTypeFromTopics(topics []xdr.ScVal) string {
	// Check topics for common event type patterns
	for _, topic := range topics {
		if topic.Type == xdr.ScValTypeScvSymbol {
			sym := string(topic.MustSym())
			// Return the first symbol as the event type
			switch sym {
			case "transfer", "Transfer":
				return "transfer"
			case "mint", "Mint":
				return "mint"
			case "burn", "Burn":
				return "burn"
			case "swap", "Swap":
				return "swap"
			case "sync", "Sync":
				return "sync"
			case "deposit", "Deposit":
				return "deposit"
			case "withdraw", "Withdraw":
				return "withdraw"
			case "approval", "Approval":
				return "approval"
			case "stake", "Stake":
				return "stake"
			case "unstake", "Unstake":
				return "unstake"
			case "claim", "Claim":
				return "claim"
			case "reward", "Reward":
				return "reward"
			case "fee", "Fee":
				return "fee"
			default:
				return sym
			}
		}
	}
	return "unknown"
}

// convertXdrScValToProto converts an XDR ScVal to a protobuf ScVal
func convertXdrScValToProto(val xdr.ScVal) *ScVal {
	switch val.Type {
	case xdr.ScValTypeScvBool:
		return &ScVal{Value: &ScVal_BoolValue{BoolValue: val.MustB()}}
	case xdr.ScValTypeScvVoid:
		return &ScVal{Value: &ScVal_VoidValue{VoidValue: &ScVoid{}}}
	case xdr.ScValTypeScvU32:
		return &ScVal{Value: &ScVal_U32Value{U32Value: uint32(val.MustU32())}}
	case xdr.ScValTypeScvI32:
		return &ScVal{Value: &ScVal_I32Value{I32Value: int32(val.MustI32())}}
	case xdr.ScValTypeScvU64:
		return &ScVal{Value: &ScVal_U64Value{U64Value: uint64(val.MustU64())}}
	case xdr.ScValTypeScvI64:
		return &ScVal{Value: &ScVal_I64Value{I64Value: int64(val.MustI64())}}
	case xdr.ScValTypeScvU128:
		u128 := val.MustU128()
		return &ScVal{Value: &ScVal_U128Value{U128Value: fmt.Sprintf("0x%032x%032x", u128.Hi, u128.Lo)}}
	case xdr.ScValTypeScvI128:
		i128 := val.MustI128()
		return &ScVal{Value: &ScVal_I128Value{I128Value: fmt.Sprintf("0x%032x%032x", i128.Hi, i128.Lo)}}
	case xdr.ScValTypeScvU256:
		u256 := val.MustU256()
		return &ScVal{Value: &ScVal_U256Value{U256Value: fmt.Sprintf("0x%032x%032x%032x%032x", u256.HiHi, u256.HiLo, u256.LoHi, u256.LoLo)}}
	case xdr.ScValTypeScvI256:
		i256 := val.MustI256()
		return &ScVal{Value: &ScVal_I256Value{I256Value: fmt.Sprintf("0x%032x%032x%032x%032x", i256.HiHi, i256.HiLo, i256.LoHi, i256.LoLo)}}
	case xdr.ScValTypeScvBytes:
		return &ScVal{Value: &ScVal_BytesValue{BytesValue: val.MustBytes()}}
	case xdr.ScValTypeScvString:
		return &ScVal{Value: &ScVal_StringValue{StringValue: string(val.MustStr())}}
	case xdr.ScValTypeScvSymbol:
		return &ScVal{Value: &ScVal_SymbolValue{SymbolValue: string(val.MustSym())}}
	case xdr.ScValTypeScvVec:
		vec := val.MustVec()
		var values []*ScVal
		for _, item := range *vec {
			values = append(values, convertXdrScValToProto(item))
		}
		return &ScVal{Value: &ScVal_VecValue{VecValue: &ScVec{Values: values}}}
	case xdr.ScValTypeScvMap:
		scMap := val.MustMap()
		var entries []*ScMapEntry
		for _, entry := range *scMap {
			entries = append(entries, &ScMapEntry{
				Key: convertXdrScValToProto(entry.Key),
				Val: convertXdrScValToProto(entry.Val),
			})
		}
		return &ScVal{Value: &ScVal_MapValue{MapValue: &ScMap{Entries: entries}}}
	case xdr.ScValTypeScvAddress:
		addr := val.MustAddress()
		switch addr.Type {
		case xdr.ScAddressTypeScAddressTypeAccount:
			accountID := addr.MustAccountId()
			return &ScVal{Value: &ScVal_AddressValue{AddressValue: accountID.Address()}}
		case xdr.ScAddressTypeScAddressTypeContract:
			contractID := addr.MustContractId()
			encoded, err := strkey.Encode(strkey.VersionByteContract, contractID[:])
			if err != nil {
				return &ScVal{Value: &ScVal_VoidValue{VoidValue: &ScVoid{}}}
			}
			return &ScVal{Value: &ScVal_AddressValue{AddressValue: encoded}}
		}
	case xdr.ScValTypeScvLedgerKeyContractInstance:
		return &ScVal{Value: &ScVal_LedgerKeyValue{LedgerKeyValue: "contract_instance"}}
	case xdr.ScValTypeScvLedgerKeyNonce:
		return &ScVal{Value: &ScVal_LedgerKeyValue{LedgerKeyValue: "nonce"}}
	case xdr.ScValTypeScvTimepoint:
		tp := time.Unix(int64(val.MustTimepoint()), 0)
		return &ScVal{Value: &ScVal_TimepointValue{TimepointValue: tp.Format(time.RFC3339)}}
	case xdr.ScValTypeScvDuration:
		return &ScVal{Value: &ScVal_DurationValue{DurationValue: uint64(val.MustDuration())}}
	}

	return &ScVal{Value: &ScVal_VoidValue{VoidValue: &ScVoid{}}}
}

// Out returns the output channel for contract events
func (p *ContractEventsOriginProto) Out() <-chan *ContractEvent {
	return p.out
}

// Close closes the output channel
func (p *ContractEventsOriginProto) Close() {
	close(p.out)
}

// Name returns the processor name
func (p *ContractEventsOriginProto) Name() string {
	return "contract-events"
}

// Type returns the processor type
func (p *ContractEventsOriginProto) Type() processor.Type {
	return processor.TypeOrigin
}
