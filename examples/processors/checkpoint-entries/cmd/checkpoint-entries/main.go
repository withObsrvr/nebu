// Command checkpoint-entries streams the ledger entries in a history archive
// checkpoint as newline-delimited JSON.
//
// It is the nebu shape of the Stellar ingest SDK's "Ledger Entry Statistics"
// example: that one prints four totals, this one emits the entries those
// totals are counted from, so the statistics are a query over the stream
// rather than a struct in the program. examples/README.md carries the
// one-liners that reproduce the SDK example's numbers.
//
// Usage:
//
//	checkpoint-entries --limit 50000 | jq -r .entry_type | sort | uniq -c
//	checkpoint-entries --network testnet --limit 1000
//	checkpoint-entries --checkpoint 65467583 > entries.jsonl
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stellar/go-stellar-sdk/network"

	checkpointentries "github.com/withObsrvr/nebu/examples/processors/checkpoint-entries"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/version"
)

const describeFlag = "describe-json"

var (
	networkName string
	archiveURLs []string
	checkpoint  uint32
	bucketList  string
	limit       uint64
	quiet       bool
	exampleVer  = "0.2.0"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "checkpoint-entries",
		Short:   "Stream the ledger entries in a history archive checkpoint as JSONL",
		Version: exampleVer,
		Long: `Stream the ledger entries in a history archive checkpoint as JSONL.

A checkpoint is a full state snapshot the archive publishes every 64 ledgers,
not a ledger range, so this reads the bucket list rather than LedgerCloseMeta
and is not a nebu origin processor.

A checkpoint publishes two bucket lists, and --bucket-list chooses:

  live  current state: accounts, trustlines, offers, contract data, the rest
  hot   the entries state archival has evicted, which are restorable but no
        longer live

"How many contract data entries exist" is two different questions over those
two lists, and the gap grows with every eviction. Each event carries a
bucket_list field so a file holding both can still be counted correctly.

A pubnet checkpoint holds tens of millions of entries and takes a long time to
read in full. Use --limit while exploring; the output is in bucket order, not
any order worth sampling, so a limited run is a sample of the bucket list
rather than of the network -- and because each bucket's hash is checked only
after its entries have been read, a run that stops at a limit is not
hash-verified.`,
		RunE: func(cmd *cobra.Command, _ []string) error { return run(cmd.Context()) },
	}

	rootCmd.Flags().StringVar(&networkName, "network", "mainnet", "Network: 'mainnet' or 'testnet' (or set NEBU_NETWORK)")
	rootCmd.Flags().StringArrayVar(&archiveURLs, "archive-url", nil, "History archive URL, repeatable (defaults to the network's published archives)")
	rootCmd.Flags().Uint32Var(&checkpoint, "checkpoint", 0, "Checkpoint ledger sequence (0 resolves the archive's latest)")
	rootCmd.Flags().StringVar(&bucketList, "bucket-list", checkpointentries.BucketListLive,
		"Which bucket list to read: 'live' for current state, 'hot' for entries state archival has evicted")
	rootCmd.Flags().Uint64Var(&limit, "limit", 0, "Stop after N entries (0 reads the whole checkpoint)")
	rootCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress progress output")
	rootCmd.Flags().Bool(describeFlag, false, "Emit machine-readable describe envelope to stdout and exit")

	// The describe protocol must answer without any other flag set, so it is
	// checked before cobra parses anything.
	if describeRequested() {
		if err := emitDescribe(os.Stdout, rootCmd); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(nebuErrors.ExitCode(err))
	}
}

func run(ctx context.Context) error {
	passphrase, label, err := resolveNetwork()
	if err != nil {
		return err
	}

	urls := archiveURLs
	if len(urls) == 0 {
		urls = defaultArchives(passphrase)
	}

	reader, err := checkpointentries.NewReader(urls, passphrase, label, version.Version, checkpoint, bucketList)
	if err != nil {
		return err
	}

	resolved, err := reader.Checkpoint()
	if err != nil {
		return err
	}
	if err := checkpointentries.ValidateCheckpoint(resolved); err != nil {
		// Checked here rather than left to the archive, which answers with a
		// 404 the stream turns into a read failure. A ledger that is not a
		// checkpoint is a usage mistake, and retrying it fails the same way.
		return nebuErrors.Usage(err)
	}
	if !quiet {
		fmt.Fprintf(os.Stderr, "Reading the %s bucket list of checkpoint %d from %s\n", bucketList, resolved, urls[0])
		if limit == 0 && bucketList == checkpointentries.BucketListLive {
			fmt.Fprintln(os.Stderr, "Reading the whole checkpoint; a pubnet snapshot is tens of millions of entries.")
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		if !quiet {
			fmt.Fprintln(os.Stderr, "\nShutting down...")
		}
		cancel()
	}()

	progress := checkpointentries.ReportProgress
	if quiet {
		progress = nil
	}

	result, err := reader.Stream(ctx, os.Stdout, limit, progress)
	if !quiet {
		fmt.Fprintf(os.Stderr, "\nWrote %d entries from the %s bucket list of checkpoint %d\n",
			result.Written, bucketList, resolved)
	}

	if err != nil && ctx.Err() == nil && result.Written == 0 {
		// Nothing reached stdout, so there is nothing to distrust: this is an
		// ordinary failure, not a partial snapshot.
		return err
	}

	if err != nil && ctx.Err() == nil {
		// A failed checkpoint read that already wrote entries is not a short
		// read. Each bucket's hash is
		// checked after its entries are yielded, so an error means a bucket
		// either failed its check or never reached it: the entries already on
		// stdout are not known to belong to this checkpoint. Exit code 8 is
		// nebu's "ran, but did not cover what was asked", which is exactly
		// this, and a pipeline reading the stream can act on it instead of
		// treating a truncated snapshot as a whole one.
		return &nebuErrors.NebuError{
			Kind:    nebuErrors.KindIncomplete,
			Cause:   err,
			Message: fmt.Sprintf("checkpoint %d read failed after %d entries: %v", resolved, result.Written, err),
			Suggestion: fmt.Sprintf(`The %d entries already written are not verified and should be discarded:
a bucket's hash is checked only once its entries have been read, so a read
that stopped here never confirmed the bucket it was in belongs to checkpoint
%d.

Retry the same checkpoint -- the archive is immutable, so a transient fetch
failure is worth retrying and a persistent one is a real archive problem.`,
				result.Written, resolved),
		}
	}

	if !result.Complete && !quiet {
		fmt.Fprintln(os.Stderr,
			"Note: the read stopped before the end of the bucket list, so these entries are "+
				"not hash-verified -- each bucket is checked only after all of its entries "+
				"have been read. Drop --limit for a verified snapshot.")
	}
	return nil
}

// resolveNetwork turns the --network flag, or NEBU_NETWORK, into a passphrase
// and the short label the events carry.
func resolveNetwork() (passphrase, label string, err error) {
	name := networkName
	if env := os.Getenv("NEBU_NETWORK"); env != "" && name == "mainnet" {
		name = env
	}

	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mainnet", "main", "public", "pubnet":
		return network.PublicNetworkPassphrase, "pubnet", nil
	case "testnet", "test":
		return network.TestNetworkPassphrase, "testnet", nil
	default:
		// A full passphrase is accepted, but it cannot be labelled, and the
		// archive check compares it to what the archive publishes.
		if name == network.PublicNetworkPassphrase {
			return name, "pubnet", nil
		}
		if name == network.TestNetworkPassphrase {
			return name, "testnet", nil
		}
		return "", "", fmt.Errorf("unknown network %q: use 'mainnet' or 'testnet'", name)
	}
}

func defaultArchives(passphrase string) []string {
	if passphrase == network.TestNetworkPassphrase {
		return network.TestNetworkhistoryArchiveURLs
	}
	return network.PublicNetworkhistoryArchiveURLs
}

// describeRequested scans os.Args directly, the way nebu's own CLI helpers do,
// so --describe-json works before cobra validates anything.
func describeRequested() bool {
	flag := "--" + describeFlag
	for _, arg := range os.Args[1:] {
		if arg == flag {
			return true
		}
		if value, ok := strings.CutPrefix(arg, flag+"="); ok {
			parsed, err := strconv.ParseBool(value)
			return err == nil && parsed
		}
	}
	return false
}

func emitDescribe(out *os.File, cmd *cobra.Command) error {
	envelope := processor.DescribeEnvelope{
		Name:        "checkpoint-entries",
		Type:        processor.TypeOrigin.String(),
		Version:     exampleVer,
		Description: cmd.Short,
		Schema: processor.DescribeSchema{
			ID: checkpointentries.SchemaID,
		},
	}

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name == describeFlag || f.Name == "help" || f.Name == "version" {
			return
		}
		envelope.Flags = append(envelope.Flags, processor.DescribeFlag{
			Name:        f.Name,
			Type:        f.Value.Type(),
			Description: f.Usage,
			Default:     f.DefValue,
		})
	})

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(envelope)
}
