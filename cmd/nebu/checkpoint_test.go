package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckpointResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fetch.json")
	w, start, complete, err := openCheckpoint(path, 100, 105, 2)
	require.NoError(t, err)
	assert.Equal(t, uint32(100), start)
	assert.False(t, complete)

	require.NoError(t, w.advance(100, false))
	_, err = os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, w.advance(101, false))

	w, start, complete, err = openCheckpoint(path, 100, 105, 2)
	require.NoError(t, err)
	assert.True(t, w.resuming())
	assert.Equal(t, uint32(102), start)
	assert.False(t, complete)

	require.NoError(t, w.advance(105, true))
	_, start, complete, err = openCheckpoint(path, 100, 105, 2)
	require.NoError(t, err)
	assert.Equal(t, uint32(105), start)
	assert.True(t, complete)
}

func TestCheckpointRejectsDifferentRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fetch.json")
	w, _, _, err := openCheckpoint(path, 100, 105, 1)
	require.NoError(t, err)
	require.NoError(t, w.advance(100, true))

	_, _, _, err = openCheckpoint(path, 100, 106, 1)
	require.ErrorContains(t, err, "belongs to range 100-105")
}

func TestInterruptedWriteFlushesCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fetch.json")
	w, _, _, err := openCheckpoint(path, 200, 203, 100)
	require.NoError(t, err)

	count, err := writeRawLedgersWithCheckpoint(
		context.Background(),
		fakeRawLedgerStream{records: [][]byte{{1}, {2}}, err: context.Canceled},
		ledgerbackend.BoundedRange(200, 203),
		io.Discard,
		200,
		203,
		w,
	)
	assert.Equal(t, 2, count)
	assert.ErrorContains(t, err, context.Canceled.Error())

	_, start, complete, err := openCheckpoint(path, 200, 203, 100)
	require.NoError(t, err)
	assert.Equal(t, uint32(202), start)
	assert.False(t, complete)
}

func TestFetchRawLedgersAppendsWhenResuming(t *testing.T) {
	dir := t.TempDir()
	checkpointPath := filepath.Join(dir, "fetch.json")
	outputPath := filepath.Join(dir, "ledgers.xdr")
	require.NoError(t, os.WriteFile(outputPath, []byte{1, 2}, 0o644))

	w, _, _, err := openCheckpoint(checkpointPath, 300, 303, 1)
	require.NoError(t, err)
	require.NoError(t, w.advance(301, true))
	w, start, _, err := openCheckpoint(checkpointPath, 300, 303, 1)
	require.NoError(t, err)

	err = fetchRawLedgers(
		context.Background(),
		fakeRawLedgerStream{records: [][]byte{{3}, {4}}},
		start,
		303,
		outputPath,
		w,
	)
	require.NoError(t, err)
	got, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3, 4}, got)
}

func TestCheckpointIntervalMustBePositive(t *testing.T) {
	_, _, _, err := openCheckpoint(filepath.Join(t.TempDir(), "fetch.json"), 1, 2, 0)
	require.ErrorContains(t, err, "must be greater than zero")
}
