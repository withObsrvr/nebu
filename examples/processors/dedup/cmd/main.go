// Package main provides a standalone CLI for the dedup transform processor.
//
// This processor removes duplicate events based on specified keys.
// It reads JSON events from stdin and writes unique events to stdout.
//
// Usage:
//
//	# Deduplicate by transaction hash
//	cat events.jsonl | dedup --key tx_hash
//
//	# Deduplicate by multiple fields
//	cat events.jsonl | dedup --key tx_hash,ledger_sequence
//
//	# Remove duplicate transfers in pipeline
//	token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
//	  dedup --key tx_hash | \
//	  json-file-sink --out unique-transfers.jsonl
package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

var dedupKeys string

// Track seen keys
var seenKeys = make(map[string]bool)

func main() {
	config := cli.TransformConfig{
		Name:        "dedup",
		Description: "Remove duplicate events based on key fields",
		Version:     version,
	}

	cli.RunTransformCLI(config, deduplicate, addFlags)
}

func addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&dedupKeys, "key", "tx_hash", "Comma-separated list of keys to use for deduplication (e.g., tx_hash or tx_hash,ledger_sequence)")
}

// deduplicate removes duplicate events based on the specified keys.
// Returns the event if it's unique, nil if it's a duplicate.
func deduplicate(event map[string]interface{}) map[string]interface{} {
	// Parse keys
	keys := strings.Split(dedupKeys, ",")

	// Build composite key from event fields
	var keyParts []string
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if value, ok := event[key]; ok {
			keyParts = append(keyParts, fmt.Sprintf("%v", value))
		} else {
			// Missing key field, treat as unique (don't filter out)
			return event
		}
	}

	// Create composite key
	compositeKey := strings.Join(keyParts, "|")

	// Check if we've seen this key before
	if seenKeys[compositeKey] {
		return nil // Duplicate, filter out
	}

	// Mark as seen and pass through
	seenKeys[compositeKey] = true
	return event
}
