package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandStatusLifecycle(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		quiet      bool
		terminal   bool
		runErr     error
		wantOutput bool
		wantState  string
	}{
		{name: "auto terminal success", mode: "auto", terminal: true, wantOutput: true, wantState: "state=done"},
		{name: "auto pipe", mode: "auto"},
		{name: "quiet auto", mode: "auto", quiet: true, terminal: true},
		{name: "always overrides quiet", mode: "always", quiet: true, wantOutput: true, wantState: "state=done"},
		{name: "error", mode: "always", runErr: errors.New("failed"), wantOutput: true, wantState: "state=error"},
		{name: "never", mode: "never", terminal: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quiet := tt.quiet
			cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return tt.runErr }}
			status := attachProgramStatus(cmd, "test-app", &quiet)
			status.mode = tt.mode
			status.isTerminal = func() bool { return tt.terminal }
			var output bytes.Buffer
			status.writer = &output

			err := cmd.Execute()
			if tt.runErr != nil {
				require.ErrorIs(t, err, tt.runErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOutput, output.Len() > 0)
			if tt.wantState != "" {
				assert.Contains(t, output.String(), "state=working")
				assert.Contains(t, output.String(), tt.wantState)
			}
		})
	}
}

func TestCommandStatusRejectsInvalidMode(t *testing.T) {
	quiet := false
	called := false
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error {
		called = true
		return nil
	}}
	status := attachProgramStatus(cmd, "test-app", &quiet)
	status.mode = "sometimes"

	err := cmd.Execute()
	require.ErrorContains(t, err, "expected auto, always, or never")
	assert.False(t, called)
}
