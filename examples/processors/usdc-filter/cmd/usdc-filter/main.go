// Package main provides a standalone CLI for the usdc-filter transform processor.
//
// This processor filters token transfer events for USDC transfers only.
// It reads JSON events from stdin and writes filtered events to stdout.
//
// Usage:
//
//	# Filter a file
//	cat events.jsonl | usdc-filter > usdc-events.jsonl
//
//	# Chain with origin processor
//	nebu fetch 60200000 60200100 | token-transfer | usdc-filter
//
//	# Full pipeline
//	token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
//	  usdc-filter | \
//	  json-file-sink --out usdc-transfers.jsonl
package main

import (
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

func main() {
	config := cli.TransformConfig{
		Name:        "usdc-filter",
		Description: "Filter token transfer events for USDC transfers only",
		Version:     version,
	}

	cli.RunTransformCLI(config, filterUSDC, nil)
}

// filterUSDC filters events to only include USDC transfers.
// Returns the event if it's a USDC transfer, nil otherwise.
func filterUSDC(event map[string]interface{}) map[string]interface{} {
	// Check if this is a transfer event (protojson format)
	transfer, ok := event["transfer"].(map[string]interface{})
	if !ok {
		return nil // Filter out non-transfer events
	}

	// Get the asset object
	asset, ok := transfer["asset"].(map[string]interface{})
	if !ok {
		return nil
	}

	// Check for issued asset (not native)
	issuedAsset, ok := asset["issuedAsset"].(map[string]interface{})
	if !ok {
		return nil // Not an issued asset
	}

	// Check if the asset code is USDC
	assetCode, ok := issuedAsset["assetCode"].(string)
	if !ok || assetCode != "USDC" {
		return nil // Filter out non-USDC events
	}

	// This is a USDC transfer - pass it through
	return event
}
