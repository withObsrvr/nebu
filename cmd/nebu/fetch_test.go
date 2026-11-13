package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"iter"
	"net/http"
	"testing"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/support/datastore"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
)

type fakeRawLedgerStream struct {
	records [][]byte
	err     error
}

func (s fakeRawLedgerStream) RawLedgers(
	_ context.Context,
	_ ledgerbackend.Range,
	_ ...ledgerbackend.StreamOption,
) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for _, record := range s.records {
			if !yield(record, nil) {
				return
			}
		}
		if s.err != nil {
			yield(nil, s.err)
		}
	}
}

func TestWriteRawLedgersPreservesExistingXDRStreamFormat(t *testing.T) {
	ledgers := []xdr.LedgerCloseMeta{testLedger(10), testLedger(11)}
	records := make([][]byte, 0, len(ledgers))
	var want bytes.Buffer
	for _, ledger := range ledgers {
		var record bytes.Buffer
		_, err := xdr.Marshal(&record, ledger)
		require.NoError(t, err)
		records = append(records, record.Bytes())
		_, err = want.Write(record.Bytes())
		require.NoError(t, err)
	}

	var got bytes.Buffer
	count, err := writeRawLedgers(
		context.Background(),
		fakeRawLedgerStream{records: records},
		ledgerbackend.BoundedRange(10, 11),
		&got,
		10,
		11,
	)

	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, want.Bytes(), got.Bytes())
}

func TestWriteRawLedgersDoesNotDecodeRecords(t *testing.T) {
	records := [][]byte{{0xde, 0xad}, {0xbe, 0xef}}
	var got bytes.Buffer

	count, err := writeRawLedgers(
		context.Background(),
		fakeRawLedgerStream{records: records},
		ledgerbackend.BoundedRange(20, 21),
		&got,
		20,
		21,
	)

	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, got.Bytes())
}

func TestWriteRawLedgersReportsStreamAndWriteErrors(t *testing.T) {
	t.Run("stream error", func(t *testing.T) {
		sentinel := errors.New("stream failed")
		count, err := writeRawLedgers(
			context.Background(),
			fakeRawLedgerStream{err: sentinel},
			ledgerbackend.SingleLedgerRange(30),
			io.Discard,
			30,
			30,
		)

		assert.Zero(t, count)
		assert.ErrorContains(t, err, sentinel.Error())
	})

	t.Run("short write", func(t *testing.T) {
		count, err := writeRawLedgers(
			context.Background(),
			fakeRawLedgerStream{records: [][]byte{{1, 2, 3}}},
			ledgerbackend.SingleLedgerRange(40),
			shortWriter{},
			40,
			40,
		)

		assert.Zero(t, count)
		assert.ErrorIs(t, err, io.ErrShortWrite)
		assert.ErrorContains(t, err, "ledger 40")
	})

	t.Run("truncated bounded range", func(t *testing.T) {
		count, err := writeRawLedgers(
			context.Background(),
			fakeRawLedgerStream{records: [][]byte{{1}, {2}}},
			ledgerbackend.BoundedRange(50, 52),
			io.Discard,
			50,
			52,
		)

		assert.Equal(t, 2, count)
		assert.ErrorContains(t, err, "incomplete fetch")
		assert.ErrorContains(t, err, "2 of 3 ledgers")
	})

	t.Run("unbounded range may end without a target count", func(t *testing.T) {
		count, err := writeRawLedgers(
			context.Background(),
			fakeRawLedgerStream{records: [][]byte{{1}, {2}}},
			ledgerbackend.UnboundedRange(60),
			io.Discard,
			60,
			0,
		)

		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("unbounded stream cancellation is preserved", func(t *testing.T) {
		count, err := writeRawLedgers(
			context.Background(),
			fakeRawLedgerStream{err: context.Canceled},
			ledgerbackend.UnboundedRange(70),
			io.Discard,
			70,
			0,
		)

		assert.Zero(t, count)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("unbounded context cancellation is preserved", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		count, err := writeRawLedgers(
			ctx,
			fakeRawLedgerStream{records: [][]byte{{1}}},
			ledgerbackend.UnboundedRange(80),
			io.Discard,
			80,
			0,
		)

		assert.Zero(t, count)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestWriteFetchFailureSummary(t *testing.T) {
	tests := []struct {
		name    string
		fetched int
		want    string
	}{
		{
			name:    "partial range",
			fetched: 2,
			want:    "fetched 2/3 ledgers, last=51, reason=archive object missing\n",
		},
		{
			name: "no ledgers",
			want: "fetched 0/3 ledgers, last=none, reason=archive object missing\n",
		},
		{
			name:    "multiline reason",
			fetched: 1,
			want:    "fetched 1/3 ledgers, last=50, reason=archive request failed retry exhausted\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			reason := errors.New("archive object missing")
			if tt.name == "multiline reason" {
				reason = errors.New("archive request failed\nretry exhausted")
			}
			require.NoError(t, writeFetchFailureSummary(&output, 50, 52, tt.fetched, reason))
			assert.Equal(t, tt.want, output.String())
		})
	}
}

func TestRawLedgerRange(t *testing.T) {
	tests := []struct {
		name        string
		start       uint32
		end         uint32
		wantFrom    uint32
		wantTo      uint32
		wantBounded bool
		wantErr     bool
	}{
		{name: "bounded", start: 10, end: 12, wantFrom: 10, wantTo: 12, wantBounded: true},
		{name: "unbounded", start: 10, end: 0, wantFrom: 10},
		{name: "zero start", start: 0, end: 1, wantErr: true},
		{name: "reversed", start: 12, end: 10, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rawLedgerRange(tt.start, tt.end)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFrom, got.From())
			assert.Equal(t, tt.wantTo, got.To())
			assert.Equal(t, tt.wantBounded, got.Bounded())
		})
	}
}

func TestFetchHeaderTransportAddsHeadersWithoutMutatingRequest(t *testing.T) {
	request, err := http.NewRequest(http.MethodPost, "https://example.test", nil)
	require.NoError(t, err)

	transport := &fetchHeaderTransport{
		base: roundTripperFunc(func(got *http.Request) (*http.Response, error) {
			assert.Equal(t, "Api-Key secret", got.Header.Get("Authorization"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     make(http.Header),
				Request:    got,
			}, nil
		}),
		headers: map[string]string{"Authorization": "Api-Key secret"},
	}

	response, err := transport.RoundTrip(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Empty(t, request.Header.Get("Authorization"))
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testLedger(sequence uint32) xdr.LedgerCloseMeta {
	return xdr.LedgerCloseMeta{
		V: 0,
		V0: &xdr.LedgerCloseMetaV0{
			LedgerHeader: xdr.LedgerHeaderHistoryEntry{
				Header: xdr.LedgerHeader{LedgerSeq: xdr.Uint32(sequence)},
			},
		},
	}
}

func TestArchiveDataStoreSchemaLeavesUnsetFieldsToTheManifest(t *testing.T) {
	tests := []struct {
		name                  string
		ledgersPerFile        int
		filesPerPartition     int
		wantLedgersPerFile    uint32
		wantFilesPerPartition uint32
	}{
		// Zero is the signal datastore.LoadSchema reads as "take this from
		// the datastore manifest"; it compares only the fields that are set.
		{name: "both unset defer to the manifest"},
		{name: "ledgers per file only", ledgersPerFile: 64, wantLedgersPerFile: 64},
		{name: "files per partition only", filesPerPartition: 10, wantFilesPerPartition: 10},
		{
			name:                  "both set",
			ledgersPerFile:        64,
			filesPerPartition:     10,
			wantLedgersPerFile:    64,
			wantFilesPerPartition: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := archiveDataStoreSchema(tt.ledgersPerFile, tt.filesPerPartition)
			require.NoError(t, err)
			assert.Equal(t, tt.wantLedgersPerFile, got.LedgersPerFile)
			assert.Equal(t, tt.wantFilesPerPartition, got.FilesPerPartition)
		})
	}
}

func TestArchiveOptionsRejectValuesThatDoNotSurviveNarrowing(t *testing.T) {
	// These options reach the SDK as uint32. A bare conversion is silently
	// wrong at both ends: a negative value becomes an enormous layout or
	// buffer, and 2^32 becomes 0 — which for a layout override silently
	// stops asserting the layout the operator asked to assert.
	tests := []struct {
		name  string
		value int
	}{
		{name: "negative", value: -1},
		{name: "wraps to zero", value: 1 << 32},
		{name: "above the maximum", value: 1<<32 + 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := archiveDataStoreSchema(tt.value, 0)
			require.Error(t, err, "--ledgers-per-file %d must be rejected", tt.value)
			assert.Equal(t, 2, nebuErrors.ExitCode(err), "out-of-range input is a usage error")
			assert.ErrorContains(t, err, "--ledgers-per-file")

			_, err = archiveDataStoreSchema(0, tt.value)
			require.Error(t, err)
			assert.ErrorContains(t, err, "--files-per-partition")

			_, err = archiveBufferConfig(tt.value, 0, 1)
			require.Error(t, err)
			assert.ErrorContains(t, err, "--buffer-size")

			_, err = archiveBufferConfig(0, tt.value, 1)
			require.Error(t, err)
			assert.ErrorContains(t, err, "--num-workers")
		})
	}
}

func TestArchiveBufferConfig(t *testing.T) {
	// The unset case must track the SDK's tuning rather than a nebu constant,
	// so the expectation is read from the SDK for the same input.
	sdkDefault := ingest.DefaultBufferedStorageBackendConfig(1)

	tests := []struct {
		name           string
		bufferSize     int
		numWorkers     int
		wantBufferSize uint32
		wantNumWorkers uint32
	}{
		{
			name:           "unset takes the SDK tuning",
			wantBufferSize: sdkDefault.BufferSize,
			wantNumWorkers: sdkDefault.NumWorkers,
		},
		{
			name:           "explicit flags win",
			bufferSize:     7,
			numWorkers:     3,
			wantBufferSize: 7,
			wantNumWorkers: 3,
		},
		{
			name:           "buffer size alone keeps the tuned worker count",
			bufferSize:     7,
			wantBufferSize: 7,
			wantNumWorkers: sdkDefault.NumWorkers,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := archiveBufferConfig(tt.bufferSize, tt.numWorkers, 1)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBufferSize, got.BufferSize)
			assert.Equal(t, tt.wantNumWorkers, got.NumWorkers)
			assert.Equal(t, sdkDefault.RetryLimit, got.RetryLimit)
		})
	}
}

func TestArchiveBufferConfigTracksTheResolvedLayout(t *testing.T) {
	// The regression this pins: tuning used to be chosen from the local
	// override, which is 0 whenever the layout comes from the manifest. The
	// SDK reads an unknown layout as a small one, so a large-file export was
	// handed the buffer meant for single-ledger files.
	small, err := archiveBufferConfig(0, 0, 1)
	require.NoError(t, err)
	large, err := archiveBufferConfig(0, 0, 64)
	require.NoError(t, err)

	assert.Equal(t, ingest.DefaultBufferedStorageBackendConfig(1).BufferSize, small.BufferSize)
	assert.Equal(t, ingest.DefaultBufferedStorageBackendConfig(64).BufferSize, large.BufferSize)
	assert.Less(t, large.BufferSize, small.BufferSize,
		"a large-file layout must not be given the single-ledger buffer")
}

func TestArchiveDataStoreConfig(t *testing.T) {
	schema := datastore.DataStoreSchema{LedgersPerFile: 64}

	gcs := archiveDataStoreConfig("GCS", "bucket/path", "us-west-2", "passphrase", schema)
	assert.Equal(t, "GCS", gcs.Type)
	assert.Equal(t, "bucket/path", gcs.Params["destination_bucket_path"])
	assert.Equal(t, schema, gcs.Schema)
	// Set so LoadSchema can compare it against the manifest, which is what
	// catches a pubnet-configured fetch aimed at a testnet export.
	assert.Equal(t, "passphrase", gcs.NetworkPassphrase)
	assert.NotContains(t, gcs.Params, "region", "region is an S3 parameter")

	s3 := archiveDataStoreConfig("S3", "bucket/path", "us-west-2", "passphrase", schema)
	assert.Equal(t, "us-west-2", s3.Params["region"])
}

func TestArchiveLayoutDisplay(t *testing.T) {
	tests := []struct {
		name      string
		resolved  datastore.DataStoreSchema
		overrides datastore.DataStoreSchema
		want      string
	}{
		{
			name:     "resolved from the manifest",
			resolved: datastore.DataStoreSchema{LedgersPerFile: 1, FilesPerPartition: 64000},
			want:     "1 ledgers/file, 64000 files/partition (datastore manifest)",
		},
		{
			name:      "fully overridden",
			resolved:  datastore.DataStoreSchema{LedgersPerFile: 64, FilesPerPartition: 10},
			overrides: datastore.DataStoreSchema{LedgersPerFile: 64, FilesPerPartition: 10},
			want:      "64 ledgers/file, 10 files/partition (flags)",
		},
		{
			name:      "half overridden",
			resolved:  datastore.DataStoreSchema{LedgersPerFile: 64, FilesPerPartition: 64000},
			overrides: datastore.DataStoreSchema{LedgersPerFile: 64},
			want:      "64 ledgers/file, 64000 files/partition (flags and manifest)",
		},
		{
			// The manifest could not be read; the stream reports why.
			name: "unresolved",
			want: "unresolved (datastore manifest)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, archiveLayoutDisplay(tt.resolved, tt.overrides))
		})
	}
}

func TestFetchLastRejectsIncoherentCombinations(t *testing.T) {
	// These are all caught before any network call, so the test needs no
	// endpoint. Each one is a case where guessing which input the operator
	// meant would quietly produce a different range than they asked for.
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "last with an explicit range",
			args: []string{"--program-status=never", "fetch", "--last", "10", "65430000", "65430010"},
		},
		{
			name: "last with a single explicit ledger",
			args: []string{"--program-status=never", "fetch", "--last", "10", "65430000"},
		},
		{
			name: "neither a range nor last",
			args: []string{"--program-status=never", "fetch"},
		},
		{
			// A checkpoint resumes a fixed range; --last resolves a
			// different one as the chain advances.
			name: "last with a checkpoint",
			args: []string{"--program-status=never", "fetch", "--last", "10",
				"--checkpoint", "/nonexistent/ckpt.json"},
		},
		{
			name: "last is not a number",
			args: []string{"--program-status=never", "fetch", "--last", "-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetStatusStream(t)
			cmd := newRootCmd()
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			assert.Equal(t, 2, execute(cmd),
				"an incoherent range request is a usage error, not a guess")
		})
	}
}
