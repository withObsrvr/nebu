package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
	"github.com/withObsrvr/nebu/pkg/source/rpc"
)

type ChangesInfo struct {
	LedgerEntriesCreated    int32 `json:"ledgerEntriesCreated"`
	LedgerEntriesUpdated    int32 `json:"ledgerEntriesUpdated"`
	LedgerEntriesDeleted    int32 `json:"ledgerEntriesDeleted"`
	FeeRelatedChanges       int32 `json:"feeRelatedChanges"`
	TxRelatedChanges        int32 `json:"txRelatedChanges"`
	OperationRelatedChanges int32 `json:"operationRelatedChanges"`
}

type ChangeStats struct{}

func (c *ChangeStats) Name() string         { return "ledger-change-statistics" }
func (c *ChangeStats) Type() processor.Type { return processor.TypeOrigin }

func changeCausedBy(change ingest.Change) xdr.LedgerEntryChangeType {
	if change.Pre == nil && change.Post != nil {
		return xdr.LedgerEntryChangeTypeLedgerEntryCreated
	}
	if change.Pre != nil && change.Post == nil {
		return xdr.LedgerEntryChangeTypeLedgerEntryRemoved
	}
	return xdr.LedgerEntryChangeTypeLedgerEntryUpdated
}

func (c *ChangeStats) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
	reader, err := ingest.NewLedgerChangeReaderFromLedgerCloseMeta(network.PublicNetworkPassphrase, ledger)
	if err != nil {
		processor.ReportWarning(ctx, c.Name(), fmt.Errorf("ledger %d: create change reader: %w", ledger.LedgerSequence(), err))
		return
	}
	defer reader.Close()

	info := ChangesInfo{}
	for {
		change, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			processor.ReportWarning(ctx, c.Name(), fmt.Errorf("ledger %d: read change: %w", ledger.LedgerSequence(), err))
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

	out, err := json.Marshal(info)
	if err != nil {
		processor.ReportWarning(ctx, c.Name(), fmt.Errorf("ledger %d: marshal stats: %w", ledger.LedgerSequence(), err))
		return
	}
	fmt.Printf("ledger %d: %s\n", ledger.LedgerSequence(), out)
}

func main() {
	src, err := rpc.NewLedgerSource("https://archive-rpc.lightsail.network")
	if err != nil {
		log.Fatalf("create ledger source: %v", err)
	}
	defer src.Close()

	rt := runtime.NewRuntime()
	if err := rt.RunOrigin(context.Background(), src, &ChangeStats{}, 60200000, 60200005); err != nil {
		log.Fatalf("run origin: %v", err)
	}
}
