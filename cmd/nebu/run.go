package main

import (
	"fmt"

	"github.com/spf13/cobra"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/registry"
)

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run processors against Stellar RPC",
		Long:  `Run origin, transform, or sink processors against Stellar ledger data.`,
	}

	cmd.AddCommand(newRunOriginCmd())
	cmd.AddCommand(newRunSinkCmd())

	return cmd
}

func newRunOriginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "origin [processor-name]",
		Short: "Show how to run an origin processor",
		Long: `Origin processors run as standalone binaries, not embedded in nebu.
This command validates that a processor is registered and provides instructions
for installing and running it.

The nebu CLI provides runtime infrastructure (registry, fetch, install), but
processors themselves run as separate executables to keep nebu minimal.

Workflow:
  1. List available processors:     nebu list
  2. Install a processor:            nebu install <processor-name>
  3. Run the processor:              <processor-name> --start-ledger X --end-ledger Y
  4. Or pipe from nebu fetch:        nebu fetch X Y | <processor-name>

Examples:
  # See available processors
  nebu list

  # Install token-transfer processor
  nebu install token-transfer

  # Run it directly
  token-transfer --start-ledger 60200000 --end-ledger 60200100

  # Or pipe from nebu fetch
  nebu fetch 60200000 60200100 | token-transfer

  # Build manually if needed
  cd examples/processors/token-transfer/cmd && go build -o token-transfer
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			processorName := args[0]

			// Load registry and validate processor exists
			reg, err := registry.LoadDefault()
			if err != nil {
				return nebuErrors.RegistryLoadFailed(err)
			}

			proc, err := reg.FindProcessor(processorName)
			if err != nil {
				// Show available processors
				available := reg.ListProcessors("origin")
				var names []string
				for _, p := range available {
					names = append(names, p.Name)
				}
				return nebuErrors.ProcessorNotFound(processorName, names)
			}

			// Validate it's an origin processor
			if proc.Type != "origin" {
				return nebuErrors.ProcessorTypeMismatch(processorName, proc.Type, "origin")
			}

			// Origin processors must be installed and run as standalone binaries
			// The nebu CLI does not embed processor logic - it only provides runtime infrastructure
			return fmt.Errorf(`Processor '%s' is registered but must be installed and run as a standalone binary.

To use this processor:

  1. Install it:
     nebu install %s

  2. Run it directly:
     %s --start-ledger START --end-ledger END

  3. Or pipe from nebu fetch:
     nebu fetch START END | %s

  4. Or build manually:
     cd %s/cmd && go build -o %s

Processor info: %s
Location: %s

See 'nebu install --help' for more details.`,
				processorName,
				processorName,
				processorName,
				processorName,
				proc.Location.Path, processorName,
				proc.Description,
				proc.Location.Path)
		},
	}

	return cmd
}

func newRunSinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sink [processor-name]",
		Short: "Show how to run a sink processor",
		Long: `Sink processors run as standalone binaries, not embedded in nebu.
This command validates that a processor is registered and provides instructions
for installing and running it.

Sink processors consume events from stdin (typically piped from origin processors).

Workflow:
  1. List available processors:     nebu list
  2. Install processors:             nebu install <processor-name>
  3. Run origin → sink pipeline:    <origin-processor> | <sink-processor>

Examples:
  # See available processors
  nebu list

  # Install processors
  nebu install token-transfer
  nebu install duckdb-sink

  # Run pipeline: token-transfer → duckdb-sink
  token-transfer --start-ledger 60200000 --end-ledger 60200100 | duckdb-sink --db events.db

  # Or use nebu fetch
  nebu fetch 60200000 60200100 | token-transfer | duckdb-sink --db events.db

  # Build manually if needed
  cd examples/processors/duckdb-sink/cmd && go build -o duckdb-sink
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			processorName := args[0]

			// Load registry and validate processor exists
			reg, err := registry.LoadDefault()
			if err != nil {
				return nebuErrors.RegistryLoadFailed(err)
			}

			proc, err := reg.FindProcessor(processorName)
			if err != nil {
				// Show available processors
				available := reg.ListProcessors("sink")
				var names []string
				for _, p := range available {
					names = append(names, p.Name)
				}
				return nebuErrors.ProcessorNotFound(processorName, names)
			}

			// Validate it's a sink processor
			if proc.Type != "sink" {
				return nebuErrors.ProcessorTypeMismatch(processorName, proc.Type, "sink")
			}

			// Sink processors must be installed and run as standalone binaries
			// The nebu CLI does not embed processor logic - it only provides runtime infrastructure
			return fmt.Errorf(`Processor '%s' is registered but must be installed and run as a standalone binary.

To use this processor:

  1. Install it:
     nebu install %s

  2. Run it (consuming events from stdin):
     <origin-processor> | %s [flags]

  3. Or build manually:
     cd %s/cmd && go build -o %s

Processor info: %s
Location: %s

See 'nebu install --help' for more details.`,
				processorName,
				processorName,
				processorName,
				proc.Location.Path, processorName,
				proc.Description,
				proc.Location.Path)
		},
	}

	return cmd
}

