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

// TransformConfig holds configuration for running a transform processor as a CLI tool.
type TransformConfig struct {
	Name        string
	Description string
	Version     string
}

// TransformFunc is a function that transforms an event.
// Returns the transformed event, or nil to filter out the event.
type TransformFunc func(event map[string]interface{}) map[string]interface{}

// RunTransformCLI creates and executes a CLI for a transform processor.
// Transform processors read JSON events from stdin and write transformed JSON to stdout.
func RunTransformCLI(config TransformConfig, transformFunc TransformFunc, flags func(*cobra.Command)) {
	var quietMode bool

	rootCmd := &cobra.Command{
		Use:     config.Name,
		Short:   config.Description,
		Version: config.Version,
		Long: fmt.Sprintf(`%s

This transform processor reads JSON events from stdin, transforms them,
and writes the results to stdout. Events can be filtered out by returning nil.

Examples:
  # Process events from a file
  cat events.jsonl | %s > filtered.jsonl

  # Chain with origin processor
  nebu fetch 60200000 60200100 | token-transfer | %s

  # Chain multiple transforms
  token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
    %s | \
    other-transform | \
    sink-processor

  # Quiet mode suppresses progress messages
  cat events.jsonl | %s -q > output.jsonl
`, config.Description, config.Name, config.Name, config.Name, config.Name),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransform(transformFunc, quietMode)
		},
	}

	rootCmd.Flags().BoolVarP(&quietMode, "quiet", "q", false, "Suppress non-error output")

	// Allow the transform to add custom flags
	if flags != nil {
		flags(rootCmd)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runTransform(transformFunc TransformFunc, quietMode bool) error {
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
	encoder := json.NewEncoder(os.Stdout)

	eventCount := 0
	outputCount := 0

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

		eventCount++

		// Transform the event
		transformed := transformFunc(event)
		if transformed == nil {
			// Event filtered out
			continue
		}

		// Write transformed event
		if err := encoder.Encode(transformed); err != nil {
			return fmt.Errorf("failed to encode output: %w", err)
		}

		outputCount++

		if !quietMode && eventCount%1000 == 0 {
			fmt.Fprintf(os.Stderr, "Processed %d events (%d output)...\n", eventCount, outputCount)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stdin: %w", err)
	}

	if !quietMode {
		fmt.Fprintf(os.Stderr, "Processed %d events (%d output)\n", eventCount, outputCount)
	}

	return nil
}
