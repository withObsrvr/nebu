package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// SinkConfig holds configuration for running a sink processor as a CLI tool.
type SinkConfig struct {
	Name        string
	Description string
	Version     string
}

// SinkFunc is a function that processes an event and produces a side effect.
// Returns an error if processing fails.
type SinkFunc func(event map[string]interface{}) error

// RunSinkCLI creates and executes a CLI for a sink processor.
// Sink processors read JSON events from stdin and produce side effects (write to DB, files, etc).
func RunSinkCLI(config SinkConfig, sinkFunc SinkFunc, flags func(*cobra.Command)) {
	var quietMode bool

	rootCmd := &cobra.Command{
		Use:     config.Name,
		Short:   config.Description,
		Version: config.Version,
		Long: fmt.Sprintf(`%s

This sink processor reads JSON events from stdin and processes them.
Sink processors produce side effects like writing to databases, files, or external services.

Examples:
  # Process events from a file
  cat events.jsonl | %s

  # Chain with origin processor
  nebu fetch 60200000 60200100 | token-transfer | %s

  # Full pipeline
  token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
    usdc-filter | \
    %s

  # Quiet mode suppresses progress messages
  cat events.jsonl | %s -q
`, config.Description, config.Name, config.Name, config.Name, config.Name),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSink(sinkFunc, quietMode)
		},
	}

	rootCmd.Flags().BoolVarP(&quietMode, "quiet", "q", false, "Suppress non-error output")

	// Allow the sink to add custom flags
	if flags != nil {
		flags(rootCmd)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runSink(sinkFunc SinkFunc, quietMode bool) error {
	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		if !quietMode {
			fmt.Fprintln(os.Stderr, "\nShutting down...")
		}
		os.Exit(0)
	}()

	if !quietMode {
		fmt.Fprintln(os.Stderr, "Reading events from stdin...")
	}

	scanner := bufio.NewScanner(os.Stdin)
	eventCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse input event
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			if !quietMode {
				fmt.Fprintf(os.Stderr, "Warning: failed to parse JSON: %v\n", err)
			}
			continue
		}

		// Process the event
		if err := sinkFunc(event); err != nil {
			return fmt.Errorf("failed to process event: %w", err)
		}

		eventCount++

		if !quietMode && eventCount%1000 == 0 {
			fmt.Fprintf(os.Stderr, "Processed %d events...\n", eventCount)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stdin: %w", err)
	}

	if !quietMode {
		fmt.Fprintf(os.Stderr, "Processed %d events\n", eventCount)
	}

	return nil
}
