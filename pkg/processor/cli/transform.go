package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

// TransformConfig holds configuration for running a transform processor as a CLI tool.
type TransformConfig struct {
	Name        string
	Description string
	Version     string

	// SchemaID is the canonical identifier for the events this
	// transform consumes and produces (e.g., "nebu.token_transfer.v1").
	// Surfaced verbatim in --describe-json output. Optional.
	SchemaID string

	// InputType is a zero-value instance of the protobuf message
	// type this transform accepts as input. When set, a JSON Schema
	// is generated from its descriptor and published in the describe
	// envelope's schema.input field. Optional: transforms that accept
	// any JSON shape should leave this nil.
	InputType proto.Message

	// OutputType is a zero-value instance of the protobuf message
	// type this transform emits. When set, a JSON Schema is generated
	// from its descriptor and published in the describe envelope's
	// schema.output field. Optional: pass-through filters may omit
	// this and consumers should assume output == input.
	OutputType proto.Message

	// Optional rich help configuration
	Help *HelpConfig
}

// TransformFunc is a function that transforms an event.
// Returns the transformed event, or nil to filter out the event.
type TransformFunc func(event map[string]interface{}) map[string]interface{}

// RunTransformCLI creates and executes a CLI for a transform processor.
// Transform processors read JSON events from stdin and write transformed JSON to stdout.
func RunTransformCLI(config TransformConfig, transformFunc TransformFunc, flags func(*cobra.Command)) {
	var quietMode bool

	// Build help text
	longHelp := buildTransformLongHelp(config)

	rootCmd := &cobra.Command{
		Use:     config.Name,
		Short:   config.Description,
		Version: config.Version,
		Long:    longHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransform(transformFunc, quietMode)
		},
	}

	rootCmd.Flags().BoolVarP(&quietMode, "quiet", "q", false, "Suppress non-error output")
	rootCmd.Flags().Bool(describeFlagName, false, "Emit machine-readable describe envelope to stdout and exit")

	// Allow the transform to add custom flags
	if flags != nil {
		flags(rootCmd)
	}

	// Short-circuit into the describe-json protocol before cobra
	// validates required flags. --describe-json must work even when
	// mandatory processor flags are missing.
	emitDescribeIfRequested(buildTransformEnvelope(rootCmd, config))

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
