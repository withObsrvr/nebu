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
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/version"
)

const describeFlag = "describe-json"

var (
	networkName string
	archiveURLs []string
	checkpoint  uint32
	limit       uint64
	quiet       bool
	exampleVer  = "0.1.0"
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

A pubnet checkpoint holds tens of millions of entries and takes a long time to
read in full. Use --limit while exploring; the output is in bucket order, not
any order worth sampling, so a limited run is a sample of the bucket list
rather than of the network.`,
		RunE: func(cmd *cobra.Command, _ []string) error { return run(cmd.Context()) },
	}

	rootCmd.Flags().StringVar(&networkName, "network", "mainnet", "Network: 'mainnet' or 'testnet' (or set NEBU_NETWORK)")
	rootCmd.Flags().StringArrayVar(&archiveURLs, "archive-url", nil, "History archive URL, repeatable (defaults to the network's published archives)")
	rootCmd.Flags().Uint32Var(&checkpoint, "checkpoint", 0, "Checkpoint ledger sequence (0 resolves the archive's latest)")
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
		os.Exit(1)
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

	reader, err := checkpointentries.NewReader(urls, passphrase, label, version.Version, checkpoint)
	if err != nil {
		return err
	}

	resolved, err := reader.Checkpoint()
	if err != nil {
		return err
	}
	if !quiet {
		fmt.Fprintf(os.Stderr, "Reading checkpoint %d from %s\n", resolved, urls[0])
		if limit == 0 {
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

	written, err := reader.Stream(ctx, os.Stdout, limit, progress)
	if !quiet {
		fmt.Fprintf(os.Stderr, "\nWrote %d entries from checkpoint %d\n", written, resolved)
	}
	if err != nil && ctx.Err() == nil {
		return err
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
