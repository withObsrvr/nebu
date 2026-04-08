// Package cli provides helpers for running processors as standalone CLI tools.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
	"github.com/withObsrvr/nebu/pkg/source/rpc"
	"github.com/withObsrvr/nebu/pkg/version"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ProtoOriginProcessor wraps a protobuf-based origin processor for CLI use.
// The event type T must implement proto.Message.
type ProtoOriginProcessor[T proto.Message] interface {
	processor.Origin
	Out() <-chan T
	Close()
}

// RunProtoOriginCLI creates and executes a CLI for a protobuf-based origin processor.
// This provides a full CLI experience with flags, help text, and all input modes.
// Events are output as JSON using protojson for proper protobuf → JSON conversion.
func RunProtoOriginCLI[T proto.Message](
	config OriginConfig,
	createProcessor func(networkPass string) ProtoOriginProcessor[T],
) {
	var (
		rpcURL      string
		startLedger uint32
		endLedger   uint32
		networkPass string
		quietMode   bool
	)

	// Build help text
	longHelp := buildOriginLongHelp(config)

	rootCmd := &cobra.Command{
		Use:     config.Name,
		Short:   config.Description,
		Version: config.Version,
		Long:    longHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check input mode
			var inputFile string
			useStdin := false

			if len(args) == 1 {
				if args[0] == "-" {
					useStdin = true
				} else {
					inputFile = args[0]
				}
			} else {
				// Auto-detect stdin
				stat, _ := os.Stdin.Stat()
				if (stat.Mode() & os.ModeCharDevice) == 0 {
					useStdin = true
				}
			}

			// Validate flags
			if !useStdin && inputFile == "" {
				if startLedger == 0 {
					return fmt.Errorf("--start-ledger is required for RPC mode (or provide input file/stdin)")
				}
				// endLedger == 0 is valid (unbounded streaming)
			}

			// Create context with cancellation
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle Ctrl+C
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigCh
				if !quietMode {
					fmt.Fprintln(os.Stderr, "\nShutting down...")
				}
				cancel()

				// Force exit after 2 seconds if graceful shutdown fails
				time.Sleep(2 * time.Second)
				if !quietMode {
					fmt.Fprintln(os.Stderr, "Force shutdown after timeout")
				}
				os.Exit(1)
			}()

			// Normalize network passphrase (support "testnet", "mainnet", "pubnet" shortcuts)
			networkPass = normalizeNetworkPassphrase(networkPass)

			// Create processor
			origin := createProcessor(networkPass)

			// Configure protojson marshaler
			marshaler := protojson.MarshalOptions{
				UseProtoNames:   false, // Use JSON names (camelCase)
				EmitUnpopulated: true,  // Emit all fields including zero values
				Indent:          "",    // Compact JSON
			}

			// Start event output goroutine
			eventCount := 0
			done := make(chan error, 1)
			go func() {
				for ev := range origin.Out() {
					eventCount++

					// Marshal protobuf to JSON
					jsonBytes, err := marshaler.Marshal(ev)
					if err != nil {
						done <- err
						return
					}

					// Simple approach: prepend schema fields to JSON
					// This avoids complex map manipulation
					fmt.Printf(`{"_schema":"%s","_nebu_version":"%s",`,
						version.GetSchemaVersion(config.Name), version.Version)

					// Print the rest of the JSON (skip opening brace)
					if len(jsonBytes) > 2 { // More than just {}
						fmt.Print(string(jsonBytes[1:])) // Skip opening { and print rest
					} else {
						fmt.Print("}") // Empty object case
					}
					fmt.Println()
				}
				done <- nil
			}()

			// Run based on input mode
			var err error
			if useStdin {
				if !quietMode {
					fmt.Fprintln(os.Stderr, "Reading ledgers from stdin...")
				}
				err = processFromStdinProto[T](ctx, origin, os.Stdin)
			} else if inputFile != "" {
				if !quietMode {
					fmt.Fprintf(os.Stderr, "Reading ledgers from %s...\n", inputFile)
				}
				err = processFromFileProto[T](ctx, origin, inputFile)
			} else {
				if !quietMode {
					if endLedger == 0 {
						fmt.Fprintf(os.Stderr, "Streaming ledgers from %d (unbounded)...\n", startLedger)
					} else {
						fmt.Fprintf(os.Stderr, "Processing ledgers %d to %d...\n", startLedger, endLedger)
					}
				}
				// Get auth header from environment
				authHeader := getAuthHeader()
				err = processFromRPCProto[T](ctx, origin, rpcURL, startLedger, endLedger, authHeader)
			}

			if err != nil && err != context.Canceled {
				return err
			}

			// Close origin to signal completion to the output goroutine
			origin.Close()

			// Wait for events to finish outputting
			if err := <-done; err != nil {
				return err
			}

			if !quietMode {
				fmt.Fprintf(os.Stderr, "Processed %d events\n", eventCount)
			}
			return nil
		},
	}

	rootCmd.Flags().StringVar(&rpcURL, "rpc-url", "https://archive-rpc.lightsail.network", "Stellar RPC endpoint")
	rootCmd.Flags().Uint32Var(&startLedger, "start-ledger", 0, "Start ledger sequence")
	rootCmd.Flags().Uint32Var(&endLedger, "end-ledger", 0, "End ledger sequence (0 for unbounded)")
	rootCmd.Flags().StringVar(&networkPass, "network", network.PublicNetworkPassphrase, "Network passphrase")
	rootCmd.Flags().BoolVarP(&quietMode, "quiet", "q", false, "Suppress non-error output")
	rootCmd.Flags().Bool(describeFlagName, false, "Emit machine-readable describe envelope to stdout and exit")

	// Short-circuit into the describe-json protocol before cobra
	// validates required flags. --describe-json must work even when
	// mandatory processor flags are missing.
	emitDescribeIfRequested(buildOriginEnvelope(rootCmd, config, descriptorOf[T]()))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func processFromRPCProto[T proto.Message](ctx context.Context, origin ProtoOriginProcessor[T], rpcURL string, start, end uint32, authHeader string) error {
	// Create RPC source with optional auth headers
	var src *rpc.LedgerSource
	var err error

	if authHeader != "" {
		headers := map[string]string{"Authorization": authHeader}
		src, err = rpc.NewLedgerSourceWithHeaders(rpcURL, headers)
	} else {
		src, err = rpc.NewLedgerSource(rpcURL)
	}

	if err != nil {
		return fmt.Errorf("failed to create RPC source: %w", err)
	}
	defer src.Close()

	rt := runtime.NewRuntime()
	return rt.RunOrigin(ctx, src, origin, start, end)
}

func processFromStdinProto[T proto.Message](ctx context.Context, origin processor.Origin, input io.Reader) error {
	reader := bufio.NewReader(input)
	ledgerCount := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var ledger xdr.LedgerCloseMeta
		_, err := xdr.Unmarshal(reader, &ledger)
		if err != nil {
			// Clean EOF or EOF-related errors after processing ledgers are OK
			if err == io.EOF || (ledgerCount > 0 && isEOFErrorProto(err)) {
				return nil
			}
			return fmt.Errorf("failed to decode XDR at ledger %d: %w", ledgerCount+1, err)
		}

		origin.ProcessLedger(ctx, ledger)
		ledgerCount++
	}
}

func processFromFileProto[T proto.Message](ctx context.Context, origin processor.Origin, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer f.Close()

	return processFromStdinProto[T](ctx, origin, f)
}

func isEOFErrorProto(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return err == io.EOF || contains(errMsg, "EOF")
}
