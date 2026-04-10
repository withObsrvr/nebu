package main

import (
	"context"
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

type TxStats struct {
	passphrase             string
	successfulTransactions int
	failedTransactions     int
	operationsInSuccessful int
	operationsInFailed     int
}

func (s *TxStats) Name() string         { return "transaction-statistics" }
func (s *TxStats) Type() processor.Type { return processor.TypeOrigin }

func (s *TxStats) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
	reader, err := ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(s.passphrase, ledger)
	if err != nil {
		processor.ReportWarning(ctx, s.Name(), fmt.Errorf("ledger %d: create tx reader: %w", ledger.LedgerSequence(), err))
		return
	}
	defer reader.Close()

	for {
		tx, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			processor.ReportWarning(ctx, s.Name(), fmt.Errorf("ledger %d: read tx: %w", ledger.LedgerSequence(), err))
			return
		}

		opCount := len(tx.Envelope.Operations())
		if tx.Result.Successful() {
			s.successfulTransactions++
			s.operationsInSuccessful += opCount
		} else {
			s.failedTransactions++
			s.operationsInFailed += opCount
		}
	}
}

func main() {
	src, err := rpc.NewLedgerSource("https://archive-rpc.lightsail.network")
	if err != nil {
		log.Fatalf("create ledger source: %v", err)
	}
	defer src.Close()

	stats := &TxStats{passphrase: network.PublicNetworkPassphrase}
	rt := runtime.NewRuntime()
	if err := rt.RunOrigin(context.Background(), src, stats, 60200000, 60200009); err != nil {
		log.Fatalf("run origin: %v", err)
	}

	fmt.Printf("txs: %d succeeded / %d failed\n", stats.successfulTransactions, stats.failedTransactions)
	fmt.Printf("ops: %d succeeded / %d failed\n", stats.operationsInSuccessful, stats.operationsInFailed)
}
