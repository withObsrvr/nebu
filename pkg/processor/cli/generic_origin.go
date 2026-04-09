// Package cli provides helpers for running processors as standalone CLI tools.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
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
)

// GenericOriginProcessor wraps a generic origin processor for CLI use.
// The event type T can be any serializable struct.
type GenericOriginProcessor[T any] interface {
	processor.Origin
	Out() <-chan *T
	Close()
}

// RunGenericOriginCLI creates and executes a CLI for a generic origin processor.
// This provides a full CLI experience with flags, help text, and all input modes.
func RunGenericOriginCLI[T any](
	config OriginConfig,
	createProcessor func(networkPass string) GenericOriginProcessor[T],
) {
	var (
		rpcURL      string
		startLedger uint32
		endLedger   uint32
		networkPass string
		quietMode   bool
	)

	rootCmd := &cobra.Command{
		Use:     config.Name,
		Short:   config.Description,
		Version: config.Version,
		Long: fmt.Sprintf(`%s

This processor can run in three modes:
  1. RPC mode: Fetch ledgers from Stellar RPC (bounded or unbounded)
  2. stdin mode: Read XDR ledgers from stdin
  3. File mode: Read XDR ledgers from a file

Examples:
  # Fetch bounded range (specific ledgers)
  %s --start-ledger 60200000 --end-ledger 60200100

  # Fetch unbounded (stream continuously from ledger 60200000)
  %s --start-ledger 60200000

  # Or explicitly set unbounded
  %s --start-ledger 60200000 --end-ledger 0

  # Read from stdin
  cat ledgers.xdr | %s

  # Read from file
  %s ledgers.xdr

  # Pipe to other tools
  nebu fetch 60200000 60200100 | %s | jq .
`, config.Description, config.Name, config.Name, config.Name, config.Name, config.Name, config.Name),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check input mode. Resolution order:
			//   1. Positional arg "-"        → explicit stdin
			//   2. Positional arg <path>     → explicit file
			//   3. --start-ledger N (N>0)    → explicit RPC mode
			//   4. stdin is a non-tty pipe   → auto-detect stdin
			//   5. (otherwise — error in validation below)
			//
			// Step 3 must come before step 4: in non-interactive
			// shells (CI, scripts, cron jobs) stdin is often
			// /dev/null, which is non-tty. Without this guard, the
			// auto-detect would override explicit --start-ledger
			// flags and fail with an EOF-decoding-XDR error.
			var inputFile string
			useStdin := false

			if len(args) == 1 {
				if args[0] == "-" {
					useStdin = true
				} else {
					inputFile = args[0]
				}
			} else if startLedger == 0 {
				// No positional arg, no explicit RPC range — try to
				// auto-detect a piped stdin source.
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
			defer origin.Close()

			// Start event output goroutine
			eventCount := 0
			done := make(chan error, 1)
			go func() {
				encoder := json.NewEncoder(os.Stdout)
				schemaID := config.SchemaID
				if schemaID == "" {
					schemaID = version.GetSchemaVersion(config.Name)
				}
				for ev := range origin.Out() {
					eventCount++
					// Add schema version wrapper
					wrapped := map[string]interface{}{
						"_schema":       schemaID,
						"_nebu_version": version.Version,
					}

					// Marshal the event to extract fields
					eventBytes, err := json.Marshal(ev)
					if err != nil {
						done <- err
						return
					}

					// Unmarshal into wrapped map to merge fields
					var eventMap map[string]interface{}
					if err := json.Unmarshal(eventBytes, &eventMap); err != nil {
						done <- err
						return
					}

					// Merge event fields into wrapped map
					for k, v := range eventMap {
						wrapped[k] = v
					}

					if err := encoder.Encode(wrapped); err != nil {
						done <- err
						return
					}
				}
				done <- nil
			}()

			// Run based on input mode
			var err error
			if useStdin {
				if !quietMode {
					fmt.Fprintln(os.Stderr, "Reading ledgers from stdin...")
				}
				err = processFromStdinGeneric[T](ctx, origin, os.Stdin)
			} else if inputFile != "" {
				if !quietMode {
					fmt.Fprintf(os.Stderr, "Reading ledgers from %s...\n", inputFile)
				}
				err = processFromFileGeneric[T](ctx, origin, inputFile)
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
				err = processFromRPCGeneric(ctx, origin, rpcURL, startLedger, endLedger, authHeader, config.Hooks)
			}

			if err != nil && err != context.Canceled {
				return err
			}

			// Wait for events
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

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func processFromRPCGeneric[T any](ctx context.Context, origin GenericOriginProcessor[T], rpcURL string, start, end uint32, authHeader string, hooks []runtime.Hooks) error {
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
	attachHooks(rt, hooks)
	return rt.RunOrigin(ctx, src, origin, start, end)
}

func processFromStdinGeneric[T any](ctx context.Context, origin processor.Origin, input io.Reader) error {
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
			if err == io.EOF || (ledgerCount > 0 && isEOFErrorGeneric(err)) {
				return nil
			}
			return fmt.Errorf("failed to decode XDR at ledger %d: %w", ledgerCount+1, err)
		}

		origin.ProcessLedger(ctx, ledger)
		ledgerCount++
	}
}

func processFromFileGeneric[T any](ctx context.Context, origin processor.Origin, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer f.Close()

	return processFromStdinGeneric[T](ctx, origin, f)
}

func isEOFErrorGeneric(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return err == io.EOF || contains(errMsg, "EOF")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
