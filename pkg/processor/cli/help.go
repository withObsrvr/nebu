// Package cli provides helpers for running processors as standalone CLI tools.
package cli

import (
	"fmt"
	"strings"
)

// HelpConfig provides rich help text configuration for processors.
type HelpConfig struct {
	// Name is the processor name (e.g., "token-transfer")
	Name string

	// Short is a one-line description
	Short string

	// Long is a detailed description (optional, auto-generated if empty)
	Long string

	// Version is the processor version
	Version string

	// OutputDescription describes what the processor outputs
	OutputDescription string

	// OutputExample is an example output JSON (optional)
	OutputExample string

	// SchemaID is the schema identifier (e.g., "nebu.token-transfer.v1")
	SchemaID string

	// Examples are usage examples with comments
	Examples []HelpExample

	// EnvVars are relevant environment variables
	EnvVars []HelpEnvVar

	// SeeAlso are related commands/processors
	SeeAlso []string

	// IsOrigin indicates this is an origin processor (shows INPUT MODES section)
	IsOrigin bool
}

// HelpExample is a usage example.
type HelpExample struct {
	Comment string
	Command string
}

// HelpEnvVar is an environment variable.
type HelpEnvVar struct {
	Name        string
	Description string
}

// BuildLongHelp generates the full help text from a HelpConfig.
func BuildLongHelp(config HelpConfig) string {
	var b strings.Builder

	// Description
	if config.Long != "" {
		b.WriteString(config.Long)
	} else {
		b.WriteString(config.Short)
	}
	b.WriteString("\n")

	// Input modes (only for origin processors)
	if config.IsOrigin {
		b.WriteString(`
INPUT MODES:
  RPC mode:   Fetch ledgers directly from Stellar RPC (default)
  stdin mode: Read XDR ledgers from stdin (piped from nebu fetch, including archive mode)
  File mode:  Read XDR ledgers from a file
`)
	}

	// Output description
	if config.OutputDescription != "" {
		b.WriteString("\nOUTPUT:\n")
		b.WriteString("  ")
		b.WriteString(config.OutputDescription)
		b.WriteString("\n")

		if config.SchemaID != "" {
			b.WriteString(fmt.Sprintf("  Schema: %s\n", config.SchemaID))
		}

		if config.OutputExample != "" {
			b.WriteString("\n  Example:\n")
			for _, line := range strings.Split(config.OutputExample, "\n") {
				b.WriteString("    ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
	}

	// Examples
	if len(config.Examples) > 0 {
		b.WriteString("\nEXAMPLES:\n")
		for _, ex := range config.Examples {
			if ex.Comment != "" {
				b.WriteString(fmt.Sprintf("  # %s\n", ex.Comment))
			}
			b.WriteString(fmt.Sprintf("  %s\n\n", ex.Command))
		}
	}

	// Environment variables
	if len(config.EnvVars) > 0 {
		b.WriteString("ENVIRONMENT:\n")
		for _, env := range config.EnvVars {
			b.WriteString(fmt.Sprintf("  %-20s %s\n", env.Name, env.Description))
		}
		b.WriteString("\n")
	}

	// See also
	if len(config.SeeAlso) > 0 {
		b.WriteString("SEE ALSO:\n")
		b.WriteString("  ")
		b.WriteString(strings.Join(config.SeeAlso, ", "))
		b.WriteString("\n")
	}

	return b.String()
}

// DefaultOriginExamples returns common examples for origin processors.
func DefaultOriginExamples(name string) []HelpExample {
	return []HelpExample{
		{
			Comment: "Process a range of ledgers",
			Command: fmt.Sprintf("%s --start-ledger 60200000 --end-ledger 60200100", name),
		},
		{
			Comment: "Stream continuously (like tail -f)",
			Command: fmt.Sprintf("%s --start-ledger 60200000 --follow", name),
		},
		{
			Comment: "Read from stdin (piped from nebu fetch)",
			Command: fmt.Sprintf("nebu fetch 60200000 60200100 | %s", name),
		},
		{
			Comment: "Backfill from the public AWS S3 archive via nebu fetch",
			Command: fmt.Sprintf("nebu fetch --mode archive --datastore-type S3 --bucket-path \"aws-public-blockchain/v1.1/stellar/ledgers/pubnet\" --region us-east-2 62080000 62080100 | %s", name),
		},
		{
			Comment: "Filter output with jq",
			Command: fmt.Sprintf("%s --start-ledger 60200000 --end-ledger 60200010 | jq 'select(.type == \"transfer\")'", name),
		},
		{
			Comment: "Limit output for exploration",
			Command: fmt.Sprintf("%s --start-ledger 60200000 --end-ledger 60200010 | head -5", name),
		},
		{
			Comment: "Count events",
			Command: fmt.Sprintf("%s --start-ledger 60200000 --end-ledger 60200010 -q | wc -l", name),
		},
	}
}

// DefaultTransformExamples returns common examples for transform processors.
func DefaultTransformExamples(originName, name string) []HelpExample {
	return []HelpExample{
		{
			Comment: "Basic filtering",
			Command: fmt.Sprintf("%s --start-ledger 60200000 --end-ledger 60200100 | %s", originName, name),
		},
		{
			Comment: "Chain with other transforms",
			Command: fmt.Sprintf("%s ... | %s | other-filter", originName, name),
		},
		{
			Comment: "Save filtered results",
			Command: fmt.Sprintf("%s ... | %s | json-file-sink --out filtered.jsonl", originName, name),
		},
	}
}

// DefaultSinkExamples returns common examples for sink processors.
func DefaultSinkExamples(originName, name string) []HelpExample {
	return []HelpExample{
		{
			Comment: "Basic usage",
			Command: fmt.Sprintf("%s --start-ledger 60200000 --end-ledger 60200100 | %s", originName, name),
		},
		{
			Comment: "With filtering",
			Command: fmt.Sprintf("%s ... | jq 'select(.amount > \"1000000\")' | %s", originName, name),
		},
	}
}

// CommonEnvVars returns environment variables common to all processors.
func CommonEnvVars() []HelpEnvVar {
	return []HelpEnvVar{
		{Name: "NEBU_RPC_URL", Description: "Default RPC endpoint"},
		{Name: "NEBU_RPC_AUTH", Description: "Authorization header (e.g., 'Api-Key xxx')"},
		{Name: "NEBU_NETWORK", Description: "Network: 'mainnet' or 'testnet'"},
	}
}

// buildOriginLongHelp builds the long help text for an origin processor.
func buildOriginLongHelp(config OriginConfig) string {
	// If custom help is provided, use it
	if config.Help != nil {
		return BuildLongHelp(*config.Help)
	}

	// Otherwise, build default help
	helpConfig := HelpConfig{
		Name:     config.Name,
		Short:    config.Description,
		Version:  config.Version,
		IsOrigin: true,
		OutputDescription: "Newline-delimited JSON (JSONL) to stdout. Each line is one event.",
		Examples: DefaultOriginExamples(config.Name),
		EnvVars:  CommonEnvVars(),
		SeeAlso:  []string{"nebu fetch", "nebu list", "jq"},
	}

	return BuildLongHelp(helpConfig)
}

// buildTransformLongHelp builds the long help text for a transform processor.
func buildTransformLongHelp(config TransformConfig) string {
	// If custom help is provided, use it
	if config.Help != nil {
		return BuildLongHelp(*config.Help)
	}

	// Otherwise, build default help
	helpConfig := HelpConfig{
		Name:    config.Name,
		Short:   config.Description,
		Version: config.Version,
		Long: fmt.Sprintf(`%s

Transform processors read events from stdin and output filtered/modified events to stdout.
They are designed to be chained in Unix pipelines.`, config.Description),
		OutputDescription: "Filtered/modified events as newline-delimited JSON.",
		Examples: DefaultTransformExamples("token-transfer", config.Name),
		SeeAlso:  []string{"token-transfer", "json-file-sink"},
	}

	return BuildLongHelp(helpConfig)
}

// buildSinkLongHelp builds the long help text for a sink processor.
func buildSinkLongHelp(config SinkConfig) string {
	// If custom help is provided, use it
	if config.Help != nil {
		return BuildLongHelp(*config.Help)
	}

	// Otherwise, build default help
	helpConfig := HelpConfig{
		Name:    config.Name,
		Short:   config.Description,
		Version: config.Version,
		Long: fmt.Sprintf(`%s

Sink processors consume events from stdin and produce side effects
(write to files, databases, message queues, etc.).`, config.Description),
		Examples: DefaultSinkExamples("token-transfer", config.Name),
		SeeAlso:  []string{"token-transfer", "nebu fetch"},
	}

	return BuildLongHelp(helpConfig)
}
