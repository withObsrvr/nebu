// Package errors provides actionable error messages for nebu.
// Errors include suggestions and examples to help users (and AI agents)
// quickly understand and fix issues.
package errors

import (
	"fmt"
	"strings"
)

// NebuError is an error with actionable guidance.
type NebuError struct {
	// Message is the main error description
	Message string
	// Suggestion is optional guidance on how to fix the error
	Suggestion string
	// Examples are optional example commands
	Examples []string
}

// Error implements the error interface.
func (e *NebuError) Error() string {
	var b strings.Builder
	b.WriteString(e.Message)

	if e.Suggestion != "" {
		b.WriteString("\n\n")
		b.WriteString(e.Suggestion)
	}

	if len(e.Examples) > 0 {
		b.WriteString("\n\nExamples:")
		for _, ex := range e.Examples {
			b.WriteString("\n  ")
			b.WriteString(ex)
		}
	}

	return b.String()
}

// New creates a simple NebuError with just a message.
func New(message string) *NebuError {
	return &NebuError{Message: message}
}

// WithSuggestion creates an error with a suggestion.
func WithSuggestion(message, suggestion string) *NebuError {
	return &NebuError{
		Message:    message,
		Suggestion: suggestion,
	}
}

// WithExamples creates an error with examples.
func WithExamples(message string, examples ...string) *NebuError {
	return &NebuError{
		Message:  message,
		Examples: examples,
	}
}

// Full creates an error with message, suggestion, and examples.
func Full(message, suggestion string, examples ...string) *NebuError {
	return &NebuError{
		Message:    message,
		Suggestion: suggestion,
		Examples:   examples,
	}
}

// --- Common Error Constructors ---

// InvalidLedgerRange returns an error for when start > end.
func InvalidLedgerRange(start, end uint32) *NebuError {
	return &NebuError{
		Message: fmt.Sprintf("Invalid ledger range: start (%d) must be less than or equal to end (%d)", start, end),
		Suggestion: fmt.Sprintf("Swap the values:\n  --start-ledger %d --end-ledger %d", end, start),
	}
}

// LedgerMustBePositive returns an error for ledger == 0.
func LedgerMustBePositive(which string) *NebuError {
	return &NebuError{
		Message:    fmt.Sprintf("%s ledger must be greater than 0", which),
		Suggestion: "Ledger sequences on Stellar start at 1.",
	}
}

// InvalidLedgerFormat returns an error for non-numeric ledger input.
func InvalidLedgerFormat(input, which string) *NebuError {
	return &NebuError{
		Message:    fmt.Sprintf("Invalid %s ledger: '%s' is not a valid number", which, input),
		Suggestion: "Ledger sequences are positive integers.",
		Examples: []string{
			"nebu fetch 60200000 60200100",
			"token-transfer --start-ledger 60200000 --end-ledger 60200100",
		},
	}
}

// RPCConnectionFailed returns an error for RPC connection issues.
func RPCConnectionFailed(url string, err error) *NebuError {
	return &NebuError{
		Message: fmt.Sprintf("Cannot connect to RPC endpoint: %s\n  %v", url, err),
		Suggestion: `Check that the URL is correct and reachable.

Common fixes:
  - Verify the URL: curl ` + url + `
  - Use a different RPC endpoint
  - Check your network connection`,
		Examples: []string{
			"nebu fetch 60200000 60200100  # uses default RPC",
			"nebu fetch --rpc-url https://your-rpc.example.com 60200000 60200100",
		},
	}
}

// RateLimited returns an error for HTTP 429 responses.
func RateLimited(url string) *NebuError {
	return &NebuError{
		Message: fmt.Sprintf("Rate limited by RPC endpoint (HTTP 429): %s", url),
		Suggestion: `The RPC endpoint has usage limits. You can:
  - Add authentication for higher limits
  - Use a smaller ledger range
  - Add delays between requests`,
		Examples: []string{
			"export NEBU_RPC_AUTH='Api-Key YOUR_KEY'",
			"nebu fetch --rpc-url https://premium-rpc.example.com ...",
		},
	}
}

// AuthRequired returns an error for HTTP 401 responses.
func AuthRequired(url string) *NebuError {
	return &NebuError{
		Message: fmt.Sprintf("Authentication required by RPC endpoint (HTTP 401): %s", url),
		Suggestion: `Set up authentication with one of these methods:

Environment variable:
  export NEBU_RPC_AUTH='Api-Key YOUR_KEY'
  export NEBU_RPC_AUTH='Bearer YOUR_TOKEN'

Or via flag:
  --rpc-header "Authorization: Bearer YOUR_TOKEN"`,
	}
}

// ProcessorNotFound returns an error when a processor doesn't exist.
func ProcessorNotFound(name string, available []string) *NebuError {
	suggestion := "Run 'nebu list' to see all available processors."
	if len(available) > 0 {
		suggestion = fmt.Sprintf("Available processors: %s\n\nRun 'nebu list' for details.", strings.Join(available, ", "))
	}
	return &NebuError{
		Message:    fmt.Sprintf("Processor '%s' not found in registry", name),
		Suggestion: suggestion,
	}
}

// ProcessorTypeMismatch returns an error when processor type doesn't match command.
func ProcessorTypeMismatch(name, gotType, wantType string) *NebuError {
	return &NebuError{
		Message: fmt.Sprintf("Processor '%s' is type '%s', but 'nebu run %s' requires type '%s'",
			name, gotType, wantType, wantType),
		Suggestion: fmt.Sprintf("Use the correct command for this processor type:\n  nebu run %s %s ...", gotType, name),
	}
}

// MissingRequiredFlags returns an error for missing required flags.
func MissingRequiredFlags(flags ...string) *NebuError {
	flagList := strings.Join(flags, ", ")
	return &NebuError{
		Message:    fmt.Sprintf("Missing required flags: %s", flagList),
		Suggestion: "These flags are required when not reading from stdin.",
		Examples: []string{
			"token-transfer --start-ledger 60200000 --end-ledger 60200100",
			"nebu fetch 60200000 60200100 | token-transfer  # reads from stdin instead",
		},
	}
}

// InvalidMode returns an error for unknown mode values.
func InvalidMode(mode string, validModes []string) *NebuError {
	return &NebuError{
		Message:    fmt.Sprintf("Invalid mode: '%s'", mode),
		Suggestion: fmt.Sprintf("Valid modes: %s", strings.Join(validModes, ", ")),
	}
}

// MissingBucketPath returns an error when bucket path is required but not set.
func MissingBucketPath() *NebuError {
	return &NebuError{
		Message:    "--bucket-path is required when using --mode archive",
		Suggestion: "Specify the storage location for ledger data.",
		Examples: []string{
			"nebu fetch --mode archive --bucket-path 'gs://bucket/ledgers' 60200000 60200100",
			"export NEBU_BUCKET_PATH='gs://bucket/ledgers'",
		},
	}
}

// FailedToCreateSource returns an error for source creation failures.
func FailedToCreateSource(sourceType string, err error) *NebuError {
	return &NebuError{
		Message: fmt.Sprintf("Failed to create %s source: %v", sourceType, err),
		Suggestion: fmt.Sprintf("Check your %s configuration and credentials.", sourceType),
	}
}

// FailedToOpenFile returns an error for file open failures.
func FailedToOpenFile(path string, err error) *NebuError {
	return &NebuError{
		Message:    fmt.Sprintf("Failed to open file '%s': %v", path, err),
		Suggestion: "Check that the file exists and you have read permission.",
	}
}

// FailedToCreateFile returns an error for file creation failures.
func FailedToCreateFile(path string, err error) *NebuError {
	return &NebuError{
		Message:    fmt.Sprintf("Failed to create file '%s': %v", path, err),
		Suggestion: "Check that the directory exists and you have write permission.",
	}
}

// XDRDecodeFailed returns an error for XDR decoding failures.
func XDRDecodeFailed(ledgerNum int, err error) *NebuError {
	suggestion := "The input may be corrupted or not in XDR format."
	if ledgerNum == 1 {
		suggestion = `The input doesn't appear to be valid XDR data.

If reading from a file, ensure it contains raw XDR (not JSON):
  nebu fetch 60200000 60200100 > ledgers.xdr  # creates XDR
  cat ledgers.xdr | token-transfer             # processes XDR

If you have JSON data, use the appropriate tool directly.`
	}
	return &NebuError{
		Message:    fmt.Sprintf("Failed to decode XDR at ledger %d: %v", ledgerNum, err),
		Suggestion: suggestion,
	}
}

// ProcessorError wraps a processor error with context.
func ProcessorError(ledgerSeq uint32, err error) *NebuError {
	return &NebuError{
		Message: fmt.Sprintf("Processor error at ledger %d: %v", ledgerSeq, err),
	}
}

// StreamError wraps a streaming error.
func StreamError(err error) *NebuError {
	return &NebuError{
		Message: fmt.Sprintf("Stream error: %v", err),
	}
}

// RegistryLoadFailed returns an error for registry loading failures.
func RegistryLoadFailed(err error) *NebuError {
	return &NebuError{
		Message: fmt.Sprintf("Failed to load processor registry: %v", err),
		Suggestion: `Make sure you're in the nebu project directory, or that nebu is properly installed.

If installed via 'go install', the registry should be embedded in the binary.
If running from source, ensure registry.yaml exists in the project root.`,
	}
}

// JSONParseFailed returns an error for JSON parsing failures.
func JSONParseFailed(line int, err error) *NebuError {
	return &NebuError{
		Message:    fmt.Sprintf("Failed to parse JSON at line %d: %v", line, err),
		Suggestion: "Input should be newline-delimited JSON (one JSON object per line).",
	}
}

// BuildFailed returns an error for processor build failures.
func BuildFailed(name string, err error) *NebuError {
	return &NebuError{
		Message: fmt.Sprintf("Failed to build processor '%s': %v", name, err),
		Suggestion: `Check that:
  - Go is installed and in your PATH
  - The processor source code is valid
  - All dependencies are available`,
	}
}

// ExternalRegistryFetchFailed returns an error for external registry fetch failures.
func ExternalRegistryFetchFailed(url string, err error) *NebuError {
	return &NebuError{
		Message:    fmt.Sprintf("Failed to fetch external registry (%s): %v", url, err),
		Suggestion: "Check your internet connection. The external registry will be skipped and only embedded processors will be available.",
	}
}

// InstallFailed returns an error for processor installation failures.
func InstallFailed(name string, err error) *NebuError {
	return &NebuError{
		Message: fmt.Sprintf("Failed to install processor '%s': %v", name, err),
		Suggestion: `Check that:
  - You have write permission to $GOPATH/bin
  - The module path is correct
  - You have network access for remote modules`,
	}
}
