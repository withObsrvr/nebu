// Package contract_invocation provides an origin processor for Stellar contract invocations.
package contract_invocation

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"

	cipb "github.com/withObsrvr/nebu/examples/processors/contract-invocation/proto"
)

// Origin is an origin processor that extracts contract invocation events.
type Origin struct {
	passphrase string
	emitter    *processor.Emitter[*cipb.ContractInvocation]
}

// NewOrigin creates a new contract invocation origin processor.
func NewOrigin(passphrase string) *Origin {
	return &Origin{
		passphrase: passphrase,
		emitter:    processor.NewEmitter[*cipb.ContractInvocation](1024),
	}
}

// Name implements processor.Processor.
func (o *Origin) Name() string {
	return "stellar/contract-invocation"
}

// Type implements processor.Processor.
func (o *Origin) Type() processor.Type {
	return processor.TypeOrigin
}

// Out returns the output channel for consuming emitted events.
func (o *Origin) Out() <-chan *cipb.ContractInvocation {
	return o.emitter.Out()
}

// Close closes the emitter, signaling that no more events will be produced.
func (o *Origin) Close() {
	o.emitter.Close()
}

// ProcessLedger implements processor.Origin.
func (o *Origin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
	sequence := ledger.LedgerSequence()
	closeTime := time.Unix(int64(ledger.LedgerHeaderHistoryEntry().Header.ScpValue.CloseTime), 0)

	// Build transaction success map
	txSuccessMap := make(map[string]bool)
	reader, err := ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(o.passphrase, ledger)
	if err != nil {
		processor.ReportWarning(ctx, o.Name(), fmt.Errorf("ledger %d: create tx reader: %w", sequence, err))
		return
	}
	defer reader.Close()

	for {
		tx, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			processor.ReportWarning(ctx, o.Name(), fmt.Errorf("ledger %d: read tx (success map): %w", sequence, err))
			return
		}
		txSuccessMap[tx.Result.TransactionHash.HexString()] = tx.Result.Successful()
	}

	// Re-create reader to process transactions again
	reader, err = ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(o.passphrase, ledger)
	if err != nil {
		processor.ReportWarning(ctx, o.Name(), fmt.Errorf("ledger %d: re-create tx reader: %w", sequence, err))
		return
	}
	defer reader.Close()

	// Process each transaction
	for {
		tx, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			processor.ReportWarning(ctx, o.Name(), fmt.Errorf("ledger %d: read tx: %w", sequence, err))
			return
		}

		// Check each operation for contract invocations
		for opIndex, op := range tx.Envelope.Operations() {
			if op.Body.Type == xdr.OperationTypeInvokeHostFunction {
				invocation, err := o.processContractInvocation(tx, opIndex, op, sequence, closeTime, txSuccessMap)
				if err != nil {
					continue
				}

				if invocation != nil {
					select {
					case <-ctx.Done():
						return
					default:
						o.emitter.Emit(invocation)
					}
				}
			}
		}
	}
}

func (o *Origin) processContractInvocation(
	tx ingest.LedgerTransaction,
	opIndex int,
	op xdr.Operation,
	sequence uint32,
	closeTime time.Time,
	txSuccessMap map[string]bool,
) (*cipb.ContractInvocation, error) {
	invokeHostFunction := op.Body.MustInvokeHostFunctionOp()

	// Get the invoking account
	var invokingAccount xdr.AccountId
	if op.SourceAccount != nil {
		invokingAccount = op.SourceAccount.ToAccountId()
	} else {
		invokingAccount = tx.Envelope.SourceAccount().ToAccountId()
	}

	contractID, err := o.extractContractID(tx, invokeHostFunction)
	if err != nil {
		return nil, err
	}

	// Determine if invocation was successful
	successful := false
	if tx.Result.Result.Result.Results != nil {
		if results := *tx.Result.Result.Result.Results; len(results) > opIndex {
			if result := results[opIndex]; result.Tr != nil {
				if invokeResult, ok := result.Tr.GetInvokeHostFunctionResult(); ok {
					successful = invokeResult.Code == xdr.InvokeHostFunctionResultCodeInvokeHostFunctionSuccess
				}
			}
		}
	}

	// Get transaction success status
	txHash := tx.Result.TransactionHash.HexString()
	inSuccessfulTx := txSuccessMap[txHash]

	// Create invocation record
	invocation := &cipb.ContractInvocation{
		Meta: &cipb.EventMeta{
			LedgerSequence:   sequence,
			ClosedAtUnix:     closeTime.Unix(),
			TxHash:           txHash,
			TransactionIndex: uint32(tx.Index),
			OperationIndex:   uint32(opIndex),
			InSuccessfulTx:   inSuccessfulTx,
		},
		ContractId:      contractID,
		InvokingAccount: invokingAccount.Address(),
		Successful:      successful,
	}

	// Extract function name and arguments
	if function := invokeHostFunction.HostFunction; function.Type == xdr.HostFunctionTypeHostFunctionTypeInvokeContract {
		invokeContract := function.MustInvokeContract()

		// Extract function name
		invocation.FunctionName = ExtractFunctionName(invokeContract)

		// Extract arguments
		if len(invokeContract.Args) > 0 {
			invocation.Arguments = ExtractArguments(invokeContract.Args)
		}
	}

	// Extract diagnostic events
	invocation.DiagnosticEvents = o.extractDiagnosticEvents(tx)

	// Extract contract calls
	invocation.ContractCalls = o.extractContractCalls(tx, opIndex, invokeHostFunction, contractID)

	// Extract state changes
	invocation.StateChanges = o.extractStateChanges(tx)

	// Extract TTL extensions (placeholder for now)
	invocation.TtlExtensions = o.extractTtlExtensions(tx)

	return invocation, nil
}

func (o *Origin) extractDiagnosticEvents(tx ingest.LedgerTransaction) []*cipb.DiagnosticEvent {
	var events []*cipb.DiagnosticEvent

	// Check if we have diagnostic events in the transaction meta
	diagnosticEvents, err := tx.GetDiagnosticEvents()
	if err != nil || len(diagnosticEvents) == 0 {
		return events
	}

	for _, diagEvent := range diagnosticEvents {
		if diagEvent.Event.ContractId == nil {
			continue
		}

		contractID, err := EncodeContractID(diagEvent.Event.ContractId)
		if err != nil {
			continue
		}

		// Decode topics
		var topics []string
		if diagEvent.Event.Body.V == 0 && diagEvent.Event.Body.V0 != nil {
			for _, topic := range diagEvent.Event.Body.V0.Topics {
				topics = append(topics, ConvertScValToString(topic))
			}
		}

		// Decode data
		var data string
		if diagEvent.Event.Body.V == 0 && diagEvent.Event.Body.V0 != nil {
			data = ConvertScValToString(diagEvent.Event.Body.V0.Data)
		}

		events = append(events, &cipb.DiagnosticEvent{
			ContractId:       contractID,
			Topics:           topics,
			Data:             data,
			InSuccessfulCall: diagEvent.InSuccessfulContractCall,
			EventType:        uint32(diagEvent.Event.Type),
		})
	}

	return events
}

func (o *Origin) extractContractCalls(
	tx ingest.LedgerTransaction,
	opIndex int,
	invokeOp xdr.InvokeHostFunctionOp,
	mainContract string,
) []*cipb.ContractCall {
	var calls []*cipb.ContractCall

	// Extract from authorization data
	executionOrder := 0
	for _, authEntry := range invokeOp.Auth {
		authType := "source_account"
		if authEntry.Credentials.Type == xdr.SorobanCredentialsTypeSorobanCredentialsAddress {
			authType = "contract"
		}

		o.processAuthorizationTree(
			&authEntry.RootInvocation,
			mainContract,
			&calls,
			0,
			authType,
			&executionOrder,
		)
	}

	return calls
}

func (o *Origin) processAuthorizationTree(
	invocation *xdr.SorobanAuthorizedInvocation,
	fromContract string,
	calls *[]*cipb.ContractCall,
	depth int,
	authType string,
	executionOrder *int,
) {
	if invocation == nil {
		return
	}

	var contractID string
	var functionName string
	var args []string

	if invocation.Function.Type == xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn {
		contractFn := invocation.Function.ContractFn

		var err error
		contractID, err = EncodeContractID(contractFn.ContractAddress.ContractId)
		if err != nil {
			return
		}

		// Get function name
		functionName = string(contractFn.FunctionName)

		// Extract arguments
		if len(contractFn.Args) > 0 {
			args = ExtractArguments(contractFn.Args)
		}
	}

	// Record the call if we have both from and to contracts (skip self-calls)
	if fromContract != "" && contractID != "" && fromContract != contractID {
		*calls = append(*calls, &cipb.ContractCall{
			FromContract:   fromContract,
			ToContract:     contractID,
			Function:       functionName,
			Arguments:      args,
			CallDepth:      uint32(depth),
			AuthType:       authType,
			Successful:     true,
			ExecutionOrder: uint32(*executionOrder),
		})
		*executionOrder++
	}

	// Process sub-invocations recursively
	for _, subInvocation := range invocation.SubInvocations {
		o.processAuthorizationTree(
			&subInvocation,
			contractID,
			calls,
			depth+1,
			authType,
			executionOrder,
		)
	}
}

func (o *Origin) extractStateChanges(tx ingest.LedgerTransaction) []*cipb.StateChange {
	var changes []*cipb.StateChange

	// Extract state changes from ledger changes in the transaction meta
	txChanges, err := tx.GetChanges()
	if err != nil {
		return changes
	}

	for _, change := range txChanges {
		// We're only interested in contract data changes
		if change.Type != xdr.LedgerEntryTypeContractData {
			continue
		}

		switch change.ChangeType {
		case xdr.LedgerEntryChangeTypeLedgerEntryCreated:
			if change.Post != nil && change.Post.Data.Type == xdr.LedgerEntryTypeContractData {
				contractData := change.Post.Data.ContractData
				if contractData != nil {
					if stateChange := o.extractStateChangeFromContractData(*contractData, xdr.ScVal{}, contractData.Val, "create"); stateChange != nil {
						changes = append(changes, stateChange)
					}
				}
			}

		case xdr.LedgerEntryChangeTypeLedgerEntryUpdated:
			if change.Pre != nil && change.Post != nil &&
				change.Pre.Data.Type == xdr.LedgerEntryTypeContractData &&
				change.Post.Data.Type == xdr.LedgerEntryTypeContractData {

				preData := change.Pre.Data.ContractData
				postData := change.Post.Data.ContractData
				if preData != nil && postData != nil {
					if stateChange := o.extractStateChangeFromContractData(*postData, preData.Val, postData.Val, "update"); stateChange != nil {
						changes = append(changes, stateChange)
					}
				}
			}

		case xdr.LedgerEntryChangeTypeLedgerEntryRemoved:
			if change.Pre != nil && change.Pre.Data.Type == xdr.LedgerEntryTypeContractData {
				contractData := change.Pre.Data.ContractData
				if contractData != nil {
					if stateChange := o.extractStateChangeFromContractData(*contractData, contractData.Val, xdr.ScVal{}, "delete"); stateChange != nil {
						changes = append(changes, stateChange)
					}
				}
			}
		}
	}

	return changes
}

func (o *Origin) extractStateChangeFromContractData(
	contractData xdr.ContractDataEntry,
	oldValueRaw, newValueRaw xdr.ScVal,
	operation string,
) *cipb.StateChange {
	contractID, err := EncodeContractID(contractData.Contract.ContractId)
	if err != nil || contractID == "" {
		return nil
	}

	key := DescribeContractDataKey(contractData, o.passphrase)

	// Decode values
	var oldValue, newValue string
	if operation != "create" {
		oldValue = ConvertScValToString(oldValueRaw)
	}
	if operation != "delete" {
		newValue = ConvertScValToString(newValueRaw)
	}

	return &cipb.StateChange{
		ContractId: contractID,
		Key:        key,
		OldValue:   oldValue,
		NewValue:   newValue,
		Operation:  operation,
	}
}

func (o *Origin) extractTtlExtensions(tx ingest.LedgerTransaction) []*cipb.TtlExtension {
	// TTL extensions are not currently extracted in this simplified version.
	return nil
}

func (o *Origin) extractContractID(tx ingest.LedgerTransaction, invokeHostFunction xdr.InvokeHostFunctionOp) (string, error) {
	if function := invokeHostFunction.HostFunction; function.Type == xdr.HostFunctionTypeHostFunctionTypeInvokeContract {
		return EncodeContractID(function.MustInvokeContract().ContractAddress.ContractId)
	}

	if contractID, ok := tx.ContractIdFromTxEnvelope(); ok && contractID != "" {
		return contractID, nil
	}

	return "", nil
}
