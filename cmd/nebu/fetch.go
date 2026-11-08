package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/support/datastore"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
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
		datastoreType      string
		bucketPath         string
		region             string
		bufferSize         int
		numWorkers         int
		ledgersPerFile     int
		filesPerPartition  int
		checkpointPath     string
		checkpointInterval uint32
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
			bufferSizeConfig := getEnvOrFlagInt("NEBU_BUFFER_SIZE", bufferSize, 0)
			numWorkersConfig := getEnvOrFlagInt("NEBU_NUM_WORKERS", numWorkers, 0)
			ledgersPerFileConfig := getEnvOrFlagInt("NEBU_LEDGERS_PER_FILE", ledgersPerFile, 0)
			filesPerPartitionConfig := getEnvOrFlagInt("NEBU_FILES_PER_PARTITION", filesPerPartition, 0)

			// Resolved here rather than at the fetch call so the startup
			// banner reports the tuning actually in effect.
			archiveBuffer := archiveBufferConfig(
				bufferSizeConfig.Value, numWorkersConfig.Value, ledgersPerFileConfig.Value,
			)

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

			if err := validateCheckpointOutputPaths(checkpointPath, outputFile); err != nil {
				return nebuErrors.Usage(err)
			}

			checkpoint, resumedStart, alreadyComplete, err := openCheckpoint(
				checkpointPath, startLedger, endLedger, checkpointInterval,
			)
			if err != nil {
				return err
			}
			if alreadyComplete {
				logInfo("Checkpoint already covers requested range through ledger %d", endLedger)
				return nil
			}
			if resumedStart != startLedger {
				logInfo("Resuming from checkpoint at ledger %d", resumedStart)
				startLedger = resumedStart
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
					"Buffer Size": fmt.Sprintf("%d", archiveBuffer.BufferSize),
					"Workers":     fmt.Sprintf("%d", archiveBuffer.NumWorkers),
					"Layout":      archiveLayoutDisplay(ledgersPerFileConfig.Value, filesPerPartitionConfig.Value),
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
				os.Exit(130)
			}()

			// Call fetchLedgers with mode-specific configuration
			if modeConfig.Value == "archive" {
				return fetchLedgersArchive(
					ctx,
					archiveOptions{
						datastoreType:     datastoreTypeConfig.Value,
						bucketPath:        bucketPathConfig.Value,
						region:            regionConfig.Value,
						networkPassphrase: normalizeNetworkPassphrase(networkConfig.Value),
						schema: archiveDataStoreSchema(
							ledgersPerFileConfig.Value, filesPerPartitionConfig.Value,
						),
						buffer: archiveBuffer,
					},
					startLedger,
					endLedger,
					outputFile,
					checkpoint,
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
					checkpoint,
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
	cmd.Flags().IntVar(&bufferSize, "buffer-size", 0, "Number of ledgers to buffer; 0 tunes from the datastore layout (or set NEBU_BUFFER_SIZE)")
	cmd.Flags().IntVar(&numWorkers, "num-workers", 0, "Number of parallel fetch workers; 0 tunes from the datastore layout (or set NEBU_NUM_WORKERS)")
	cmd.Flags().IntVar(&ledgersPerFile, "ledgers-per-file", 0, "Datastore ledgers per file; 0 reads it from the datastore manifest (or set NEBU_LEDGERS_PER_FILE)")
	cmd.Flags().IntVar(&filesPerPartition, "files-per-partition", 0, "Datastore files per partition; 0 reads it from the datastore manifest (or set NEBU_FILES_PER_PARTITION)")

	// Common flags
	cmd.Flags().StringVar(&outputFile, "output", "", "Output file (default: stdout)")
	cmd.Flags().StringVar(&checkpointPath, "checkpoint", "", "Resume checkpoint file (opt-in)")
	cmd.Flags().Uint32Var(&checkpointInterval, "checkpoint-interval", 1, "Ledgers between durable checkpoint writes")

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

func fetchLedgersRPC(ctx context.Context, rpcURL, _ string, start, end uint32, outputFile, authHeader string, checkpoint *checkpointWriter) error {
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
	return fetchRawLedgers(ctx, stream, start, end, outputFile, checkpoint)
}

// archiveOptions carries the resolved archive-mode configuration.
type archiveOptions struct {
	datastoreType     string
	bucketPath        string
	region            string
	networkPassphrase string
	schema            datastore.DataStoreSchema
	buffer            ledgerbackend.BufferedStorageBackendConfig
}

// archiveDataStoreSchema builds the local schema overrides for archive mode.
//
// The zero values are deliberate. datastore.LoadSchema — which
// NewBufferedStorageStream calls on the first pull — reads the layout from the
// manifest the exporter published in the bucket, and compares only the fields
// the local config actually sets. Leaving a field at 0 therefore means "take
// it from the datastore". nebu previously asserted a hardcoded 1 ledger per
// file and 64000 files per partition against every bucket, which made any
// other layout fail as a config mismatch with no flag available to correct it.
func archiveDataStoreSchema(ledgersPerFile, filesPerPartition int) datastore.DataStoreSchema {
	return datastore.DataStoreSchema{
		LedgersPerFile:    uint32(ledgersPerFile),
		FilesPerPartition: uint32(filesPerPartition),
	}
}

// archiveBufferConfig resolves the buffered-storage tuning for archive mode.
//
// With the flags left at 0 the values come from the SDK's
// ingest.DefaultBufferedStorageBackendConfig, which picks them from the
// datastore's ledgers-per-file against its own benchmarks. nebu's previous
// fixed BufferSize of 100 throttled bounded backfills on the common
// one-ledger-per-file layout, where the SDK asks for 10000 so the workers see
// the whole queue up front instead of pacing on the per-pull feedback loop.
// The buffer is capped to the size of the requested range, so the larger
// number is an upper bound rather than a fixed allocation.
//
// ledgersPerFile is 0 whenever the layout is coming from the manifest, which
// is not read until streaming starts; the SDK's small-layout default is the
// right guess in that case, since that is the layout it describes.
func archiveBufferConfig(bufferSize, numWorkers, ledgersPerFile int) ledgerbackend.BufferedStorageBackendConfig {
	config := ingest.DefaultBufferedStorageBackendConfig(uint32(ledgersPerFile))
	if bufferSize > 0 {
		config.BufferSize = uint32(bufferSize)
	}
	if numWorkers > 0 {
		config.NumWorkers = uint32(numWorkers)
	}
	return config
}

// archiveLayoutDisplay describes the resolved datastore layout for the banner.
func archiveLayoutDisplay(ledgersPerFile, filesPerPartition int) string {
	if ledgersPerFile == 0 && filesPerPartition == 0 {
		return "from datastore manifest"
	}
	parts := make([]string, 0, 2)
	if ledgersPerFile > 0 {
		parts = append(parts, fmt.Sprintf("%d ledgers/file", ledgersPerFile))
	} else {
		parts = append(parts, "ledgers/file from manifest")
	}
	if filesPerPartition > 0 {
		parts = append(parts, fmt.Sprintf("%d files/partition", filesPerPartition))
	} else {
		parts = append(parts, "files/partition from manifest")
	}
	return strings.Join(parts, ", ")
}

func fetchLedgersArchive(
	ctx context.Context,
	opts archiveOptions,
	start, end uint32,
	outputFile string,
	checkpoint *checkpointWriter,
) error {
	datastoreConfig := datastore.DataStoreConfig{
		Type: opts.datastoreType,
		Params: map[string]string{
			"destination_bucket_path": opts.bucketPath,
		},
		Schema: opts.schema,
		// Setting the passphrase lets datastore.LoadSchema check it against
		// the manifest in the bucket, so pointing a pubnet-configured fetch
		// at a testnet export fails loudly rather than returning ledgers
		// from the wrong network.
		NetworkPassphrase: opts.networkPassphrase,
	}
	if opts.datastoreType == "S3" {
		datastoreConfig.Params["region"] = opts.region
	}

	stream := ledgerbackend.NewBufferedStorageStream(opts.buffer, datastoreConfig, nil)
	return fetchRawLedgers(ctx, stream, start, end, outputFile, checkpoint)
}

func fetchRawLedgers(
	ctx context.Context,
	stream ledgerbackend.LedgerStream,
	start, end uint32,
	outputFile string,
	checkpoint *checkpointWriter,
) error {
	ledgerRange, err := rawLedgerRange(start, end)
	if err != nil {
		return err
	}

	var output io.Writer = os.Stdout
	var outputFileHandle *os.File
	if outputFile != "" {
		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if checkpoint.resuming() {
			flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		outputFileHandle, err = os.OpenFile(outputFile, flags, 0o644)
		if err != nil {
			return nebuErrors.FailedToCreateFile(outputFile, err)
		}
		output = outputFileHandle
	}

	outputSyncFailed := false
	var syncOutput func() error
	if outputFileHandle != nil {
		syncOutput = func() error {
			if err := outputFileHandle.Sync(); err != nil {
				outputSyncFailed = true
				return fmt.Errorf("sync output file %q: %w", outputFile, err)
			}
			return nil
		}
	}

	ledgerCount, streamErr := writeRawLedgersWithCheckpoint(
		ctx, stream, ledgerRange, output, start, end, checkpoint, syncOutput,
	)
	if outputFileHandle != nil {
		if syncErr := syncOutput(); syncErr != nil {
			streamErr = errors.Join(streamErr, syncErr)
		}
		if closeErr := outputFileHandle.Close(); closeErr != nil {
			outputSyncFailed = true
			streamErr = errors.Join(streamErr, fmt.Errorf("failed to close output file %q: %w", outputFile, closeErr))
		}
	}
	if ledgerCount > 0 && checkpoint != nil && !outputSyncFailed {
		last := uint64(start) + uint64(ledgerCount) - 1
		if err := checkpoint.advance(uint32(last), true); err != nil {
			streamErr = errors.Join(streamErr, fmt.Errorf("save final checkpoint after ledger %d: %w", last, err))
		}
	}
	if streamErr != nil {
		if end > 0 {
			if summaryErr := writeFetchFailureSummary(os.Stderr, start, end, ledgerCount, streamErr); summaryErr != nil {
				streamErr = errors.Join(streamErr, fmt.Errorf("failed to write fetch failure summary: %w", summaryErr))
			}
		}
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
	return writeRawLedgersWithCheckpoint(ctx, stream, ledgerRange, output, start, end, nil, nil)
}

func writeRawLedgersWithCheckpoint(
	ctx context.Context,
	stream ledgerbackend.LedgerStream,
	ledgerRange ledgerbackend.Range,
	output io.Writer,
	start, end uint32,
	checkpoint *checkpointWriter,
	syncOutput func() error,
) (ledgerCount int, retErr error) {
	reportInterval := 10
	if end == 0 {
		reportInterval = 100
	}

	for raw, streamErr := range stream.RawLedgers(ctx, ledgerRange) {
		if streamErr != nil {
			if errors.Is(streamErr, context.Canceled) {
				return ledgerCount, context.Canceled
			}
			return ledgerCount, nebuErrors.StreamError(streamErr)
		}

		select {
		case <-ctx.Done():
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
		if err := writeStructuredProgress(uint32(sequence), end); err != nil {
			return ledgerCount, fmt.Errorf("write status after ledger %d: %w", sequence, err)
		}
		if checkpoint.willPersist(false) && syncOutput != nil {
			if err := syncOutput(); err != nil {
				return ledgerCount, fmt.Errorf("sync output before checkpoint after ledger %d: %w", sequence, err)
			}
		}
		if err := checkpoint.advance(uint32(sequence), false); err != nil {
			return ledgerCount, fmt.Errorf("save checkpoint after ledger %d: %w", sequence, err)
		}
		if ledgerCount%reportInterval == 0 {
			if end == 0 {
				logInfo("Streaming... fetched %d ledgers (current: %d)", ledgerCount, sequence)
			} else {
				logInfo("Fetched %d ledgers...", ledgerCount)
			}
		}
	}

	if end > 0 {
		expected := uint64(end) - uint64(start) + 1
		if uint64(ledgerCount) != expected {
			return ledgerCount, fmt.Errorf(
				"incomplete fetch: stream ended after %d of %d ledgers",
				ledgerCount,
				expected,
			)
		}
	}

	return ledgerCount, nil
}

func writeFetchFailureSummary(w io.Writer, start, end uint32, fetched int, reason error) error {
	expected := uint64(end) - uint64(start) + 1
	last := "none"
	if fetched > 0 {
		last = fmt.Sprintf("%d", uint64(start)+uint64(fetched)-1)
	}
	_, err := fmt.Fprintf(
		w,
		"fetched %d/%d ledgers, last=%s, reason=%s\n",
		fetched,
		expected,
		last,
		oneLine(reason.Error()),
	)
	return err
}
