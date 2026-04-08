package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
)

// processFromStdin reads XDR ledgers from stdin and processes them through an origin processor.
func processFromStdin(ctx context.Context, origin processor.Origin, input io.Reader) error {
	reader := bufio.NewReader(input)
	ledgerCount := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read a ledger from XDR
		var ledger xdr.LedgerCloseMeta
		_, err := xdr.Unmarshal(reader, &ledger)
		if err != nil {
			if err == io.EOF {
				// End of input - this is normal
				logInfo("Processed %d ledgers from stdin", ledgerCount)
				return nil
			}
			return fmt.Errorf("failed to decode XDR at ledger %d: %w", ledgerCount+1, err)
		}

		// Process the ledger. Per-ledger errors are reported via the
		// processor's Reporter (stderr by default); this function only
		// returns on source/decode failures and context cancellation.
		origin.ProcessLedger(ctx, ledger)
		ledgerCount++
		if ledgerCount%100 == 0 {
			logInfo("Processed %d ledgers...", ledgerCount)
		}
	}
}

// processFromFile reads XDR ledgers from a file and processes them through an origin processor.
func processFromFile(ctx context.Context, origin processor.Origin, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer f.Close()

	logInfo("Reading ledgers from %s...", filePath)
	return processFromStdin(ctx, origin, f)
}
