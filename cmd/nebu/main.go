// Package main implements the nebu CLI for running processors and scaffolding new ones.
package main

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/programstatus"
	"github.com/withObsrvr/nebu/pkg/version"
	"golang.org/x/term"
)

var (
	programStatusMode    string
	programStatusStarted bool
)

func main() {
	os.Exit(execute(newRootCmd()))
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "nebu",
		Short: "nebu - modular streaming runtime for Stellar",
		Long: `nebu is a minimal, Unix-philosophy streaming runtime for Stellar blockchain data.

Build custom indexers, analytics pipelines, and real-time automation by composing
processors that operate on Stellar ledger data via Unix pipes.

QUICK START:
  # Install a processor
  nebu install token-transfer

  # Extract token transfer events
  token-transfer --start-ledger 60200000 --end-ledger 60200100

  # Or pipe from nebu fetch
  nebu fetch 60200000 60200100 | token-transfer

  # Filter and analyze with standard tools
  token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
    jq 'select(.type == "transfer")' | \
    head -10

PROCESSOR TYPES:
  origin     Extract events from ledgers (token-transfer, contract-events)
  transform  Filter/modify events (usdc-filter, amount-filter, dedup)
  sink       Store events (json-file-sink, postgres-sink, nats-sink)

COMMON WORKFLOWS:
  # List available processors
  nebu list

  # Fetch raw ledger data
  nebu fetch 60200000 60200100 > ledgers.xdr

  # Build a full pipeline
  token-transfer --start 60200000 --end 60200100 | \
    usdc-filter | \
    amount-filter --min 1000000 | \
    json-file-sink --out large-usdc.jsonl

ENVIRONMENT:
  NEBU_RPC_URL    RPC endpoint (default: mainnet)
  NEBU_RPC_AUTH   Authorization header (e.g., 'Api-Key xxx')
  NEBU_NETWORK    Network: 'mainnet' or 'testnet'`,
		Version: version.Version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateProgramStatusMode(programStatusMode); err != nil {
				return err
			}
			if err := openStatusStream(cmd); err != nil {
				return err
			}
			programStatusStarted = true
			writeProgramStatus(programstatus.Working, "Running "+cmd.CommandPath())
			return writeStructuredStatus(programstatus.Working, "Running "+cmd.CommandPath())
		},
	}

	// Add global flags
	rootCmd.PersistentFlags().BoolVarP(&quietMode, "quiet", "q", false, "suppress non-error output")
	rootCmd.PersistentFlags().StringVar(&programStatusMode, "program-status", programStatusDefault(), "program status reporting: auto, always, or never (or set NEBU_PROGRAM_STATUS)")
	rootCmd.PersistentFlags().StringVar(&statusFilePath, "status-file", statusFileDefault(), "write nebu.status.v1 NDJSON updates to a dedicated file (or set NEBU_STATUS_FILE)")

	// Add subcommands
	rootCmd.AddCommand(newFetchCmd())
	rootCmd.AddCommand(newInstallCmd())
	rootCmd.AddCommand(newNewCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newDescribeCmd())
	rootCmd.AddCommand(newResumeCmd())
	markUsageErrors(rootCmd)
	return rootCmd
}

func execute(rootCmd *cobra.Command) int {
	err := rootCmd.Execute()
	err = finishStatusStream(err)
	if err != nil {
		writeProgramStatus(programstatus.Error, oneLine(err.Error()))
		fmt.Fprintln(os.Stderr, err)
		return nebuErrors.ExitCode(err)
	}
	if programStatusStarted {
		writeProgramStatus(programstatus.Done, "Command finished")
	}
	return 0
}

func markUsageErrors(command *cobra.Command) {
	if validateArgs := command.Args; validateArgs != nil {
		command.Args = func(cmd *cobra.Command, args []string) error {
			if err := validateArgs(cmd, args); err != nil {
				return nebuErrors.Usage(err)
			}
			return nil
		}
	}
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return nebuErrors.Usage(err)
	})
	for _, child := range command.Commands() {
		markUsageErrors(child)
	}
}

func programStatusDefault() string {
	if value := os.Getenv("NEBU_PROGRAM_STATUS"); value != "" {
		return value
	}
	return "auto"
}

func validateProgramStatusMode(mode string) error {
	switch mode {
	case "auto", "always", "never":
		return nil
	default:
		return nebuErrors.Usage(fmt.Errorf("invalid --program-status value %q: expected auto, always, or never", mode))
	}
}

func programStatusEnabled() bool {
	if validateProgramStatusMode(programStatusMode) != nil {
		return false
	}
	if programStatusMode == "never" || (quietMode && programStatusMode != "always") {
		return false
	}
	if programStatusMode == "always" {
		return true
	}
	return terminalFile(os.Stderr)
}

func terminalFile(file *os.File) bool {
	return os.Getenv("TERM") != "dumb" && term.IsTerminal(int(file.Fd()))
}

func writeProgramStatus(state programstatus.State, message string) {
	if !programStatusEnabled() {
		return
	}
	if err := programstatus.Write(os.Stderr, programstatus.Report{
		State: state, App: "nebu", Message: message,
	}); err != nil {
		// Status reporting is an optional terminal enhancement and must
		// never make the underlying command fail.
		_ = err
	}
}

func oneLine(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	if len(value) > 2048 {
		return strings.ToValidUTF8(value[:2048], "")
	}
	return value
}
