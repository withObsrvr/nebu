package ledger_change_stats

import (
	"context"
	"fmt"
	"io"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"

	changestatspb "github.com/withObsrvr/nebu/examples/processors/ledger-change-stats/proto"
)

type Origin struct {
	passphrase string
	emitter    *processor.Emitter[*changestatspb.LedgerChangeStats]
}

func NewOrigin(passphrase string) *Origin {
	return &Origin{
		passphrase: passphrase,
		emitter:    processor.NewEmitter[*changestatspb.LedgerChangeStats](128),
	}
}

func (o *Origin) Name() string                                 { return "ledger-change-stats" }
func (o *Origin) Type() processor.Type                         { return processor.TypeOrigin }
func (o *Origin) Out() <-chan *changestatspb.LedgerChangeStats { return o.emitter.Out() }
func (o *Origin) Close()                                       { o.emitter.Close() }

func changeCausedBy(change ingest.Change) xdr.LedgerEntryChangeType {
	if change.Pre == nil && change.Post != nil {
		return xdr.LedgerEntryChangeTypeLedgerEntryCreated
	}
	if change.Pre != nil && change.Post == nil {
		return xdr.LedgerEntryChangeTypeLedgerEntryRemoved
	}
	return xdr.LedgerEntryChangeTypeLedgerEntryUpdated
}

func (o *Origin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
	seq := ledger.LedgerSequence()
	reader, err := ingest.NewLedgerChangeReaderFromLedgerCloseMeta(o.passphrase, ledger)
	if err != nil {
		processor.ReportWarning(ctx, o.Name(), fmt.Errorf("ledger %d: create change reader: %w", seq, err))
		return
	}
	defer reader.Close()

	info := &changestatspb.LedgerChangeStats{LedgerSequence: seq}
	for {
		change, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			processor.ReportWarning(ctx, o.Name(), fmt.Errorf("ledger %d: read change: %w", seq, err))
			return
		}

		switch changeCausedBy(change) {
		case xdr.LedgerEntryChangeTypeLedgerEntryCreated:
			info.LedgerEntriesCreated++
		case xdr.LedgerEntryChangeTypeLedgerEntryRemoved:
			info.LedgerEntriesDeleted++
		case xdr.LedgerEntryChangeTypeLedgerEntryUpdated:
			info.LedgerEntriesUpdated++
		}

		switch change.Reason {
		case ingest.LedgerEntryChangeReasonFee:
			info.FeeRelatedChanges++
		case ingest.LedgerEntryChangeReasonTransaction:
			info.TxRelatedChanges++
		case ingest.LedgerEntryChangeReasonOperation:
			info.OperationRelatedChanges++
		}
	}

	select {
	case <-ctx.Done():
		return
	default:
		o.emitter.Emit(info)
	}
}
