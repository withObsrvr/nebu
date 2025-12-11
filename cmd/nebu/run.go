package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/processors/token_transfer"
	"github.com/withObsrvr/nebu/pkg/runtime"
	"github.com/withObsrvr/nebu/pkg/source"
	ttp "github.com/withObsrvr/nebu/processors/core/token_transfer"
)

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run processors against Stellar RPC",
		Long:  `Run origin, transform, or sink processors against Stellar ledger data.`,
	}

	cmd.AddCommand(newRunOriginCmd())

	return cmd
}

func newRunOriginCmd() *cobra.Command {
	var (
		rpcURL      string
		startLedger uint32
		endLedger   uint32
		networkPass string
		jsonOutput  bool
	)

	cmd := &cobra.Command{
		Use:   "origin [processor-name]",
		Short: "Run an origin processor",
		Long: `Run an origin processor against Stellar RPC.

Currently supported processors:
  token-transfer  - Stream token transfer events (transfers, mints, burns, etc.)

Examples:
  # Stream token transfers from ledgers 60200000-60200100
  nebu run origin token-transfer --start 60200000 --end 60200100

  # Use custom RPC endpoint
  nebu run origin token-transfer --start 60200000 --end 60200100 --rpc-url https://rpc-pubnet.nodeswithobsrvr.co
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			processorName := args[0]

			// Validate required flags
			if startLedger == 0 {
				return fmt.Errorf("--start-ledger is required")
			}
			if endLedger == 0 {
				return fmt.Errorf("--end-ledger is required")
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
				fmt.Fprintln(os.Stderr, "\nShutting down...")
				cancel()
			}()

			// Run the appropriate processor
			switch processorName {
			case "token-transfer":
				return runTokenTransfer(ctx, rpcURL, networkPass, startLedger, endLedger, jsonOutput)
			default:
				return fmt.Errorf("unknown processor: %s\nSupported processors: token-transfer", processorName)
			}
		},
	}

	cmd.Flags().StringVar(&rpcURL, "rpc-url", "https://mainnet.sorobanrpc.com", "Stellar RPC endpoint")
	cmd.Flags().Uint32Var(&startLedger, "start-ledger", 0, "Start ledger sequence (required)")
	cmd.Flags().Uint32Var(&endLedger, "end-ledger", 0, "End ledger sequence (required)")
	cmd.Flags().StringVar(&networkPass, "network", network.PublicNetworkPassphrase, "Network passphrase")
	cmd.Flags().BoolVar(&jsonOutput, "json", true, "Output events as JSON (default true)")

	return cmd
}

func runTokenTransfer(ctx context.Context, rpcURL, networkPass string, start, end uint32, jsonOutput bool) error {
	// Create RPC source
	src, err := source.NewRPCLedgerSource(rpcURL)
	if err != nil {
		return fmt.Errorf("failed to create RPC source: %w", err)
	}
	defer src.Close()

	// Create token transfer origin processor
	origin := ttp.NewOrigin(networkPass)
	defer origin.Close()

	// Start collecting events in background
	eventCount := 0
	done := make(chan error, 1)

	go func() {
		encoder := json.NewEncoder(os.Stdout)
		for ev := range origin.Out() {
			eventCount++
			if jsonOutput {
				// Convert to simplified JSON format
				simplified := simplifyTokenTransferEvent(ev)
				if err := encoder.Encode(simplified); err != nil {
					done <- err
					return
				}
			} else {
				// Just print a summary
				fmt.Printf("Event %d: %s\n", eventCount, getEventType(ev))
			}
		}
		done <- nil
	}()

	// Run the runtime
	fmt.Fprintf(os.Stderr, "Processing ledgers %d to %d...\n", start, end)

	rt := runtime.NewRuntime()
	if err := rt.RunOrigin(ctx, src, origin, start, end); err != nil && err != context.Canceled {
		return fmt.Errorf("runtime error: %w", err)
	}

	// Close origin and wait for event collection to finish
	origin.Close()
	if err := <-done; err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Processed %d events\n", eventCount)
	return nil
}

// simplifyTokenTransferEvent converts a protobuf event to a JSON-friendly map
func simplifyTokenTransferEvent(ev *token_transfer.TokenTransferEvent) map[string]interface{} {
	meta := ev.GetMeta()
	event := map[string]interface{}{
		"ledger_sequence": meta.LedgerSequence,
		"tx_hash":         meta.TxHash,
	}

	if meta.ContractAddress != "" {
		event["contract_address"] = meta.ContractAddress
	}

	// Handle asset
	if asset := ev.GetAsset(); asset != nil {
		assetInfo := make(map[string]string)
		if asset.GetNative() {
			assetInfo["code"] = "native"
		} else if issued := asset.GetIssuedAsset(); issued != nil {
			assetInfo["code"] = issued.AssetCode
			assetInfo["issuer"] = issued.Issuer
		}
		event["asset"] = assetInfo
	}

	// Handle different event types
	switch {
	case ev.GetTransfer() != nil:
		transfer := ev.GetTransfer()
		event["type"] = "transfer"
		event["from"] = transfer.From
		event["to"] = transfer.To
		event["amount"] = transfer.Amount

	case ev.GetMint() != nil:
		mint := ev.GetMint()
		event["type"] = "mint"
		event["to"] = mint.To
		event["amount"] = mint.Amount

	case ev.GetBurn() != nil:
		burn := ev.GetBurn()
		event["type"] = "burn"
		event["from"] = burn.From
		event["amount"] = burn.Amount

	case ev.GetClawback() != nil:
		clawback := ev.GetClawback()
		event["type"] = "clawback"
		event["from"] = clawback.From
		event["amount"] = clawback.Amount

	case ev.GetFee() != nil:
		fee := ev.GetFee()
		event["type"] = "fee"
		event["from"] = fee.From
		event["amount"] = fee.Amount

	default:
		event["type"] = "unknown"
	}

	return event
}

func getEventType(ev *token_transfer.TokenTransferEvent) string {
	switch {
	case ev.GetTransfer() != nil:
		return "transfer"
	case ev.GetMint() != nil:
		return "mint"
	case ev.GetBurn() != nil:
		return "burn"
	case ev.GetClawback() != nil:
		return "clawback"
	case ev.GetFee() != nil:
		return "fee"
	default:
		return "unknown"
	}
}
