package transaction_stats

import (
	"context"
	"fmt"
	"io"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"

	txstatspb "github.com/withObsrvr/nebu/examples/processors/transaction-stats/proto"
)

type Origin struct {
	passphrase string
	start      uint32
	end        uint32
	successTx  uint64
	failedTx   uint64
	successOps uint64
	failedOps  uint64
	emitter    *processor.Emitter[*txstatspb.TransactionStats]
}

func NewOrigin(passphrase string) *Origin {
	return &Origin{
		passphrase: passphrase,
		emitter:    processor.NewEmitter[*txstatspb.TransactionStats](1),
	}
}

func (o *Origin) Name() string                            { return "transaction-stats" }
func (o *Origin) Type() processor.Type                    { return processor.TypeOrigin }
func (o *Origin) Out() <-chan *txstatspb.TransactionStats { return o.emitter.Out() }

func (o *Origin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
	seq := ledger.LedgerSequence()
	if o.start == 0 || seq < o.start {
		o.start = seq
	}
	if seq > o.end {
		o.end = seq
	}

	reader, err := ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(o.passphrase, ledger)
	if err != nil {
		processor.ReportWarning(ctx, o.Name(), fmt.Errorf("ledger %d: create tx reader: %w", seq, err))
		return
	}
	defer reader.Close()

	for {
		tx, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			processor.ReportWarning(ctx, o.Name(), fmt.Errorf("ledger %d: read tx: %w", seq, err))
			return
		}

		opCount := uint64(len(tx.Envelope.Operations()))
		if tx.Result.Successful() {
			o.successTx++
			o.successOps += opCount
		} else {
			o.failedTx++
			o.failedOps += opCount
		}
	}
}

func (o *Origin) Close() {
	totalTx := o.successTx + o.failedTx
	totalOps := o.successOps + o.failedOps
	o.emitter.Emit(&txstatspb.TransactionStats{
		StartLedger:            o.start,
		EndLedger:              o.end,
		SuccessfulTransactions: o.successTx,
		FailedTransactions:     o.failedTx,
		TotalTransactions:      totalTx,
		OperationsInSuccessful: o.successOps,
		OperationsInFailed:     o.failedOps,
		TotalOperations:        totalOps,
	})
	o.emitter.Close()
}
