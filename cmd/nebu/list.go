package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/registry"
)

func newListCmd() *cobra.Command {
	var procType string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available processors from the registry",
		Long: `List processors that are available to run via nebu.

Processors are defined in registry.yaml and can be local examples
or external packages.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load registry
			reg, err := registry.LoadDefault()
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}

			// Filter by type if specified
			processors := reg.ListProcessors(procType)

			if len(processors) == 0 {
				if procType != "" {
					fmt.Printf("No %s processors found in registry\n", procType)
				} else {
					fmt.Println("No processors found in registry")
				}
				return nil
			}

			// Print header
			if procType != "" {
				fmt.Printf("Available %s processors:\n\n", procType)
			} else {
				fmt.Println("Available processors:\n")
			}

			// Create table writer
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tLOCATION\tDESCRIPTION")
			fmt.Fprintln(w, "----\t----\t--------\t-----------")

			for _, proc := range processors {
				location := proc.Location.Type
				if proc.Location.Type == "local" {
					location = "local"
				} else if proc.Location.Type == "git" {
					location = "git"
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					proc.Name,
					proc.Type,
					location,
					truncate(proc.Description, 50))
			}

			w.Flush()

			fmt.Println("\nRun a processor:")
			fmt.Println("  nebu run origin <name> --start-ledger X --end-ledger Y")
			fmt.Println("\nView processor details:")
			fmt.Println("  cat $(find . -name manifest.yaml | grep <name>)")

			return nil
		},
	}

	cmd.Flags().StringVarP(&procType, "type", "t", "", "Filter by processor type (origin, transform, sink)")

	return cmd
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
