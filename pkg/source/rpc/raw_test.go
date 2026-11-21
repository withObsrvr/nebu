package rpc

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/withObsrvr/nebu/pkg/source"
)

// The raw path must be a RawLedgerSource, or callers type-asserting for it
// silently fall back to decoding.
var _ source.RawLedgerSource = (*LedgerSource)(nil)

func TestStreamRaw(t *testing.T) {
	sentinel := errors.New("stream failed")
	stop := errors.New("caller stopped")

	tests := []struct {
		name      string
		stream    fakeStream
		start     uint32
		end       uint32
		fn        func(seq uint32, raw []byte) error
		wantCount uint32
		wantErr   error
	}{
		{
			name:      "bounded range delivers every ledger",
			stream:    sequentialStream(),
			start:     10,
			end:       14,
			wantCount: 5,
		},
		{
			name:      "single ledger",
			stream:    sequentialStream(),
			start:     10,
			end:       10,
			wantCount: 1,
		},
		{
			// The count is the only thing that can notice a short range: the
			// stream ending early is not an error the source can report.
			name:      "a stream that fails part-way reports what it delivered",
			stream:    fakeStream{ledger: ledgerWithSeq, failAfter: 2, err: sentinel},
			start:     10,
			end:       14,
			wantCount: 2,
			wantErr:   sentinel,
		},
		{
			// Returned unwrapped, so a caller can match on its own error.
			name:      "an error from fn stops the stream and is returned as-is",
			stream:    sequentialStream(),
			start:     10,
			end:       14,
			fn:        func(seq uint32, _ []byte) error { return stop },
			wantCount: 0,
			wantErr:   stop,
		},
		{
			name:    "an invalid range is refused before streaming",
			stream:  sequentialStream(),
			start:   0,
			end:     5,
			wantErr: errInvalidRange,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &LedgerSource{stream: tt.stream}
			fn := tt.fn
			if fn == nil {
				fn = func(uint32, []byte) error { return nil }
			}

			count, err := src.StreamRaw(context.Background(), tt.start, tt.end, fn)

			if tt.wantErr == errInvalidRange {
				if err == nil {
					t.Fatal("StreamRaw() error = nil, want a range error")
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("StreamRaw() error = %v, want %v", err, tt.wantErr)
			}
			if count != tt.wantCount {
				t.Errorf("StreamRaw() count = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

// errInvalidRange is a marker for the table above: Range's error is built
// with fmt.Errorf and has no sentinel to match on.
var errInvalidRange = errors.New("invalid range")

// Sequences must arrive in order and start at the requested ledger, because
// a caller writing a file has no other way to label what it wrote.
func TestStreamRawReportsSequencesInOrder(t *testing.T) {
	src := &LedgerSource{stream: sequentialStream()}

	var got []uint32
	count, err := src.StreamRaw(context.Background(), 100, 104, func(seq uint32, _ []byte) error {
		got = append(got, seq)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamRaw() error = %v", err)
	}

	want := []uint32{100, 101, 102, 103, 104}
	if count != uint32(len(want)) {
		t.Errorf("count = %d, want %d", count, len(want))
	}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("sequences = %v, want %v", got, want)
		}
	}
}

// The bytes handed to fn must be the ledger's own XDR, not an empty or
// recycled buffer. This is the assertion that would catch a pump that
// forwarded the slice after the stream moved on.
func TestStreamRawDeliversDistinctLedgerBytes(t *testing.T) {
	src := &LedgerSource{stream: sequentialStream()}

	var copies [][]byte
	if _, err := src.StreamRaw(context.Background(), 1, 3, func(_ uint32, raw []byte) error {
		if len(raw) == 0 {
			t.Error("fn received an empty slice")
		}
		copies = append(copies, bytes.Clone(raw))
		return nil
	}); err != nil {
		t.Fatalf("StreamRaw() error = %v", err)
	}

	if len(copies) != 3 {
		t.Fatalf("got %d ledgers, want 3", len(copies))
	}
	// Different ledgers encode differently; identical bytes would mean every
	// call saw the same underlying buffer.
	if bytes.Equal(copies[0], copies[1]) || bytes.Equal(copies[1], copies[2]) {
		t.Error("consecutive ledgers produced identical bytes, so fn is seeing a shared buffer")
	}
}

// A cancelled caller must not receive another ledger.
func TestStreamRawStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	src := &LedgerSource{stream: sequentialStream()}

	calls := 0
	count, err := src.StreamRaw(ctx, 1, 0, func(uint32, []byte) error {
		calls++
		if calls == 2 {
			cancel()
		}
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("StreamRaw() error = %v, want context.Canceled", err)
	}
	if calls != 2 {
		t.Errorf("fn called %d times after cancel, want 2", calls)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}
