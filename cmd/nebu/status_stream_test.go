package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
