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

// SinkConfig holds configuration for running a sink processor as a CLI tool.
type SinkConfig struct {
	Name        string
	Description string
	Version     string

	// SchemaID is the canonical identifier for the events this
	// sink consumes (e.g., "nebu.token_transfer.v1"). Surfaced
	// verbatim in --describe-json output. Optional.
	SchemaID string

	// InputType is a zero-value instance of the protobuf message
	// type this sink accepts as input. When set, a JSON Schema is
	// generated from its descriptor and published in the describe
	// envelope's schema.input field. Optional: sinks that accept any
	// JSON shape (like json-file-sink) should leave this nil.
	InputType proto.Message

	// Optional rich help configuration
	Help *HelpConfig
}

// SinkFunc is a function that processes an event and produces a side effect.
// Returns an error if processing fails.
type SinkFunc func(event map[string]interface{}) error

// RunSinkCLI creates and executes a CLI for a sink processor.
// Sink processors read JSON events from stdin and produce side effects (write to DB, files, etc).
func RunSinkCLI(config SinkConfig, sinkFunc SinkFunc, flags func(*cobra.Command)) {
	var quietMode bool

	// Build help text
	longHelp := buildSinkLongHelp(config)

	rootCmd := &cobra.Command{
		Use:     config.Name,
		Short:   config.Description,
		Version: config.Version,
		Long:    longHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSink(sinkFunc, quietMode)
		},
	}

	rootCmd.Flags().BoolVarP(&quietMode, "quiet", "q", false, "Suppress non-error output")
	rootCmd.Flags().Bool(describeFlagName, false, "Emit machine-readable describe envelope to stdout and exit")

	// Allow the sink to add custom flags
	if flags != nil {
		flags(rootCmd)
	}

	// Short-circuit into the describe-json protocol before cobra
	// validates required flags. --describe-json must work even when
	// mandatory processor flags are missing.
	emitDescribeIfRequested(buildSinkEnvelope(rootCmd, config))

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
	// Increase buffer size to handle large events (contract invocations with many args/diagnostics)
	// Default is 64KB, we set to 10MB
	const maxTokenSize = 10 * 1024 * 1024 // 10MB
	buf := make([]byte, maxTokenSize)
	scanner.Buffer(buf, maxTokenSize)
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

		// Process the event. Per-event failures are logged as warnings
		// and the loop continues (streams-never-throw). Genuinely fatal
		// conditions must be handled by the sinkFunc itself — either
		// return a non-nil error (which is logged as a warning and the
		// loop continues), or call os.Exit / panic directly for
		// unrecoverable failures. The SinkFunc signature does not
		// accept a context; sinks that need cancellation should wire
		// their own via package-level variables or closure capture.
		if err := sinkFunc(event); err != nil {
			if !quietMode {
				fmt.Fprintf(os.Stderr, "[sink] warning: failed to process event: %v\n", err)
			}
			continue
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
