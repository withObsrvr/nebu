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
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/processors/token_transfer"
	"github.com/stellar/go-stellar-sdk/xdr"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
	"github.com/withObsrvr/nebu/pkg/source/rpc"
	"github.com/withObsrvr/nebu/pkg/version"
)

// getAuthHeader returns the authorization header value from the NEBU_RPC_AUTH environment variable.
func getAuthHeader() string {
	return os.Getenv("NEBU_RPC_AUTH")
}

// normalizeNetworkPassphrase converts common network aliases to full passphrases.
// Accepts: "mainnet", "testnet", "pubnet", or full passphrases.
func normalizeNetworkPassphrase(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "mainnet", "main", "public", "pubnet":
		return network.PublicNetworkPassphrase
	case "testnet", "test":
		return network.TestNetworkPassphrase
	default:
		return input
	}
}

// OriginConfig holds configuration for running an origin processor as a CLI tool.
type OriginConfig struct {
	Name        string
	Description string
	Version     string

	// Optional rich help configuration
	Help *HelpConfig
}

// TokenTransferOriginProcessor wraps the token transfer origin for CLI use.
type TokenTransferOriginProcessor interface {
	processor.Origin
	Out() <-chan *token_transfer.TokenTransferEvent
	Close()
}

// RunOriginCLI creates and executes a CLI for an origin processor.
// This provides a full CLI experience with flags, help text, and all input modes.
func RunOriginCLI(config OriginConfig, createProcessor func(networkPass string) TokenTransferOriginProcessor) {
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
			// Determine input mode:
			//   1. Explicit file argument or "-" for stdin
			//   2. --start-ledger flag → RPC mode (takes priority over stdin detection)
			//   3. Auto-detect piped stdin as fallback
			var inputFile string
			useStdin := false

			if len(args) == 1 {
				if args[0] == "-" {
					useStdin = true
				} else {
					inputFile = args[0]
				}
			} else if startLedger > 0 {
				// Explicit --start-ledger flag: use RPC mode
			} else {
				// No flags, no args: check if stdin is a pipe
				stat, _ := os.Stdin.Stat()
				if (stat.Mode() & os.ModeCharDevice) == 0 {
					useStdin = true
				}
			}

			// Validate flags
			if !useStdin && inputFile == "" {
				if startLedger == 0 {
					return nebuErrors.MissingRequiredFlags("--start-ledger")
				}
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

			// Start event output goroutine
			eventCount := 0
			done := make(chan error, 1)
			go func() {
				encoder := json.NewEncoder(os.Stdout)
				for ev := range origin.Out() {
					eventCount++
					simplified := simplifyEvent(ev)
					if err := encoder.Encode(simplified); err != nil {
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
				err = processFromStdin(ctx, origin, os.Stdin)
			} else if inputFile != "" {
				if !quietMode {
					fmt.Fprintf(os.Stderr, "Reading ledgers from %s...\n", inputFile)
				}
				err = processFromFile(ctx, origin, inputFile)
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
				err = processFromRPC(ctx, origin, rpcURL, startLedger, endLedger, authHeader)
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
	rootCmd.Flags().Uint32Var(&endLedger, "end-ledger", 0, "End ledger sequence")
	rootCmd.Flags().StringVar(&networkPass, "network", network.PublicNetworkPassphrase, "Network passphrase")
	rootCmd.Flags().BoolVarP(&quietMode, "quiet", "q", false, "Suppress non-error output")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func processFromRPC(ctx context.Context, origin processor.Origin, rpcURL string, start, end uint32, authHeader string) error {
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
		return nebuErrors.FailedToCreateSource("RPC", err)
	}
	defer src.Close()

	rt := runtime.NewRuntime()
	return rt.RunOrigin(ctx, src, origin, start, end)
}

func processFromStdin(ctx context.Context, origin processor.Origin, input io.Reader) error {
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
			if err == io.EOF || (ledgerCount > 0 && isEOFError(err)) {
				return nil
			}
			return nebuErrors.XDRDecodeFailed(ledgerCount+1, err)
		}

		origin.ProcessLedger(ctx, ledger)
		ledgerCount++
	}
}

// isEOFError checks if an error is EOF-related (e.g., "EOF while decoding")
func isEOFError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	// Check for EOF patterns - the XDR decoder can return various EOF-related errors
	// when it hits end of stream while reading a ledger
	return err == io.EOF || strings.Contains(errMsg, "EOF")
}

func processFromFile(ctx context.Context, origin processor.Origin, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return nebuErrors.FailedToOpenFile(filePath, err)
	}
	defer f.Close()

	return processFromStdin(ctx, origin, f)
}

func simplifyEvent(ev *token_transfer.TokenTransferEvent) map[string]interface{} {
	meta := ev.GetMeta()
	event := map[string]interface{}{
		"_schema":         version.GetSchemaVersion("token_transfer"),
		"_nebu_version":   version.Version,
		"ledger_sequence": meta.LedgerSequence,
		"tx_hash":         meta.TxHash,
	}

	if meta.ContractAddress != "" {
		event["contract_address"] = meta.ContractAddress
	}

	// Handle asset
	if asset := ev.GetAsset(); asset != nil {
		assetInfo := make(map[string]string)
		if asset.GetNative() {
			assetInfo["code"] = "native"
		} else if issued := asset.GetIssuedAsset(); issued != nil {
			assetInfo["code"] = issued.AssetCode
			assetInfo["issuer"] = issued.Issuer
		}
		event["asset"] = assetInfo
	}

	// Handle different event types
	switch {
	case ev.GetTransfer() != nil:
		transfer := ev.GetTransfer()
		event["type"] = "transfer"
		event["from"] = transfer.From
		event["to"] = transfer.To
		event["amount"] = transfer.Amount

	case ev.GetMint() != nil:
		mint := ev.GetMint()
		event["type"] = "mint"
		event["to"] = mint.To
		event["amount"] = mint.Amount

	case ev.GetBurn() != nil:
		burn := ev.GetBurn()
		event["type"] = "burn"
		event["from"] = burn.From
		event["amount"] = burn.Amount

	case ev.GetClawback() != nil:
		clawback := ev.GetClawback()
		event["type"] = "clawback"
		event["from"] = clawback.From
		event["amount"] = clawback.Amount

	case ev.GetFee() != nil:
		fee := ev.GetFee()
		event["type"] = "fee"
		event["from"] = fee.From
		event["amount"] = fee.Amount

	default:
		event["type"] = "unknown"
	}

	return event
}
