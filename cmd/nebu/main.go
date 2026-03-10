// Package main implements the nebu CLI for running processors and scaffolding new ones.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/version"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "nebu",
		Short: "nebu - modular streaming runtime for Stellar",
		Long: `nebu is a minimal, Unix-philosophy streaming runtime for Stellar blockchain data.

Build custom indexers, analytics pipelines, and real-time automation by composing
processors that operate on Stellar ledger data via Unix pipes.

QUICK START:
  # Install a processor
  nebu install token-transfer

  # Extract token transfer events
  token-transfer --start-ledger 60200000 --end-ledger 60200100

  # Or pipe from nebu fetch
  nebu fetch 60200000 60200100 | token-transfer

  # Filter and analyze with standard tools
  token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
    jq 'select(.type == "transfer")' | \
    head -10

PROCESSOR TYPES:
  origin     Extract events from ledgers (token-transfer, contract-events)
  transform  Filter/modify events (usdc-filter, amount-filter, dedup)
  sink       Store events (json-file-sink, postgres-sink, nats-sink)

COMMON WORKFLOWS:
  # List available processors
  nebu list

  # Fetch raw ledger data
  nebu fetch 60200000 60200100 > ledgers.xdr

  # Build a full pipeline
  token-transfer --start 60200000 --end 60200100 | \
    usdc-filter | \
    amount-filter --min 1000000 | \
    json-file-sink --out large-usdc.jsonl

ENVIRONMENT:
  NEBU_RPC_URL    RPC endpoint (default: mainnet)
  NEBU_RPC_AUTH   Authorization header (e.g., 'Api-Key xxx')
  NEBU_NETWORK    Network: 'mainnet' or 'testnet'`,
		Version: version.Version,
	}

	// Add global flags
	rootCmd.PersistentFlags().BoolVarP(&quietMode, "quiet", "q", false, "suppress non-error output")

	// Add subcommands
	rootCmd.AddCommand(newFetchCmd())
	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newInstallCmd())
	rootCmd.AddCommand(newNewCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newDescribeCmd())
	rootCmd.AddCommand(newResumeCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
