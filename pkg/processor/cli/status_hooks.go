package cli

import (
	"context"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
)

func withStructuredStatusHook(hooks []runtime.Hooks, commandStatus *commandStatus, target uint32) []runtime.Hooks {
	if commandStatus == nil || commandStatus.structured == nil {
		return hooks
	}
	statusHook := runtime.Hooks{
		AfterLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta, _ runtime.LedgerStats) {
			if err := commandStatus.ledgerProgress(ledger.LedgerSequence(), target); err != nil {
				processor.ReportFatal(ctx, "structured-status", err)
			}
		},
	}
	return append(hooks, statusHook)
}
