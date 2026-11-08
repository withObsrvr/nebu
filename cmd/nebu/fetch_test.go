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
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			got := archiveDataStoreSchema(tt.ledgersPerFile, tt.filesPerPartition)
			assert.Equal(t, tt.wantLedgersPerFile, got.LedgersPerFile)
			assert.Equal(t, tt.wantFilesPerPartition, got.FilesPerPartition)
		})
	}
}

func TestArchiveBufferConfig(t *testing.T) {
	// The unset case must track the SDK's tuning rather than a nebu constant,
	// so the expectation is read from the SDK for the same input.
	sdkDefault := ingest.DefaultBufferedStorageBackendConfig(0)

	tests := []struct {
		name           string
		bufferSize     int
		numWorkers     int
		ledgersPerFile int
		wantBufferSize uint32
		wantNumWorkers uint32
		wantRetryLimit uint32
	}{
		{
			name:           "unset takes the SDK tuning",
			wantBufferSize: sdkDefault.BufferSize,
			wantNumWorkers: sdkDefault.NumWorkers,
			wantRetryLimit: sdkDefault.RetryLimit,
		},
		{
			name:           "explicit flags win",
			bufferSize:     7,
			numWorkers:     3,
			wantBufferSize: 7,
			wantNumWorkers: 3,
			wantRetryLimit: sdkDefault.RetryLimit,
		},
		{
			name:           "buffer size alone keeps the tuned worker count",
			bufferSize:     7,
			wantBufferSize: 7,
			wantNumWorkers: sdkDefault.NumWorkers,
			wantRetryLimit: sdkDefault.RetryLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := archiveBufferConfig(tt.bufferSize, tt.numWorkers, tt.ledgersPerFile)
			assert.Equal(t, tt.wantBufferSize, got.BufferSize)
			assert.Equal(t, tt.wantNumWorkers, got.NumWorkers)
			assert.Equal(t, tt.wantRetryLimit, got.RetryLimit)
		})
	}
}

func TestArchiveBufferConfigTracksLedgersPerFile(t *testing.T) {
	// A large-file layout gets a different tuning from the SDK than the
	// one-ledger-per-file layout; the point of passing the value through is
	// that nebu does not pin either.
	small := archiveBufferConfig(0, 0, 1)
	large := archiveBufferConfig(0, 0, 64)

	assert.Equal(t, ingest.DefaultBufferedStorageBackendConfig(1).BufferSize, small.BufferSize)
	assert.Equal(t, ingest.DefaultBufferedStorageBackendConfig(64).BufferSize, large.BufferSize)
	assert.NotEqual(t, small.BufferSize, large.BufferSize)
}

func TestArchiveLayoutDisplay(t *testing.T) {
	tests := []struct {
		name              string
		ledgersPerFile    int
		filesPerPartition int
		want              string
	}{
		{name: "both unset", want: "from datastore manifest"},
		{name: "both set", ledgersPerFile: 64, filesPerPartition: 10, want: "64 ledgers/file, 10 files/partition"},
		{name: "ledgers only", ledgersPerFile: 64, want: "64 ledgers/file, files/partition from manifest"},
		{name: "partition only", filesPerPartition: 10, want: "ledgers/file from manifest, 10 files/partition"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, archiveLayoutDisplay(tt.ledgersPerFile, tt.filesPerPartition))
		})
	}
}
