package main

import (
	"context"
	"errors"
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
	dir := t.TempDir()
	path := filepath.Join(dir, "fetch.json")
	outputPath := filepath.Join(dir, "ledgers.xdr")
	w, _, _, err := openCheckpoint(path, 200, 203, 100)
	require.NoError(t, err)

	err = fetchRawLedgers(
		context.Background(),
		fakeRawLedgerStream{records: [][]byte{{1}, {2}}, err: context.Canceled},
		200,
		203,
		outputPath,
		w,
	)
	assert.ErrorContains(t, err, context.Canceled.Error())

	_, start, complete, err := openCheckpoint(path, 200, 203, 100)
	require.NoError(t, err)
	assert.Equal(t, uint32(202), start)
	assert.False(t, complete)
}

func TestCheckpointRejectsOutputPathAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data")

	t.Run("identical cleaned paths", func(t *testing.T) {
		err := validateCheckpointOutputPaths(path, filepath.Join(dir, ".", "data"))
		require.ErrorContains(t, err, "must refer to different files")
	})

	t.Run("hard link aliases", func(t *testing.T) {
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))
		alias := filepath.Join(dir, "alias")
		require.NoError(t, os.Link(path, alias))

		err := validateCheckpointOutputPaths(path, alias)
		require.ErrorContains(t, err, "must refer to different files")
	})

	t.Run("nonexistent files beneath symlinked parent", func(t *testing.T) {
		realDir := filepath.Join(dir, "real")
		require.NoError(t, os.Mkdir(realDir, 0o755))
		linkedDir := filepath.Join(dir, "linked")
		require.NoError(t, os.Symlink(realDir, linkedDir))

		err := validateCheckpointOutputPaths(
			filepath.Join(realDir, "future"),
			filepath.Join(linkedDir, "future"),
		)
		require.ErrorContains(t, err, "must refer to different files")
	})
}

func TestOutputSyncPrecedesCheckpointPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fetch.json")
	w, _, _, err := openCheckpoint(path, 400, 400, 1)
	require.NoError(t, err)

	syncCalls := 0
	_, err = writeRawLedgersWithCheckpoint(
		context.Background(),
		fakeRawLedgerStream{records: [][]byte{{1}}},
		ledgerbackend.BoundedRange(400, 400),
		io.Discard,
		400,
		400,
		w,
		func() error {
			syncCalls++
			_, statErr := os.Stat(path)
			assert.ErrorIs(t, statErr, os.ErrNotExist)
			return nil
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, syncCalls)
	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestOutputSyncFailureDoesNotAdvanceCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fetch.json")
	w, _, _, err := openCheckpoint(path, 500, 500, 1)
	require.NoError(t, err)

	_, err = writeRawLedgersWithCheckpoint(
		context.Background(),
		fakeRawLedgerStream{records: [][]byte{{1}}},
		ledgerbackend.BoundedRange(500, 500),
		io.Discard,
		500,
		500,
		w,
		func() error { return errors.New("sync failed") },
	)
	require.ErrorContains(t, err, "sync failed")
	_, err = os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
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
