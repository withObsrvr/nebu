package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/support/datastore"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/source/storage"
)

func newFetchCmd() *cobra.Command {
	var (
		// RPC mode flags
		rpcURL      string
		startLedger uint32
		endLedger   uint32
		networkPass string
		outputFile  string
		rpcHeaders  []string
		follow      bool

		// Mode selection
		mode string

		// Archive mode flags
		datastoreType string
		bucketPath    string
		region        string
		bufferSize    int
		numWorkers    int
	)

	cmd := &cobra.Command{
		Use:   "fetch [start-ledger] [end-ledger]",
		Short: "Fetch ledgers from Stellar RPC or archive storage and output XDR",
		Long: `Fetch ledgers from Stellar RPC or archive storage (GCS/S3) and output raw XDR to stdout or file.

This command fetches ledgers in the specified range (bounded or unbounded) and
outputs them as raw XDR data that can be piped to processors or saved for later use.

Modes:
  rpc     - Fetch from Stellar RPC endpoint (default, best for recent ledgers)
  archive - Fetch from GCS/S3 storage (best for historical data and lakehouse building)

RPC Mode Examples:
  # Fetch bounded range
  nebu fetch 60200000 60200100 > ledgers.xdr

  # Stream continuously (follow mode, like 'tail -f')
  nebu fetch 60200000 --follow > ledgers.xdr
  nebu fetch 60200000 -f > ledgers.xdr

  # Alternative: set end-ledger to 0 for unbounded streaming
  nebu fetch 60200000 0 > ledgers.xdr

  # Pipe directly to processor
  nebu fetch 60200000 60200100 | token-transfer

Archive Mode Examples:
  # Fetch from GCS bucket
  nebu fetch --mode archive --bucket-path "my-bucket/ledgers" 60200000 60200100 > ledgers.xdr

  # Fetch from S3 with configuration
  nebu fetch --mode archive --datastore-type S3 --bucket-path "my-bucket/ledgers" \
    --region us-west-2 60200000 60200100 > ledgers.xdr

  # Use environment variables
  export NEBU_MODE=archive
  export NEBU_BUCKET_PATH="stellar-data/mainnet"
  nebu fetch 60200000 60200100 | gzip > historical.xdr.gz
`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Auto-quiet when stdout is a pipe and outputting binary XDR.
			// Prevents status messages from corrupting the stream if stderr
			// is merged with stdout (common in some SSH/shell configurations).
			if outputFile == "" {
				if stat, _ := os.Stdout.Stat(); stat != nil && (stat.Mode()&os.ModeCharDevice) == 0 {
					quietMode = true
				}
			}

			// Parse start ledger (always required)
			var err error
			_, err = fmt.Sscanf(args[0], "%d", &startLedger)
			if err != nil {
				return nebuErrors.InvalidLedgerFormat(args[0], "start")
			}

			if startLedger == 0 {
				return nebuErrors.LedgerMustBePositive("Start")
			}

			// Parse end ledger based on args and flags
			if len(args) == 2 {
				// Two arguments provided: use end-ledger from args
				_, err = fmt.Sscanf(args[1], "%d", &endLedger)
				if err != nil {
					return nebuErrors.InvalidLedgerFormat(args[1], "end")
				}
			} else if follow {
				// Only one argument + --follow flag: stream indefinitely
				endLedger = 0
			} else {
				// Only one argument, no --follow: fetch single ledger
				endLedger = startLedger
			}

			// Validate ledger range
			// endLedger == 0 is valid (unbounded streaming)
			if endLedger > 0 && startLedger > endLedger {
				return nebuErrors.InvalidLedgerRange(startLedger, endLedger)
			}

			// Get mode configuration
			modeConfig := getEnvOrFlag("NEBU_MODE", mode, "rpc")

			// Get RPC configuration (used when mode == "rpc")
			rpcConfig := getEnvOrFlag("NEBU_RPC_URL", rpcURL, "https://rpc.lightsail.network")
			networkConfig := getEnvOrFlag("NEBU_NETWORK", networkPass, network.PublicNetworkPassphrase)
			authHeader := getAuthHeader(rpcHeaders)

			// Get archive configuration (used when mode == "archive")
			datastoreTypeConfig := getEnvOrFlag("NEBU_DATASTORE_TYPE", datastoreType, "GCS")
			bucketPathConfig := getEnvOrFlag("NEBU_BUCKET_PATH", bucketPath, "")
			regionConfig := getEnvOrFlag("NEBU_REGION", region, "us-east-1")
			bufferSizeConfig := getEnvOrFlagInt("NEBU_BUFFER_SIZE", bufferSize, 100)
			numWorkersConfig := getEnvOrFlagInt("NEBU_NUM_WORKERS", numWorkers, 10)

			// Validate configuration based on mode
			if modeConfig.Value == "archive" {
				if bucketPathConfig.Value == "" {
					return nebuErrors.MissingBucketPath()
				}
			} else if modeConfig.Value == "rpc" {
				// Validate network configuration for RPC mode
				if err := validateNetworkConfig(rpcConfig.Value, networkConfig.Value); err != nil {
					return err
				}
			} else {
				return nebuErrors.InvalidMode(modeConfig.Value, []string{"rpc", "archive"})
			}

			// Display startup banner
			outputDest := "stdout"
			if outputFile != "" {
				outputDest = outputFile
			}
			rangeDisplay := fmt.Sprintf("%d → %d", startLedger, endLedger)
			if endLedger == 0 {
				rangeDisplay = fmt.Sprintf("%d → ∞ (unbounded)", startLedger)
			}

			// Mode-specific banner
			if modeConfig.Value == "archive" {
				printStartupBanner("fetch", map[string]ConfigSource{
					"Mode":      modeConfig,
					"Datastore": datastoreTypeConfig,
					"Bucket":    bucketPathConfig,
					"Region":    regionConfig,
				}, map[string]string{
					"Range":       rangeDisplay,
					"Buffer Size": fmt.Sprintf("%d", bufferSizeConfig.Value),
					"Workers":     fmt.Sprintf("%d", numWorkersConfig.Value),
					"Output":      outputDest,
				})
			} else {
				authConfig := getAuthConfig(authHeader)
				printStartupBanner("fetch", map[string]ConfigSource{
					"RPC":     rpcConfig,
					"Network": {Value: getNetworkDisplay(networkConfig.Value), Source: networkConfig.Source},
					"Auth":    authConfig,
				}, map[string]string{
					"Range":  rangeDisplay,
					"Output": outputDest,
				})
			}

			// Create context with cancellation
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle Ctrl+C
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigCh
				logInfo("\nShutting down...")
				cancel()

				// Force exit after 2 seconds if graceful shutdown fails
				time.Sleep(2 * time.Second)
				logInfo("Force shutdown after timeout")
				os.Exit(1)
			}()

			// Call fetchLedgers with mode-specific configuration
			if modeConfig.Value == "archive" {
				return fetchLedgersArchive(
					ctx,
					datastoreTypeConfig.Value,
					bucketPathConfig.Value,
					regionConfig.Value,
					uint32(bufferSizeConfig.Value),
					uint32(numWorkersConfig.Value),
					startLedger,
					endLedger,
					outputFile,
				)
			} else {
				return fetchLedgersRPC(
					ctx,
					rpcConfig.Value,
					networkConfig.Value,
					startLedger,
					endLedger,
					outputFile,
					authHeader,
				)
			}
		},
	}

	// Mode selection
	cmd.Flags().StringVar(&mode, "mode", "rpc", "Data source mode: 'rpc' or 'archive' (or set NEBU_MODE)")

	// RPC mode flags
	cmd.Flags().StringVar(&rpcURL, "rpc-url", "https://archive-rpc.lightsail.network", "Stellar RPC endpoint (or set NEBU_RPC_URL)")
	cmd.Flags().StringVar(&networkPass, "network", network.PublicNetworkPassphrase, "Network passphrase: 'mainnet' or 'testnet' (or set NEBU_NETWORK)")
	cmd.Flags().StringArrayVar(&rpcHeaders, "rpc-header", nil, "Custom HTTP header for RPC (format: 'Key: Value', repeatable)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream ledgers continuously from start-ledger (same as setting end-ledger to 0)")

	// Archive mode flags
	cmd.Flags().StringVar(&datastoreType, "datastore-type", "GCS", "Datastore type: 'GCS' or 'S3' (or set NEBU_DATASTORE_TYPE)")
	cmd.Flags().StringVar(&bucketPath, "bucket-path", "", "Storage bucket path (or set NEBU_BUCKET_PATH)")
	cmd.Flags().StringVar(&region, "region", "us-east-1", "S3 region (or set NEBU_REGION)")
	cmd.Flags().IntVar(&bufferSize, "buffer-size", 100, "Number of ledgers to buffer (or set NEBU_BUFFER_SIZE)")
	cmd.Flags().IntVar(&numWorkers, "num-workers", 10, "Number of parallel fetch workers (or set NEBU_NUM_WORKERS)")

	// Common flags
	cmd.Flags().StringVar(&outputFile, "output", "", "Output file (default: stdout)")

	return cmd
}

// fetchHeaderTransport adds fetch-specific headers without mutating requests
// owned by the Stellar RPC client.
type fetchHeaderTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *fetchHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqCopy := req.Clone(req.Context())
	for key, value := range t.headers {
		reqCopy.Header.Set(key, value)
	}
	return t.base.RoundTrip(reqCopy)
}

func fetchLedgersRPC(ctx context.Context, rpcURL, _ string, start, end uint32, outputFile, authHeader string) error {
	if rpcURL == "" {
		return nebuErrors.FailedToCreateSource("RPC", errors.New("rpcURL cannot be empty"))
	}

	options := ledgerbackend.RPCLedgerBackendOptions{RPCServerURL: rpcURL}
	if authHeader != "" {
		options.HttpClient = &http.Client{Transport: &fetchHeaderTransport{
			base:    http.DefaultTransport,
			headers: map[string]string{"Authorization": authHeader},
		}}
	}

	stream := ledgerbackend.NewRPCStream(options, nil)
	return fetchRawLedgers(ctx, stream, start, end, outputFile)
}

func fetchLedgersArchive(
	ctx context.Context,
	datastoreType, bucketPath, region string,
	bufferSize, numWorkers uint32,
	start, end uint32,
	outputFile string,
) error {
	datastoreConfig := datastore.DataStoreConfig{
		Type: datastoreType,
		Params: map[string]string{
			"destination_bucket_path": bucketPath,
		},
		Schema: storage.DefaultDataStoreSchema(),
	}
	if datastoreType == "S3" {
		datastoreConfig.Params["region"] = region
	}

	bufferConfig := ledgerbackend.BufferedStorageBackendConfig{
		BufferSize: bufferSize,
		NumWorkers: numWorkers,
		RetryLimit: 3,
		RetryWait:  5 * time.Second,
	}

	stream := ledgerbackend.NewBufferedStorageStream(bufferConfig, datastoreConfig, nil)
	return fetchRawLedgers(ctx, stream, start, end, outputFile)
}

func fetchRawLedgers(
	ctx context.Context,
	stream ledgerbackend.LedgerStream,
	start, end uint32,
	outputFile string,
) error {
	ledgerRange, err := rawLedgerRange(start, end)
	if err != nil {
		return err
	}

	var output io.Writer = os.Stdout
	var outputFileHandle *os.File
	if outputFile != "" {
		outputFileHandle, err = os.Create(outputFile)
		if err != nil {
			return nebuErrors.FailedToCreateFile(outputFile, err)
		}
		output = outputFileHandle
	}

	ledgerCount, streamErr := writeRawLedgers(ctx, stream, ledgerRange, output, start, end)
	if outputFileHandle != nil {
		if closeErr := outputFileHandle.Close(); closeErr != nil {
			streamErr = errors.Join(streamErr, fmt.Errorf("failed to close output file %q: %w", outputFile, closeErr))
		}
	}
	if streamErr != nil {
		return streamErr
	}

	if end == 0 {
		logInfo("Stopped streaming at %d ledgers", ledgerCount)
	} else {
		logInfo("Fetched %d ledgers", ledgerCount)
	}
	return nil
}

func rawLedgerRange(start, end uint32) (ledgerbackend.Range, error) {
	if start == 0 {
		return ledgerbackend.Range{}, errors.New("start ledger must be greater than zero")
	}
	if end > 0 && start > end {
		return ledgerbackend.Range{}, fmt.Errorf("invalid ledger range: start %d exceeds end %d", start, end)
	}
	if end == 0 {
		return ledgerbackend.UnboundedRange(start), nil
	}
	return ledgerbackend.BoundedRange(start, end), nil
}

func writeRawLedgers(
	ctx context.Context,
	stream ledgerbackend.LedgerStream,
	ledgerRange ledgerbackend.Range,
	output io.Writer,
	start, end uint32,
) (int, error) {
	ledgerCount := 0
	reportInterval := 10
	if end == 0 {
		reportInterval = 100
	}

	for raw, streamErr := range stream.RawLedgers(ctx, ledgerRange) {
		if streamErr != nil {
			if end == 0 && errors.Is(streamErr, context.Canceled) {
				return ledgerCount, nil
			}
			return ledgerCount, nebuErrors.StreamError(streamErr)
		}

		select {
		case <-ctx.Done():
			if end == 0 {
				return ledgerCount, nil
			}
			return ledgerCount, ctx.Err()
		default:
		}

		sequence := uint64(start) + uint64(ledgerCount)
		n, err := output.Write(raw)
		if err != nil {
			return ledgerCount, fmt.Errorf("failed to write ledger %d: %w", sequence, err)
		}
		if n != len(raw) {
			return ledgerCount, fmt.Errorf("failed to write ledger %d: wrote %d of %d bytes: %w", sequence, n, len(raw), io.ErrShortWrite)
		}

		ledgerCount++
		if ledgerCount%reportInterval == 0 {
			if end == 0 {
				logInfo("Streaming... fetched %d ledgers (current: %d)", ledgerCount, sequence)
			} else {
				logInfo("Fetched %d ledgers...", ledgerCount)
			}
		}
	}

	return ledgerCount, nil
}
