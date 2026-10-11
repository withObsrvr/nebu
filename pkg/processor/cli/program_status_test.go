package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/withObsrvr/nebu/pkg/programstatus"
	statusstream "github.com/withObsrvr/nebu/pkg/status"
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

			cmd.SetArgs([]string{})
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

	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.ErrorContains(t, err, "expected auto, always, or never")
	assert.False(t, called)
}

func TestCommandStatusCancellationDoesNotReportDone(t *testing.T) {
	quiet := false
	var status *commandStatus
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error {
		status.cancel()
		return nil
	}}
	status = attachProgramStatus(cmd, "test-app", &quiet)
	status.mode = "always"
	var output bytes.Buffer
	status.writer = &output

	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, output.String(), "state=working")
	assert.Contains(t, output.String(), "state=idle")
	assert.NotContains(t, output.String(), "state=done")
}

func TestTerminalFileRejectsNonTerminalCharacterDevice(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, devNull.Close()) })

	assert.False(t, terminalFile(devNull))
}

func TestCommandStructuredStatusLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.ndjson")
	quiet := false
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	commandStatus := attachProgramStatus(cmd, "test-app", &quiet)
	commandStatus.mode = "never"
	commandStatus.statusPath = path

	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())
	records := readProcessorStatusRecords(t, path)
	require.Len(t, records, 2)
	assert.Equal(t, programstatus.Working, records[0].State)
	assert.Equal(t, programstatus.Done, records[1].State)
}

func TestCommandStructuredStatusCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.ndjson")
	quiet := false
	var commandStatus *commandStatus
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error {
		commandStatus.cancel()
		return errors.New("should remain the command error")
	}}
	commandStatus = attachProgramStatus(cmd, "test-app", &quiet)
	commandStatus.mode = "never"
	commandStatus.statusPath = path

	cmd.SetArgs([]string{})
	require.Error(t, cmd.Execute())
	records := readProcessorStatusRecords(t, path)
	require.Len(t, records, 2)
	assert.Equal(t, programstatus.Working, records[0].State)
	assert.Equal(t, programstatus.Idle, records[1].State)
}

func TestCommandStructuredProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.ndjson")
	file, err := os.Create(path)
	require.NoError(t, err)
	commandStatus := &commandStatus{structured: statusstream.NewWriter(file, "test-app"), statusFile: file}

	require.NoError(t, commandStatus.ledgerProgress(100, 110))
	require.NoError(t, commandStatus.eventProgress(7))
	require.NoError(t, commandStatus.closeStructured())
	records := readProcessorStatusRecords(t, path)
	require.Len(t, records, 2)
	assert.Equal(t, uint32(100), *records[0].CurrentLedger)
	assert.Equal(t, uint64(10), *records[0].LedgerLag)
	assert.Equal(t, uint64(7), *records[1].EventsProcessed)
}

func readProcessorStatusRecords(t *testing.T, path string) []statusstream.Record {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	var records []statusstream.Record
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record statusstream.Record
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))
		records = append(records, record)
	}
	require.NoError(t, scanner.Err())
	return records
}
