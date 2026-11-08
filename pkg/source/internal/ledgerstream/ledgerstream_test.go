package ledgerstream

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRange(t *testing.T) {
	tests := []struct {
		name        string
		start       uint32
		end         uint32
		wantFrom    uint32
		wantTo      uint32
		wantBounded bool
		wantErr     string
	}{
		{name: "bounded", start: 10, end: 12, wantFrom: 10, wantTo: 12, wantBounded: true},
		{name: "single ledger", start: 10, end: 10, wantFrom: 10, wantTo: 10, wantBounded: true},
		{name: "unbounded", start: 10, end: 0, wantFrom: 10},
		{name: "zero start", start: 0, end: 10, wantErr: "start ledger must be > 0"},
		{name: "reversed", start: 12, end: 10, wantErr: "invalid range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Range(tt.start, tt.end)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFrom, got.From())
			assert.Equal(t, tt.wantTo, got.To())
			assert.Equal(t, tt.wantBounded, got.Bounded())
		})
	}
}

// recordStream yields the given raw records, then the given error if set.
type recordStream struct {
	records [][]byte
	err     error
}

func (s recordStream) RawLedgers(
	ctx context.Context,
	_ ledgerbackend.Range,
	_ ...ledgerbackend.StreamOption,
) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for _, record := range s.records {
			if ctx.Err() != nil {
				return
			}
			if !yield(record, nil) {
				return
			}
		}
		if s.err != nil {
			yield(nil, s.err)
		}
	}
}

func rawLedger(t *testing.T, sequence uint32) []byte {
	t.Helper()

	ledger := xdr.LedgerCloseMeta{
		V: 0,
		V0: &xdr.LedgerCloseMetaV0{
			LedgerHeader: xdr.LedgerHeaderHistoryEntry{
				Header: xdr.LedgerHeader{LedgerSeq: xdr.Uint32(sequence)},
			},
		},
	}
	raw, err := ledger.MarshalBinary()
	require.NoError(t, err)
	return raw
}

func TestPumpDecodesEveryRecordInOrder(t *testing.T) {
	stream := recordStream{records: [][]byte{
		rawLedger(t, 10), rawLedger(t, 11), rawLedger(t, 12),
	}}
	ledgerRange, err := Range(10, 12)
	require.NoError(t, err)

	out := make(chan xdr.LedgerCloseMeta, 3)
	require.NoError(t, Pump(context.Background(), stream, ledgerRange, out))
	close(out)

	var got []uint32
	for ledger := range out {
		got = append(got, ledger.LedgerSequence())
	}
	assert.Equal(t, []uint32{10, 11, 12}, got)
}

func TestPumpDoesNotAliasTheBorrowedRecord(t *testing.T) {
	// RawLedgers yields a slice that is only valid until the next iteration
	// step. Pump sends decoded values to another goroutine, so overwriting
	// the record after it was sent must not change what was sent.
	record := rawLedger(t, 10)
	stream := recordStream{records: [][]byte{record}}
	ledgerRange, err := Range(10, 10)
	require.NoError(t, err)

	out := make(chan xdr.LedgerCloseMeta, 1)
	require.NoError(t, Pump(context.Background(), stream, ledgerRange, out))

	for i := range record {
		record[i] = 0xff
	}

	ledger := <-out
	assert.Equal(t, uint32(10), ledger.LedgerSequence())
}

func TestPumpWrapsStreamErrorsWithTheLedgerSequence(t *testing.T) {
	sentinel := errors.New("storage unavailable")
	stream := recordStream{records: [][]byte{rawLedger(t, 10)}, err: sentinel}
	ledgerRange, err := Range(10, 12)
	require.NoError(t, err)

	out := make(chan xdr.LedgerCloseMeta, 3)
	pumpErr := Pump(context.Background(), stream, ledgerRange, out)

	require.Error(t, pumpErr)
	assert.ErrorIs(t, pumpErr, sentinel, "the cause must survive wrapping")
	// Naming the ledger it stopped on is what makes the gap identifiable.
	assert.ErrorContains(t, pumpErr, "ledger 11")
}

func TestPumpReportsUndecodableRecords(t *testing.T) {
	stream := recordStream{records: [][]byte{[]byte("not xdr")}}
	ledgerRange, err := Range(10, 10)
	require.NoError(t, err)

	out := make(chan xdr.LedgerCloseMeta, 1)
	pumpErr := Pump(context.Background(), stream, ledgerRange, out)

	require.Error(t, pumpErr)
	assert.ErrorContains(t, pumpErr, "failed to decode ledger 10")
}

func TestPumpReturnsTheContextErrorOnCancellation(t *testing.T) {
	// A stream that stops early because the context was cancelled looks
	// exactly like a completed range from the iterator alone, so Pump has
	// to consult the context to tell them apart.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stream := recordStream{records: [][]byte{rawLedger(t, 10)}}
	ledgerRange, err := Range(10, 12)
	require.NoError(t, err)

	out := make(chan xdr.LedgerCloseMeta, 1)
	assert.ErrorIs(t, Pump(ctx, stream, ledgerRange, out), context.Canceled)
}
