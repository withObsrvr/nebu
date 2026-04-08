package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/support/datastore"
	"github.com/stellar/go-stellar-sdk/xdr"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/source/rpc"
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
  nebu fetch 60200000 60200100 | nebu run origin token-transfer

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

func fetchLedgersRPC(ctx context.Context, rpcURL, networkPass string, start, end uint32, outputFile, authHeader string) error {
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

	// Determine output destination
	var output *os.File
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return nebuErrors.FailedToCreateFile(outputFile, err)
		}
		defer f.Close()
		output = f
	} else {
		output = os.Stdout
	}

	// Stream ledgers
	ledgerCh := make(chan xdr.LedgerCloseMeta, 128)
	errCh := make(chan error, 1)

	go func() {
		err := src.Stream(ctx, start, end, ledgerCh)
		if err != nil && err != context.Canceled {
			errCh <- err
		}
		close(errCh)
	}()

	ledgerCount := 0
	for ledger := range ledgerCh {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Write XDR to output using xdr.Marshal (includes framing)
		_, err := xdr.Marshal(output, ledger)
		if err != nil {
			return fmt.Errorf("failed to marshal ledger %d: %w", ledger.LedgerSequence(), err)
		}

		ledgerCount++
		// For unbounded streaming, report progress more frequently
		reportInterval := 10
		if end == 0 {
			reportInterval = 100 // Report every 100 ledgers for unbounded
		}
		if ledgerCount%reportInterval == 0 {
			if end == 0 {
				logInfo("Streaming... fetched %d ledgers (current: %d)", ledgerCount, ledger.LedgerSequence())
			} else {
				logInfo("Fetched %d ledgers...", ledgerCount)
			}
		}
	}

	// Check for stream errors
	if err := <-errCh; err != nil {
		return nebuErrors.StreamError(err)
	}

	if end == 0 {
		logInfo("Stopped streaming at %d ledgers", ledgerCount)
	} else {
		logInfo("Fetched %d ledgers", ledgerCount)
	}
	return nil
}

func fetchLedgersArchive(
	ctx context.Context,
	datastoreType, bucketPath, region string,
	bufferSize, numWorkers uint32,
	start, end uint32,
	outputFile string,
) error {
	// Build datastore configuration
	datastoreConfig := datastore.DataStoreConfig{
		Type: datastoreType,
		Params: map[string]string{
			"destination_bucket_path": bucketPath,
		},
		Schema: storage.DefaultDataStoreSchema(),
	}

	// Add region for S3
	if datastoreType == "S3" {
		datastoreConfig.Params["region"] = region
	}

	// Build buffer configuration
	bufferConfig := ledgerbackend.BufferedStorageBackendConfig{
		BufferSize: bufferSize,
		NumWorkers: numWorkers,
		RetryLimit: 3,
		RetryWait:  5 * time.Second,
	}

	// Create storage source
	src, err := storage.NewLedgerSource(datastoreConfig, bufferConfig)
	if err != nil {
		return nebuErrors.FailedToCreateSource("storage", err)
	}
	defer src.Close()

	// Determine output destination
	var output *os.File
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return nebuErrors.FailedToCreateFile(outputFile, err)
		}
		defer f.Close()
		output = f
	} else {
		output = os.Stdout
	}

	// Stream ledgers
	ledgerCh := make(chan xdr.LedgerCloseMeta, 128)
	errCh := make(chan error, 1)

	go func() {
		err := src.Stream(ctx, start, end, ledgerCh)
		if err != nil && err != context.Canceled {
			errCh <- err
		}
		close(errCh)
	}()

	ledgerCount := 0
	for ledger := range ledgerCh {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Write XDR to output using xdr.Marshal (includes framing)
		_, err := xdr.Marshal(output, ledger)
		if err != nil {
			return fmt.Errorf("failed to marshal ledger %d: %w", ledger.LedgerSequence(), err)
		}

		ledgerCount++
		// For unbounded streaming, report progress more frequently
		reportInterval := 10
		if end == 0 {
			reportInterval = 100 // Report every 100 ledgers for unbounded
		}
		if ledgerCount%reportInterval == 0 {
			if end == 0 {
				logInfo("Streaming... fetched %d ledgers (current: %d)", ledgerCount, ledger.LedgerSequence())
			} else {
				logInfo("Fetched %d ledgers...", ledgerCount)
			}
		}
	}

	// Check for stream errors
	if err := <-errCh; err != nil {
		return nebuErrors.StreamError(err)
	}

	if end == 0 {
		logInfo("Stopped streaming at %d ledgers", ledgerCount)
	} else {
		logInfo("Fetched %d ledgers", ledgerCount)
	}
	return nil
}
