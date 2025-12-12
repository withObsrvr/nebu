// Package main implements a simple JSON file sink that reads events from stdin.
//
// This processor writes JSON events to a JSONL file.
//
// Usage:
//
//	# Write to file
//	cat events.jsonl | json-file-sink --out output.jsonl
//
//	# Chain with origin processor
//	nebu fetch 60200000 60200100 | token-transfer | json-file-sink --out transfers.jsonl
//
//	# Full pipeline with transform
//	token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
//	  usdc-filter | \
//	  json-file-sink --out usdc-transfers.jsonl
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

var outputFile string
var fileWriter *bufio.Writer
var file *os.File

func main() {
	config := cli.SinkConfig{
		Name:        "json-file-sink",
		Description: "Write JSON events to a JSONL file",
		Version:     version,
	}

	cli.RunSinkCLI(config, writeToFile, addFlags)
}

func addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&outputFile, "out", "events.jsonl", "Output file path (JSONL format)")
}

func writeToFile(event map[string]interface{}) error {
	// Open file on first event
	if file == nil {
		var err error
		file, err = os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		fileWriter = bufio.NewWriter(file)
	}

	// Write event as JSON line
	encoder := json.NewEncoder(fileWriter)
	if err := encoder.Encode(event); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	// Flush periodically to ensure data is written
	return fileWriter.Flush()
}
