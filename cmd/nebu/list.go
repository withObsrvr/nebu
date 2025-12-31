package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/registry"
)

// Type descriptions for header output
var typeDescriptions = map[string]string{
	"origin":    "extract events from ledgers",
	"transform": "filter/modify event streams",
	"sink":      "store events",
}

// Type order for consistent output
var typeOrder = []string{"origin", "transform", "sink"}

func newListCmd() *cobra.Command {
	var (
		procType   string
		verbose    bool
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available processors from the registry",
		Long: `List processors that are available to run via nebu.

Processors are defined in registry.yaml and can be local examples
or external packages.

PROCESSOR TYPES:
  origin     Extract events from Stellar ledgers (token-transfer, contract-events, etc.)
  transform  Filter or modify event streams (usdc-filter, amount-filter, etc.)
  sink       Store events or produce side effects (json-file-sink, postgres-sink, etc.)

EXAMPLES:
  # List all processors (grouped by type)
  nebu list

  # List only origin processors
  nebu list --type origin

  # Verbose output with examples
  nebu list --verbose

  # JSON output for scripting
  nebu list --json

  # Get origin processor names for scripting
  nebu list --json | jq '.[] | select(.type == "origin") | .name'

TYPICAL WORKFLOW:
  1. List processors:      nebu list
  2. Get details:          nebu describe token-transfer
  3. Install processor:    nebu install token-transfer
  4. Run processor:        token-transfer --start-ledger 60200000 --end-ledger 60200100
  5. Or pipe from fetch:   nebu fetch 60200000 60200100 | token-transfer`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load registry
			reg, err := registry.LoadDefault()
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}

			// Get processors
			var processors []registry.ProcessorEntry
			if procType != "" {
				processors = reg.ListProcessors(procType)
			} else {
				processors = reg.Processors
			}

			if len(processors) == 0 {
				if procType != "" {
					fmt.Printf("No %s processors found in registry\n", procType)
				} else {
					fmt.Println("No processors found in registry")
				}
				return nil
			}

			// Output based on format
			if jsonOutput {
				return printJSONList(processors)
			}

			if verbose {
				return printVerboseList(processors, procType)
			}

			return printGroupedList(processors, procType)
		},
	}

	cmd.Flags().StringVarP(&procType, "type", "t", "", "Filter by processor type (origin, transform, sink)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed information including examples")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON for scripting")

	return cmd
}

// printGroupedList prints processors grouped by type
func printGroupedList(processors []registry.ProcessorEntry, filterType string) error {
	// Group by type
	byType := make(map[string][]registry.ProcessorEntry)
	for _, proc := range processors {
		byType[proc.Type] = append(byType[proc.Type], proc)
	}

	fmt.Println("Available processors:")
	fmt.Println()

	// Print each type in order
	for _, pType := range typeOrder {
		procs, ok := byType[pType]
		if !ok || len(procs) == 0 {
			continue
		}

		// Print type header
		desc := typeDescriptions[pType]
		fmt.Printf("%s PROCESSORS (%s)\n", strings.ToUpper(pType), desc)

		// Print processors with aligned descriptions
		maxNameLen := 0
		for _, proc := range procs {
			if len(proc.Name) > maxNameLen {
				maxNameLen = len(proc.Name)
			}
		}

		for _, proc := range procs {
			fmt.Printf("  %-*s  %s\n", maxNameLen, proc.Name, truncate(proc.Description, 60))
		}
		fmt.Println()
	}

	fmt.Println("Run 'nebu describe <processor>' for details and examples.")

	return nil
}

// printVerboseList prints detailed info for each processor
func printVerboseList(processors []registry.ProcessorEntry, filterType string) error {
	// Group by type
	byType := make(map[string][]registry.ProcessorEntry)
	for _, proc := range processors {
		byType[proc.Type] = append(byType[proc.Type], proc)
	}

	fmt.Println("Available processors:")
	fmt.Println()

	for _, pType := range typeOrder {
		procs, ok := byType[pType]
		if !ok || len(procs) == 0 {
			continue
		}

		// Print type header
		fmt.Printf("%s PROCESSORS\n", strings.ToUpper(pType))
		fmt.Println(strings.Repeat("-", len(pType)+11))
		fmt.Println()

		for _, proc := range procs {
			fmt.Printf("%s\n", proc.Name)

			// Type and schema
			fmt.Printf("  Type:    %s\n", proc.Type)
			if proc.Schema != nil && proc.Schema.Identifier != "" {
				fmt.Printf("  Schema:  %s\n", proc.Schema.Identifier)
			}

			// Events (for origin processors)
			if len(proc.Events) > 0 {
				eventNames := make([]string, len(proc.Events))
				for i, ev := range proc.Events {
					eventNames[i] = ev.Name
				}
				fmt.Printf("  Events:  %s\n", strings.Join(eventNames, ", "))
			}

			// First example
			if len(proc.Examples) > 0 {
				fmt.Printf("  Example: %s\n", proc.Examples[0].Command)
			} else {
				// Generate default example
				switch proc.Type {
				case "origin":
					fmt.Printf("  Example: %s --start-ledger 60200000 --end-ledger 60200100 | head -5\n", proc.Name)
				case "transform":
					fmt.Printf("  Example: token-transfer ... | %s\n", proc.Name)
				case "sink":
					fmt.Printf("  Example: token-transfer ... | %s\n", proc.Name)
				}
			}

			fmt.Println()
		}
	}

	return nil
}

// printJSONList outputs processors as JSON
func printJSONList(processors []registry.ProcessorEntry) error {
	type jsonProcessor struct {
		Name        string   `json:"name"`
		Type        string   `json:"type"`
		Description string   `json:"description"`
		Schema      string   `json:"schema,omitempty"`
		Events      []string `json:"events,omitempty"`
		Example     string   `json:"example,omitempty"`
	}

	output := make([]jsonProcessor, len(processors))
	for i, proc := range processors {
		jp := jsonProcessor{
			Name:        proc.Name,
			Type:        proc.Type,
			Description: proc.Description,
		}

		if proc.Schema != nil {
			jp.Schema = proc.Schema.Identifier
		}

		if len(proc.Events) > 0 {
			jp.Events = make([]string, len(proc.Events))
			for j, ev := range proc.Events {
				jp.Events[j] = ev.Name
			}
		}

		if len(proc.Examples) > 0 {
			jp.Example = proc.Examples[0].Command
		} else {
			// Generate default example
			switch proc.Type {
			case "origin":
				jp.Example = fmt.Sprintf("%s --start-ledger 60200000 --end-ledger 60200100", proc.Name)
			case "transform", "sink":
				jp.Example = fmt.Sprintf("token-transfer ... | %s", proc.Name)
			}
		}

		output[i] = jp
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// printTableList prints the classic table format (kept for reference)
func printTableList(processors []registry.ProcessorEntry, procType string) error {
	// Print header
	if procType != "" {
		fmt.Printf("Available %s processors:\n\n", procType)
	} else {
		fmt.Println("Available processors:")
		fmt.Println()
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
	fmt.Println("  nebu describe <processor>")

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
