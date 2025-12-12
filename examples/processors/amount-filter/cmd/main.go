// Package main provides a standalone CLI for the amount-filter transform processor.
//
// This processor filters transfer events based on amount ranges.
// It reads JSON events from stdin and writes filtered events to stdout.
//
// Usage:
//
//	# Filter for amounts >= 1M stroops
//	cat events.jsonl | amount-filter --min 1000000
//
//	# Filter for amounts between 1M and 100M
//	cat events.jsonl | amount-filter --min 1000000 --max 100000000
//
//	# Filter for large USDC transfers
//	token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
//	  amount-filter --min 10000000 --asset USDC
//
//	# Find whale movements (100M+)
//	token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
//	  amount-filter --min 100000000 | \
//	  json-file-sink --out whales.jsonl
package main

import (
	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/examples/processors/amount-filter"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

var (
	minAmount int64
	maxAmount int64
	assetCode string
)

func main() {
	config := cli.TransformConfig{
		Name:        "amount-filter",
		Description: "Filter transfer events by amount range",
		Version:     version,
	}

	cli.RunTransformCLI(config, filterByAmount, addFlags)
}

func addFlags(cmd *cobra.Command) {
	cmd.Flags().Int64Var(&minAmount, "min", 0, "Minimum amount (inclusive, in stroops)")
	cmd.Flags().Int64Var(&maxAmount, "max", 0, "Maximum amount (inclusive, in stroops, 0 = no limit)")
	cmd.Flags().StringVar(&assetCode, "asset", "", "Filter by asset code (optional, e.g., USDC, XLM)")
}

// filterByAmount filters events based on amount and optionally asset code.
// Returns the event if it passes the filters, nil otherwise.
func filterByAmount(event map[string]interface{}) map[string]interface{} {
	// Use shared filter logic
	filter := amount_filter.NewFilter(minAmount, maxAmount, assetCode)
	return filter.FilterEvent(event)
}
