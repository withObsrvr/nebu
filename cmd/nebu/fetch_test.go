package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"iter"
	"net/http"
	"testing"

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
