package cli

import (
	"context"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/runtime"
)

func withStructuredStatusHook(hooks []runtime.Hooks, commandStatus *commandStatus, target uint32) []runtime.Hooks {
	if commandStatus == nil || commandStatus.structured == nil {
		return hooks
	}
	statusHook := runtime.Hooks{
		BeforeLedger: func(_ context.Context, _ xdr.LedgerCloseMeta) error {
			return commandStatus.takeStatusError()
		},
		AfterLedger: func(_ context.Context, ledger xdr.LedgerCloseMeta, _ runtime.LedgerStats) {
			commandStatus.recordStatusError(commandStatus.ledgerProgress(ledger.LedgerSequence(), target))
		},
	}
	return append(hooks, statusHook)
}
