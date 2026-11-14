package rpc

import "testing"

// The SDK's RPC backend asks for BufferSize ledgers per getLedgers call and
// defaults to 10, so nebu's one-ledger range downloaded ten ledgers: 11.9 MB
// fetched to use 1.0 MB, and the download was 2.0 of the run's 2.4 seconds.
// Narrowing the buffer to the range took a single-ledger run from ~1.8s to
// ~0.8s against the same endpoint.
//
// Only ranges below the default are narrowed. A larger buffer is the right
// trade for a long range -- fewer round trips, and fewer of the GetHealth
// calls the backend makes on every buffer miss -- so 0 here means "leave the
// default alone".
func TestBufferSizeFor(t *testing.T) {
	tests := []struct {
		name  string
		start uint32
		end   uint32
		want  uint32
	}{
		{
			name:  "one ledger asks for one",
			start: 65447235,
			end:   65447235,
			want:  1,
		},
		{
			name:  "three ledgers ask for three",
			start: 65447235,
			end:   65447237,
			want:  3,
		},
		{
			// One below the default, still worth narrowing.
			name:  "nine ledgers ask for nine",
			start: 100,
			end:   108,
			want:  9,
		},
		{
			// Exactly the default: nothing to gain, and setting it would be
			// the same number by a longer route.
			name:  "ten ledgers keep the default",
			start: 100,
			end:   109,
			want:  0,
		},
		{
			name:  "a long range keeps the default",
			start: 60200000,
			end:   60200100,
			want:  0,
		},
		{
			// --follow and unbounded streaming: there is no range to size to.
			name:  "unbounded keeps the default",
			start: 65447235,
			end:   0,
			want:  0,
		},
		{
			// Range() rejects this; sizing must not underflow on the way
			// there. end-start+1 on these would wrap to a huge buffer.
			name:  "a reversed range keeps the default",
			start: 200,
			end:   100,
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bufferSizeFor(tt.start, tt.end); got != tt.want {
				t.Errorf("bufferSizeFor(%d, %d) = %d, want %d", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

// streamFor must hand back the source's own stream whenever the default
// stands, so an unbounded run does not pay to rebuild one.
func TestStreamForReusesTheSourceStream(t *testing.T) {
	src, err := NewLedgerSource("https://archive-rpc.lightsail.network")
	if err != nil {
		t.Fatalf("NewLedgerSource: %v", err)
	}

	if got := src.streamFor(100, 0); got != src.stream {
		t.Error("unbounded range built a new stream")
	}
	if got := src.streamFor(100, 200); got != src.stream {
		t.Error("a range at or above the default built a new stream")
	}
	if got := src.streamFor(100, 102); got == src.stream {
		t.Error("a three-ledger range reused the default-sized stream")
	}
}

// A source handed a stream directly must use it for every range. The first
// version of this change rebuilt the stream whenever the range was short,
// which quietly replaced an injected fake with a real RPC client and broke
// every offline test in this package.
func TestStreamForKeepsAnInjectedStream(t *testing.T) {
	injected := &fakeStream{}
	src := &LedgerSource{stream: injected}

	for _, r := range [][2]uint32{{100, 100}, {100, 102}, {100, 200}, {100, 0}} {
		if got := src.streamFor(r[0], r[1]); got != injected {
			t.Errorf("streamFor(%d, %d) replaced the injected stream", r[0], r[1])
		}
	}
}
