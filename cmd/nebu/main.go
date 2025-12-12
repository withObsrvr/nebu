// Package main implements the nebu CLI for running processors and scaffolding new ones.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "0.2.0"

func main() {
	rootCmd := &cobra.Command{
		Use:   "nebu",
		Short: "nebu - modular streaming runtime for Stellar",
		Long: `nebu is a minimal, IDL-first streaming runtime for building
modular data pipelines on Stellar.

Build custom indexers, analytics pipelines, and real-time automation
by composing processors that operate on Stellar ledger data.`,
		Version: version,
	}

	// Add global flags
	rootCmd.PersistentFlags().BoolVarP(&quietMode, "quiet", "q", false, "suppress non-error output")

	// Add subcommands
	rootCmd.AddCommand(newFetchCmd())
	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newInstallCmd())
	rootCmd.AddCommand(newNewCmd())
	rootCmd.AddCommand(newListCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
