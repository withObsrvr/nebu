package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/registry"
)

// Types for JSON output
type describeExample struct {
	Comment string `json:"comment,omitempty"`
	Command string `json:"command"`
}

type describeWorksWith struct {
	Input      []string `json:"input,omitempty"`
	Transforms []string `json:"transforms,omitempty"`
	Sinks      []string `json:"sinks,omitempty"`
}

func newDescribeCmd() *cobra.Command {
	var (
		jsonOutput   bool
		schemaOutput bool
	)

	cmd := &cobra.Command{
		Use:   "describe <processor>",
		Short: "Show detailed information about a processor",
		Long: `Show detailed information about a processor including description,
output schema, example commands, and compatible processors.

When the processor binary is installed, 'nebu describe' executes
'<processor> --describe-json' to fetch its live schema and flag set.
Otherwise it falls back to registry metadata.

EXAMPLES:
  # Get details about token-transfer
  nebu describe token-transfer

  # Output the full envelope as JSON for scripting
  nebu describe token-transfer --json

  # Extract just the JSON Schema for validation
  nebu describe token-transfer --schema > token-transfer.schema.json

SEE ALSO:
  nebu list     List all available processors
  nebu install  Install a processor`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			// Load registry (embedded + external)
			reg, err := registry.LoadAll()
			if err != nil {
				return errors.RegistryLoadFailed(err)
			}

			// Find processor in the registry.
			proc, err := reg.FindProcessor(name)
			if err != nil {
				return errors.ProcessorNotFound(name, reg.ListProcessorNames())
			}

			// Try to fetch a live envelope from the installed binary.
			// nil is fine — the fallback paths handle a missing envelope.
			env := tryFetchEnvelope(name)

			if schemaOutput {
				return printSchemaOnly(name, env)
			}
			if jsonOutput {
				return printJSONDescribe(proc, env)
			}
			return printDescribe(proc, env)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output the full describe envelope as JSON for scripting")
	cmd.Flags().BoolVar(&schemaOutput, "schema", false, "Output only the JSON Schema of the processor's events (requires binary installed)")

	return cmd
}

// printSchemaOnly extracts and prints the JSON Schema from a describe
// envelope. Used by 'nebu describe <name> --schema' to feed schemas
// into validators or other tools.
func printSchemaOnly(name string, env *processor.DescribeEnvelope) error {
	if env == nil {
		return fmt.Errorf("cannot emit schema for %q: processor binary is not installed (use 'nebu install %s')", name, name)
	}
	// Prefer output schema (present for origins and most transforms).
	// Fall back to input schema (transforms/sinks that declared input).
	var schema map[string]any
	switch {
	case env.Schema.Output != nil:
		schema = env.Schema.Output
	case env.Schema.Input != nil:
		schema = env.Schema.Input
	default:
		return fmt.Errorf("processor %q did not declare a schema in its describe envelope", name)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(schema)
}

// printDescribe prints detailed processor information. When env is
// non-nil (the binary is installed and responded to --describe-json),
// its richer data is preferred over the registry metadata.
func printDescribe(proc *registry.ProcessorEntry, env *processor.DescribeEnvelope) error {
	// Header
	fmt.Println()
	fmt.Println(proc.Name)
	fmt.Println(strings.Repeat("=", len(proc.Name)))
	fmt.Printf("Type: %s\n", proc.Type)
	if proc.Source == "community" {
		fmt.Printf("Source: community (external registry)\n")
	}
	if env != nil && env.Version != "" {
		fmt.Printf("Version: %s\n", env.Version)
	}
	// Schema ID: prefer the live envelope over registry metadata since
	// the binary is the source of truth for its own schema.
	switch {
	case env != nil && env.Schema.ID != "":
		fmt.Printf("Schema: %s\n", env.Schema.ID)
	case proc.Schema != nil && proc.Schema.Identifier != "":
		fmt.Printf("Schema: %s\n", proc.Schema.Identifier)
	}
	fmt.Println()

	// Description
	fmt.Println("DESCRIPTION")
	if proc.LongDescription != "" {
		for _, line := range strings.Split(strings.TrimSpace(proc.LongDescription), "\n") {
			fmt.Printf("  %s\n", line)
		}
	} else {
		fmt.Printf("  %s\n", proc.Description)
	}
	fmt.Println()

	// Events (for origin processors)
	if len(proc.Events) > 0 {
		fmt.Println("OUTPUT EVENTS")
		maxNameLen := 0
		for _, ev := range proc.Events {
			if len(ev.Name) > maxNameLen {
				maxNameLen = len(ev.Name)
			}
		}
		for _, ev := range proc.Events {
			fmt.Printf("  %-*s  %s\n", maxNameLen, ev.Name, ev.Description)
		}
		fmt.Println()
	}

	// Output fields
	if len(proc.OutputFields) > 0 {
		fmt.Println("OUTPUT FIELDS")
		maxPathLen := 0
		for _, f := range proc.OutputFields {
			if len(f.Path) > maxPathLen {
				maxPathLen = len(f.Path)
			}
		}
		for _, f := range proc.OutputFields {
			fmt.Printf("  %-*s  %s\n", maxPathLen, f.Path, f.Description)
		}
		fmt.Println()
	}

	// Flags (only available when the envelope was fetched live).
	if env != nil && len(env.Flags) > 0 {
		fmt.Println("FLAGS")
		maxNameLen := 0
		for _, f := range env.Flags {
			if len(f.Name) > maxNameLen {
				maxNameLen = len(f.Name)
			}
		}
		for _, f := range env.Flags {
			marker := " "
			if f.Required {
				marker = "*"
			}
			line := fmt.Sprintf("  %s --%-*s  %s", marker, maxNameLen, f.Name, f.Description)
			if f.Default != "" && f.Default != "false" && f.Default != "0" && f.Default != "[]" {
				line += fmt.Sprintf(" (default %q)", f.Default)
			}
			fmt.Println(line)
		}
		fmt.Println("  * = required")
		fmt.Println()
	}

	// Examples
	fmt.Println("COMMON PIPELINES")
	if len(proc.Examples) > 0 {
		for _, ex := range proc.Examples {
			if ex.Comment != "" {
				fmt.Printf("  # %s\n", ex.Comment)
			}
			fmt.Printf("  %s\n", ex.Command)
			fmt.Println()
		}
	} else {
		// Generate default examples based on processor type
		printDefaultExamples(proc)
	}

	// Works with
	if proc.WorksWith != nil {
		fmt.Println("WORKS WITH")
		if len(proc.WorksWith.Input) > 0 {
			fmt.Printf("  Input:      %s\n", strings.Join(proc.WorksWith.Input, ", "))
		}
		if len(proc.WorksWith.Transforms) > 0 {
			fmt.Printf("  Transforms: %s\n", strings.Join(proc.WorksWith.Transforms, ", "))
		}
		if len(proc.WorksWith.Sinks) > 0 {
			fmt.Printf("  Sinks:      %s\n", strings.Join(proc.WorksWith.Sinks, ", "))
		}
		fmt.Println()
	} else {
		// Print default compatibility
		printDefaultWorksWith(proc)
	}

	// Maintainer
	if proc.Maintainer != nil {
		fmt.Println("MAINTAINER")
		fmt.Printf("  %s", proc.Maintainer.Name)
		if proc.Maintainer.URL != "" {
			fmt.Printf(" (%s)", proc.Maintainer.URL)
		}
		fmt.Println()
		fmt.Println()
	}

	return nil
}

// printDefaultExamples generates default examples based on processor type
func printDefaultExamples(proc *registry.ProcessorEntry) {
	switch proc.Type {
	case "origin":
		fmt.Printf("  # Basic usage\n")
		fmt.Printf("  %s --start-ledger 60200000 --end-ledger 60200100\n\n", proc.Name)

		fmt.Printf("  # Pipe from nebu fetch\n")
		fmt.Printf("  nebu fetch 60200000 60200100 | %s\n\n", proc.Name)

		fmt.Printf("  # Filter output with jq\n")
		fmt.Printf("  %s --start-ledger 60200000 --end-ledger 60200100 | jq '.meta'\n\n", proc.Name)

		fmt.Printf("  # Save to file\n")
		fmt.Printf("  %s --start-ledger 60200000 --end-ledger 60200100 | json-file-sink --out output.jsonl\n\n", proc.Name)

	case "transform":
		fmt.Printf("  # Basic filtering\n")
		fmt.Printf("  token-transfer --start-ledger 60200000 --end-ledger 60200100 | %s\n\n", proc.Name)

		fmt.Printf("  # Chain with other transforms\n")
		fmt.Printf("  token-transfer ... | %s | dedup --key meta.txHash\n\n", proc.Name)

		fmt.Printf("  # Save filtered results\n")
		fmt.Printf("  token-transfer ... | %s | json-file-sink --out filtered.jsonl\n\n", proc.Name)

	case "sink":
		fmt.Printf("  # Basic usage\n")
		fmt.Printf("  token-transfer --start-ledger 60200000 --end-ledger 60200100 | %s\n\n", proc.Name)

		fmt.Printf("  # With transform pipeline\n")
		fmt.Printf("  token-transfer ... | usdc-filter | amount-filter --min 1000000 | %s\n\n", proc.Name)
	}
}

// printDefaultWorksWith generates default compatibility info
func printDefaultWorksWith(proc *registry.ProcessorEntry) {
	fmt.Println("WORKS WITH")
	switch proc.Type {
	case "origin":
		fmt.Println("  Input:      nebu fetch (XDR stream) or --start-ledger/--end-ledger")
		fmt.Println("  Transforms: usdc-filter, amount-filter, dedup, time-window")
		fmt.Println("  Sinks:      json-file-sink, nats-sink, postgres-sink")
	case "transform":
		fmt.Println("  Input:      Any origin processor (token-transfer, contract-events, etc.)")
		fmt.Println("  Chain:      Other transform processors")
		fmt.Println("  Output:     json-file-sink, nats-sink, postgres-sink")
	case "sink":
		fmt.Println("  Input:      Any origin or transform processor")
	}
	fmt.Println()
}

// printJSONDescribe outputs processor details as JSON. When env is
// non-nil, the envelope's fields (name, type, version, description,
// schema, flags, examples) take precedence over registry metadata.
// Registry-only fields (long_description, events, output_fields,
// works_with, maintainer) are always included when present.
func printJSONDescribe(proc *registry.ProcessorEntry, env *processor.DescribeEnvelope) error {
	type jsonEvent struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	type jsonField struct {
		Path        string `json:"path"`
		Description string `json:"description"`
	}

	type jsonDescribe struct {
		Name            string                    `json:"name"`
		Type            string                    `json:"type"`
		Version         string                    `json:"version,omitempty"`
		Description     string                    `json:"description"`
		LongDescription string                    `json:"long_description,omitempty"`
		Schema          *processor.DescribeSchema `json:"schema,omitempty"`
		Flags           []processor.DescribeFlag  `json:"flags,omitempty"`
		Events          []jsonEvent               `json:"events,omitempty"`
		OutputFields    []jsonField               `json:"output_fields,omitempty"`
		Examples        []describeExample         `json:"examples,omitempty"`
		WorksWith       *describeWorksWith        `json:"works_with,omitempty"`
		Maintainer      string                    `json:"maintainer,omitempty"`
	}

	output := jsonDescribe{
		Name:            proc.Name,
		Type:            proc.Type,
		Description:     proc.Description,
		LongDescription: proc.LongDescription,
	}

	// Envelope takes precedence for fields it provides.
	if env != nil {
		if env.Name != "" {
			output.Name = env.Name
		}
		if env.Type != "" {
			output.Type = env.Type
		}
		if env.Version != "" {
			output.Version = env.Version
		}
		if env.Description != "" {
			output.Description = env.Description
		}
		// Always use the full envelope schema when available — it
		// carries the JSON Schema that registry metadata cannot.
		schemaCopy := env.Schema
		output.Schema = &schemaCopy
		output.Flags = env.Flags
	}

	if output.Schema == nil && proc.Schema != nil && proc.Schema.Identifier != "" {
		// Fall back to registry-only schema ID when no envelope.
		output.Schema = &processor.DescribeSchema{ID: proc.Schema.Identifier}
	}

	if len(proc.Events) > 0 {
		output.Events = make([]jsonEvent, len(proc.Events))
		for i, ev := range proc.Events {
			output.Events[i] = jsonEvent{
				Name:        ev.Name,
				Description: ev.Description,
			}
		}
	}

	if len(proc.OutputFields) > 0 {
		output.OutputFields = make([]jsonField, len(proc.OutputFields))
		for i, f := range proc.OutputFields {
			output.OutputFields[i] = jsonField{
				Path:        f.Path,
				Description: f.Description,
			}
		}
	}

	// Examples priority: envelope > registry > defaults.
	switch {
	case env != nil && len(env.Examples) > 0:
		output.Examples = make([]describeExample, len(env.Examples))
		for i, ex := range env.Examples {
			output.Examples[i] = describeExample{
				Comment: ex.Comment,
				Command: ex.Command,
			}
		}
	case len(proc.Examples) > 0:
		output.Examples = make([]describeExample, len(proc.Examples))
		for i, ex := range proc.Examples {
			output.Examples[i] = describeExample{
				Comment: ex.Comment,
				Command: ex.Command,
			}
		}
	default:
		output.Examples = generateDefaultExamples(proc)
	}

	if proc.WorksWith != nil {
		output.WorksWith = &describeWorksWith{
			Input:      proc.WorksWith.Input,
			Transforms: proc.WorksWith.Transforms,
			Sinks:      proc.WorksWith.Sinks,
		}
	} else {
		// Generate default works_with
		output.WorksWith = generateDefaultWorksWithStruct(proc)
	}

	if proc.Maintainer != nil {
		if proc.Maintainer.URL != "" {
			output.Maintainer = fmt.Sprintf("%s (%s)", proc.Maintainer.Name, proc.Maintainer.URL)
		} else {
			output.Maintainer = proc.Maintainer.Name
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func generateDefaultExamples(proc *registry.ProcessorEntry) []describeExample {
	var examples []describeExample

	switch proc.Type {
	case "origin":
		examples = []describeExample{
			{Comment: "Basic usage", Command: fmt.Sprintf("%s --start-ledger 60200000 --end-ledger 60200100", proc.Name)},
			{Comment: "Pipe from nebu fetch", Command: fmt.Sprintf("nebu fetch 60200000 60200100 | %s", proc.Name)},
			{Comment: "Save to file", Command: fmt.Sprintf("%s --start-ledger 60200000 --end-ledger 60200100 | json-file-sink --out output.jsonl", proc.Name)},
		}
	case "transform":
		examples = []describeExample{
			{Comment: "Basic filtering", Command: fmt.Sprintf("token-transfer ... | %s", proc.Name)},
			{Comment: "Chain with other transforms", Command: fmt.Sprintf("token-transfer ... | %s | dedup --key meta.txHash", proc.Name)},
		}
	case "sink":
		examples = []describeExample{
			{Comment: "Basic usage", Command: fmt.Sprintf("token-transfer --start-ledger 60200000 --end-ledger 60200100 | %s", proc.Name)},
			{Comment: "With transform pipeline", Command: fmt.Sprintf("token-transfer ... | usdc-filter | %s", proc.Name)},
		}
	}

	return examples
}

func generateDefaultWorksWithStruct(proc *registry.ProcessorEntry) *describeWorksWith {
	result := &describeWorksWith{}

	switch proc.Type {
	case "origin":
		result.Input = []string{"nebu fetch"}
		result.Transforms = []string{"usdc-filter", "amount-filter", "dedup", "time-window"}
		result.Sinks = []string{"json-file-sink", "nats-sink", "postgres-sink"}
	case "transform":
		result.Input = []string{"token-transfer", "contract-events", "contract-invocation"}
		result.Transforms = []string{"usdc-filter", "amount-filter", "dedup", "time-window"}
		result.Sinks = []string{"json-file-sink", "nats-sink", "postgres-sink"}
	case "sink":
		result.Input = []string{"token-transfer", "contract-events", "usdc-filter", "amount-filter"}
	}

	return result
}
