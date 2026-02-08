package errors

import (
	"errors"
	"strings"
	"testing"
)

func TestNebuError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *NebuError
		contains []string
	}{
		{
			name:     "simple message",
			err:      New("something went wrong"),
			contains: []string{"something went wrong"},
		},
		{
			name:     "with suggestion",
			err:      WithSuggestion("invalid input", "try a different value"),
			contains: []string{"invalid input", "try a different value"},
		},
		{
			name:     "with examples",
			err:      WithExamples("missing flag", "command --flag value", "command --flag other"),
			contains: []string{"missing flag", "Examples:", "command --flag value", "command --flag other"},
		},
		{
			name:     "full error",
			err:      Full("error message", "helpful suggestion", "example1", "example2"),
			contains: []string{"error message", "helpful suggestion", "Examples:", "example1", "example2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Error() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestInvalidLedgerRange(t *testing.T) {
	err := InvalidLedgerRange(100, 50)
	got := err.Error()

	if !strings.Contains(got, "100") {
		t.Errorf("Error should contain start ledger 100")
	}
	if !strings.Contains(got, "50") {
		t.Errorf("Error should contain end ledger 50")
	}
	if !strings.Contains(got, "Swap") {
		t.Errorf("Error should suggest swapping values")
	}
}

func TestProcessorNotFound(t *testing.T) {
	err := ProcessorNotFound("my-processor", []string{"proc-a", "proc-b"})
	got := err.Error()

	if !strings.Contains(got, "my-processor") {
		t.Errorf("Error should contain processor name")
	}
	if !strings.Contains(got, "proc-a") {
		t.Errorf("Error should list available processors")
	}
}

func TestRPCConnectionFailed(t *testing.T) {
	originalErr := errors.New("connection refused")
	err := RPCConnectionFailed("https://example.com", originalErr)
	got := err.Error()

	if !strings.Contains(got, "https://example.com") {
		t.Errorf("Error should contain URL")
	}
	if !strings.Contains(got, "connection refused") {
		t.Errorf("Error should contain original error")
	}
	if !strings.Contains(got, "curl") {
		t.Errorf("Error should suggest using curl to verify")
	}
}

func TestXDRDecodeFailed(t *testing.T) {
	// First ledger should have special message
	err := XDRDecodeFailed(1, errors.New("unexpected format"))
	got := err.Error()

	if !strings.Contains(got, "ledger 1") {
		t.Errorf("Error should mention ledger number")
	}
	if !strings.Contains(got, "XDR") {
		t.Errorf("Error should mention XDR format")
	}
}
