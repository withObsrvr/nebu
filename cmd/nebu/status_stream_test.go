package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/withObsrvr/nebu/pkg/programstatus"
	"github.com/withObsrvr/nebu/pkg/status"
)

func TestStructuredStatusLifecycleWithoutTTY(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.ndjson")
	programStatusStarted = false
	statusFilePath = ""
	statusFileHandle = nil
	statusWriter = nil

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--program-status=never", "--status-file", path, "list"})
	require.Equal(t, 0, execute(cmd))

	records := readStatusRecords(t, path)
	require.Len(t, records, 2)
	assert.Equal(t, status.Schema, records[0].Schema)
	assert.Equal(t, programstatus.Working, records[0].State)
	assert.Equal(t, programstatus.Done, records[1].State)
}

func TestStatusFileRejectsOutputAliasBeforeTruncating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.WriteFile(path, []byte("keep"), 0o644))
	programStatusStarted = false
	statusFilePath = ""
	statusFileHandle = nil
	statusWriter = nil

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--program-status=never", "--status-file", path,
		"fetch", "--output", path, "1",
	})
	require.Equal(t, 1, execute(cmd))
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(contents))
}

func TestStatusFileRejectsRedirectedStdoutAlias(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redirected.xdr")
	redirected, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	require.NoError(t, err)
	originalStdout := os.Stdout
	os.Stdout = redirected
	defer func() {
		os.Stdout = originalStdout
		require.NoError(t, redirected.Close())
	}()

	statusFilePath = path
	statusFileHandle = nil
	statusWriter = nil
	err = openStatusStream(newFetchCmd())
	require.ErrorContains(t, err, "redirected stdout")
}

func TestStructuredStatusCancellationEndsIdle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.ndjson")
	file, err := os.Create(path)
	require.NoError(t, err)
	statusFileHandle = file
	statusWriter = status.NewWriter(file, "nebu")

	require.ErrorIs(t, finishStatusStream(context.Canceled), context.Canceled)
	records := readStatusRecords(t, path)
	require.Len(t, records, 1)
	assert.Equal(t, programstatus.Idle, records[0].State)
}

func TestStructuredFetchProgressIncludesLedgerLag(t *testing.T) {
	var output bytes.Buffer
	statusWriter = status.NewWriter(&output, "nebu")
	defer func() { statusWriter = nil }()

	count, err := writeRawLedgers(
		context.Background(),
		fakeRawLedgerStream{records: [][]byte{{1}, {2}}},
		ledgerbackend.BoundedRange(700, 701),
		&bytes.Buffer{},
		700,
		701,
	)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	var records []status.Record
	decoder := json.NewDecoder(&output)
	for decoder.More() {
		var record status.Record
		require.NoError(t, decoder.Decode(&record))
		records = append(records, record)
	}
	require.Len(t, records, 2)
	assert.Equal(t, uint32(700), *records[0].CurrentLedger)
	assert.Equal(t, uint32(701), *records[0].TargetLedger)
	assert.Equal(t, uint64(1), *records[0].LedgerLag)
	assert.Equal(t, uint64(0), *records[1].LedgerLag)
}

func readStatusRecords(t *testing.T, path string) []status.Record {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	var records []status.Record
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record status.Record
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))
		records = append(records, record)
	}
	require.NoError(t, scanner.Err())
	return records
}

// resetStatusStream restores the package-level status handles so these tests
// do not leak state into each other.
func resetStatusStream(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		statusFileHandle = nil
		statusWriter = nil
		statusCanceled = false
	})
	statusFileHandle = nil
	statusWriter = nil
	statusCanceled = false
}

func TestCancelStatusStreamWritesTheTerminalRecordBeforeAnyForcedExit(t *testing.T) {
	// The regression: a command that cannot stop promptly is force-exited
	// with os.Exit, which runs no deferred code, so the terminal record
	// written on the normal finish path never happened. The stream's last
	// word stayed "working" while the process exited 130, and anything
	// following it showed the run as still live.
	resetStatusStream(t)
	path := filepath.Join(t.TempDir(), "status.ndjson")
	file, err := os.Create(path)
	require.NoError(t, err)
	statusFileHandle = file
	statusWriter = status.NewWriter(file, "nebu")

	cancelStatusStream()

	records := readStatusRecords(t, path)
	require.Len(t, records, 1, "cancellation must be reported without waiting for the finish path")
	assert.Equal(t, programstatus.Idle, records[0].State)
}

func TestFinishStatusStreamDoesNotAddASecondTerminalRecord(t *testing.T) {
	// The stream's contract is one terminal record. When the command does
	// unwind after the signal, the finish path must not write another.
	resetStatusStream(t)
	path := filepath.Join(t.TempDir(), "status.ndjson")
	file, err := os.Create(path)
	require.NoError(t, err)
	statusFileHandle = file
	statusWriter = status.NewWriter(file, "nebu")

	cancelStatusStream()
	require.ErrorIs(t, finishStatusStream(context.Canceled), context.Canceled)

	records := readStatusRecords(t, path)
	require.Len(t, records, 1)
	assert.Equal(t, programstatus.Idle, records[0].State)
}

func TestCancelStatusStreamIsIdempotent(t *testing.T) {
	resetStatusStream(t)
	path := filepath.Join(t.TempDir(), "status.ndjson")
	file, err := os.Create(path)
	require.NoError(t, err)
	statusFileHandle = file
	statusWriter = status.NewWriter(file, "nebu")

	cancelStatusStream()
	cancelStatusStream()
	cancelStatusStream()

	assert.Len(t, readStatusRecords(t, path), 1)
}

func TestFinishStatusStreamStillReportsSuccessAndFailure(t *testing.T) {
	tests := []struct {
		name       string
		commandErr error
		wantState  programstatus.State
	}{
		{name: "success", wantState: programstatus.Done},
		{name: "failure", commandErr: errors.New("boom"), wantState: programstatus.Error},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetStatusStream(t)
			path := filepath.Join(t.TempDir(), "status.ndjson")
			file, err := os.Create(path)
			require.NoError(t, err)
			statusFileHandle = file
			statusWriter = status.NewWriter(file, "nebu")

			_ = finishStatusStream(tt.commandErr)

			records := readStatusRecords(t, path)
			require.Len(t, records, 1)
			assert.Equal(t, tt.wantState, records[0].State)
		})
	}
}

func TestStatusStreamWritesAreSafeUnderConcurrentCancellation(t *testing.T) {
	// A signal handler reports cancellation from its own goroutine while
	// the command is still emitting progress from the main one, so these
	// writes genuinely overlap. Run under -race.
	resetStatusStream(t)
	path := filepath.Join(t.TempDir(), "status.ndjson")
	file, err := os.Create(path)
	require.NoError(t, err)
	statusFileHandle = file
	statusWriter = status.NewWriter(file, "nebu")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 200 {
			_ = writeStructuredProgress(uint32(700+i), 900)
		}
	}()
	go func() {
		defer wg.Done()
		cancelStatusStream()
	}()
	wg.Wait()

	require.NoError(t, closeStatusStream())

	terminal := 0
	for _, record := range readStatusRecords(t, path) {
		if record.State == programstatus.Idle {
			terminal++
		}
	}
	assert.Equal(t, 1, terminal, "exactly one terminal record, however the writes interleave")
}
