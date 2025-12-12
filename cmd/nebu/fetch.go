package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/source"
)

func newFetchCmd() *cobra.Command {
	var (
		rpcURL      string
		startLedger uint32
		endLedger   uint32
		networkPass string
		outputFile  string
	)

	cmd := &cobra.Command{
		Use:   "fetch [start-ledger] [end-ledger]",
		Short: "Fetch ledgers from Stellar RPC and output XDR",
		Long: `Fetch ledgers from Stellar RPC and output raw XDR to stdout or file.

This command fetches ledgers in the specified range and outputs them as raw XDR
data that can be piped to processors or saved for later use.

Examples:
  # Fetch to stdout
  nebu fetch 60200000 60200100 > ledgers.xdr

  # Fetch to file
  nebu fetch 60200000 60200100 --output ledgers.xdr

  # Pipe directly to processor
  nebu fetch 60200000 60200100 | nebu run origin token-transfer

  # Quiet mode for clean piping
  nebu fetch -q 60200000 60200100 | nebu run origin token-transfer -q
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse ledger range from positional args
			var err error
			_, err = fmt.Sscanf(args[0], "%d", &startLedger)
			if err != nil {
				return fmt.Errorf("invalid start ledger: %s", args[0])
			}
			_, err = fmt.Sscanf(args[1], "%d", &endLedger)
			if err != nil {
				return fmt.Errorf("invalid end ledger: %s", args[1])
			}

			if startLedger == 0 || endLedger == 0 {
				return fmt.Errorf("ledger numbers must be > 0")
			}
			if startLedger > endLedger {
				return fmt.Errorf("start ledger must be <= end ledger")
			}

			// Create context with cancellation
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle Ctrl+C
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigCh
				logInfo("\nShutting down...")
				cancel()
			}()

			return fetchLedgers(ctx, rpcURL, networkPass, startLedger, endLedger, outputFile)
		},
	}

	cmd.Flags().StringVar(&rpcURL, "rpc-url", "https://mainnet.sorobanrpc.com", "Stellar RPC endpoint")
	cmd.Flags().StringVar(&networkPass, "network", network.PublicNetworkPassphrase, "Network passphrase")
	cmd.Flags().StringVar(&outputFile, "output", "", "Output file (default: stdout)")

	return cmd
}

func fetchLedgers(ctx context.Context, rpcURL, networkPass string, start, end uint32, outputFile string) error {
	// Create RPC source
	src, err := source.NewRPCLedgerSource(rpcURL)
	if err != nil {
		return fmt.Errorf("failed to create RPC source: %w", err)
	}
	defer src.Close()

	// Determine output destination
	var output *os.File
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		output = f
		logInfo("Fetching ledgers %d to %d to %s...", start, end, outputFile)
	} else {
		output = os.Stdout
		logInfo("Fetching ledgers %d to %d...", start, end)
	}

	// Stream ledgers
	ledgerCh := make(chan xdr.LedgerCloseMeta, 128)
	errCh := make(chan error, 1)

	go func() {
		err := src.Stream(ctx, start, end, ledgerCh)
		if err != nil && err != context.Canceled {
			errCh <- err
		}
		close(errCh)
	}()

	ledgerCount := 0
	for ledger := range ledgerCh {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Write XDR to output using xdr.Marshal (includes framing)
		_, err := xdr.Marshal(output, ledger)
		if err != nil {
			return fmt.Errorf("failed to marshal ledger %d: %w", ledger.LedgerSequence(), err)
		}

		ledgerCount++
		if ledgerCount%10 == 0 {
			logInfo("Fetched %d ledgers...", ledgerCount)
		}
	}

	// Check for stream errors
	if err := <-errCh; err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	logInfo("Fetched %d ledgers", ledgerCount)
	return nil
}
