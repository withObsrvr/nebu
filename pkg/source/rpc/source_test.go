package rpc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/clients/rpcclient"
	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRPCURL = "https://archive-rpc.lightsail.network"

// recentLedgerWindow returns the latest ledger sequence reported by the RPC
// health endpoint. lookback is the deepest offset the caller will subtract
// from the result; the helper fails fast if the endpoint's retention window
// no longer covers that offset, instead of letting the test die later on a
// confusing wrapped RPC error.
func recentLedgerWindow(t *testing.T, lookback uint32) uint32 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := rpcclient.NewClient(testRPCURL, nil)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	health, err := client.GetHealth(ctx)
	require.NoError(t, err)
	require.Greater(t, health.LatestLedger, lookback,
		"latest ledger %d too low to subtract lookback %d without underflow",
		health.LatestLedger, lookback)
	require.GreaterOrEqual(t, health.LatestLedger-lookback, health.OldestLedger,
		"RPC retention window too small for this test (latest=%d oldest=%d lookback=%d)",
		health.LatestLedger, health.OldestLedger, lookback)
	return health.LatestLedger
}

func TestRPCLedgerSource_Stream(t *testing.T) {
	// This is an integration test that hits real Stellar RPC.
	if testing.Short() {
		t.Skip("skipping network-dependent RPC integration test in short mode")
	}

	src, err := NewLedgerSource(testRPCURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, src.Close())
	})

	// Stay a few ledgers behind the network head to avoid indexing races.
	// Resolve the range before creating the stream context so the health
	// call doesn't eat into the stream's timeout budget.
	endLedger := recentLedgerWindow(t, 9) - 5
	startLedger := endLedger - 4

	ch := make(chan xdr.LedgerCloseMeta, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- src.Stream(ctx, startLedger, endLedger, ch)
	}()

	// Collect ledgers
	var ledgers []xdr.LedgerCloseMeta
	for ledger := range ch {
		ledgers = append(ledgers, ledger)
	}
	require.NoError(t, <-errCh)

	// Should have received exactly 5 ledgers
	assert.Equal(t, 5, len(ledgers), "expected 5 ledgers")

	// Verify ledger sequences are correct
	for i, ledger := range ledgers {
		expectedSeq := startLedger + uint32(i)
		actualSeq := ledger.LedgerSequence()
		assert.Equal(t, expectedSeq, actualSeq, "ledger %d has wrong sequence", i)
	}
}

func TestRPCLedgerSource_InvalidRange(t *testing.T) {
	// Range validation fails before any RPC call, so this runs offline.
	src, err := NewLedgerSource(testRPCURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, src.Close())
	})

	// Bound the context so a validation regression turns into a fast,
	// clear failure instead of a hanging network call.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tests := []struct {
		name    string
		start   uint32
		end     uint32
		wantErr string
	}{
		{"zero start", 0, 100, "start ledger must be > 0"},
		{"both zero", 0, 0, "start ledger must be > 0"},
		{"start > end", 100, 50, "invalid range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh channel for each test to avoid close panics
			ch := make(chan xdr.LedgerCloseMeta, 10)
			err := src.Stream(ctx, tt.start, tt.end, ch)
			assert.ErrorContains(t, err, tt.wantErr, "expected validation error for invalid range")
		})
	}
}

func TestRPCLedgerSource_Unbounded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent RPC integration test in short mode")
	}

	src, err := NewLedgerSource(testRPCURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, src.Close())
	})

	// Stay a few ledgers behind the network head to avoid indexing races.
	// Resolve this before creating the stream context so the health call
	// doesn't eat into the stream's timeout budget.
	startLedger := recentLedgerWindow(t, 5) - 5

	ch := make(chan xdr.LedgerCloseMeta, 10)
	// Overall test timeout — if the RPC stalls or the Stream impl fails
	// to close ch after cancellation, this prevents the test (and CI)
	// from hanging indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Capture Stream's return so we can assert the goroutine actually
	// exited with the expected context error after cancel().
	errCh := make(chan error, 1)
	go func() {
		errCh <- src.Stream(ctx, startLedger, 0, ch)
	}()

	// Read up to 2 ledgers, then trigger cancellation. Using a select
	// against ctx.Done() guarantees we can't get stuck on the channel
	// read if the stream stalls.
	count := 0
readLoop:
	for count < 2 {
		select {
		case _, ok := <-ch:
			if !ok {
				break readLoop
			}
			count++
			if count == 2 {
				cancel()
			}
		case <-ctx.Done():
			break readLoop
		}
	}

	// Drain remaining ledgers until Stream closes ch. Bounded by a
	// safety timeout so a broken Stream impl can't hang the test.
	drainTimeout := time.After(5 * time.Second)
drainLoop:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break drainLoop
			}
			count++
		case <-drainTimeout:
			t.Error("timed out draining channel after cancellation — Stream may not be closing ch")
			break drainLoop
		}
	}

	// Assert the Stream goroutine returned within a short window.
	// After cancel(), Stream should return context.Canceled; if the
	// overall ctx deadline fired first we accept DeadlineExceeded.
	// Stream wraps backend errors, so match with errors.Is — and an
	// unbounded stream has no clean-exit path, so nil is a bug too.
	select {
	case streamErr := <-errCh:
		require.Error(t, streamErr, "unbounded Stream must not return nil")
		if !errors.Is(streamErr, context.Canceled) && !errors.Is(streamErr, context.DeadlineExceeded) {
			t.Errorf("Stream returned unexpected error: %v", streamErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stream goroutine did not return after cancellation")
	}

	assert.GreaterOrEqual(t, count, 1, "unbounded stream should yield ledgers before cancellation")
}

func TestRPCLedgerSource_Cancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent RPC integration test in short mode")
	}

	src, err := NewLedgerSource(testRPCURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, src.Close())
	})

	// Start streaming 38 recent ledgers, staying behind the network head.
	endLedger := recentLedgerWindow(t, 42) - 5
	startLedger := endLedger - 37

	ch := make(chan xdr.LedgerCloseMeta, 10)
	// Deadline guard: cancel() is expected to end the stream, but if the
	// RPC stalls first this keeps the test from hanging indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- src.Stream(ctx, startLedger, endLedger, ch)
	}()

	// Read a few ledgers then cancel. Selecting against ctx.Done()
	// guarantees we can't get stuck on the channel read.
	count := 0
	cancelled := false
readLoop:
	for count < 3 {
		select {
		case _, ok := <-ch:
			if !ok {
				break readLoop
			}
			count++
			if count == 3 {
				cancel()
				cancelled = true
			}
		case <-ctx.Done():
			break readLoop
		}
	}

	// Drain remaining ledgers until Stream closes ch. Bounded by a
	// safety timeout so a broken Stream impl can't hang the test.
	drainTimeout := time.After(5 * time.Second)
drainLoop:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break drainLoop
			}
			count++
		case <-drainTimeout:
			t.Error("timed out draining channel after cancellation — Stream may not be closing ch")
			break drainLoop
		}
	}

	select {
	case streamErr := <-errCh:
		if cancelled {
			assert.ErrorIs(t, streamErr, context.Canceled)
		} else {
			// cancel() never fired: the stream ended or the deadline hit
			// first. Surface the underlying error instead of a misleading
			// Canceled mismatch.
			t.Errorf("stream ended before cancel() fired after %d ledgers: %v", count, streamErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stream goroutine did not return after cancellation")
	}

	// Should have stopped early (not all 38 ledgers)
	assert.Less(t, count, 38, "should have stopped before completing full range")
}

func TestNewLedgerSource_EmptyURL(t *testing.T) {
	_, err := NewLedgerSource("")
	assert.Error(t, err, "should reject empty URL")
}

func TestHeaderTransport_RoundTrip(t *testing.T) {
	received := make(chan http.Header, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case received <- r.Header.Clone():
		default:
		}
	}))
	defer srv.Close()

	transport := &headerTransport{
		base: http.DefaultTransport,
		headers: map[string]string{
			"Authorization": "Api-Key test-key",
			"X-Custom":      "custom-value",
		},
	}
	client := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	got := <-received
	assert.Equal(t, "Api-Key test-key", got.Get("Authorization"))
	assert.Equal(t, "custom-value", got.Get("X-Custom"))

	// RoundTrip must clone the request, not mutate the caller's copy.
	assert.Empty(t, req.Header.Get("Authorization"))
	assert.Empty(t, req.Header.Get("X-Custom"))
}

func TestNewLedgerSourceWithHeaders_SendsHeaders(t *testing.T) {
	received := make(chan http.Header, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case received <- r.Header.Clone():
		default:
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	src, err := NewLedgerSourceWithHeaders(srv.URL, map[string]string{
		"Authorization": "Api-Key test-key",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, src.Close())
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The stream fails (the fake server always errors), but preparing the
	// range must send at least one RPC request through the custom transport.
	ch := make(chan xdr.LedgerCloseMeta, 1)
	err = src.Stream(ctx, 1, 2, ch)
	require.Error(t, err)

	select {
	case got := <-received:
		assert.Equal(t, "Api-Key test-key", got.Get("Authorization"))
	default:
		t.Fatal("RPC server received no requests")
	}
}

// fakeBackend is an offline LedgerBackend for exercising Stream's error and
// success paths without a live RPC server.
type fakeBackend struct {
	prepareErr error
	getLedger  func(seq uint32) (xdr.LedgerCloseMeta, error)
}

func (f *fakeBackend) GetLatestLedgerSequence(ctx context.Context) (uint32, error) {
	// Not exercised by Stream today; fail loudly if that ever changes.
	return 0, errors.New("fakeBackend: GetLatestLedgerSequence not implemented")
}

func (f *fakeBackend) GetLedger(ctx context.Context, seq uint32) (xdr.LedgerCloseMeta, error) {
	if f.getLedger == nil {
		return xdr.LedgerCloseMeta{}, errors.New("fakeBackend: unexpected GetLedger call")
	}
	return f.getLedger(seq)
}

func (f *fakeBackend) PrepareRange(ctx context.Context, r ledgerbackend.Range) error {
	return f.prepareErr
}

func (f *fakeBackend) IsPrepared(ctx context.Context, r ledgerbackend.Range) (bool, error) {
	// Not exercised by Stream today; fail loudly if that ever changes.
	return false, errors.New("fakeBackend: IsPrepared not implemented")
}

func (f *fakeBackend) Close() error { return nil }

func ledgerWithSeq(seq uint32) xdr.LedgerCloseMeta {
	return xdr.LedgerCloseMeta{
		V: 0,
		V0: &xdr.LedgerCloseMetaV0{
			LedgerHeader: xdr.LedgerHeaderHistoryEntry{
				Header: xdr.LedgerHeader{LedgerSeq: xdr.Uint32(seq)},
			},
		},
	}
}

func TestStream_ErrorPaths(t *testing.T) {
	sentinel := errors.New("backend boom")

	tests := []struct {
		name    string
		backend *fakeBackend
		wantErr string
	}{
		{
			name:    "prepare range fails",
			backend: &fakeBackend{prepareErr: sentinel},
			wantErr: "failed to prepare range",
		},
		{
			name: "get ledger fails",
			backend: &fakeBackend{getLedger: func(seq uint32) (xdr.LedgerCloseMeta, error) {
				return xdr.LedgerCloseMeta{}, sentinel
			}},
			wantErr: "failed to get ledger 10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &LedgerSource{backend: tt.backend}
			ch := make(chan xdr.LedgerCloseMeta, 1)

			errCh := make(chan error, 1)
			go func() {
				errCh <- src.Stream(context.Background(), 10, 12, ch)
			}()

			// Stream must close out on every return path; a consumer
			// ranging over the channel would otherwise deadlock.
			drainTimeout := time.After(5 * time.Second)
		drainLoop:
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						break drainLoop
					}
				case <-drainTimeout:
					t.Fatal("Stream did not close the output channel on error")
				}
			}

			select {
			case err := <-errCh:
				assert.ErrorIs(t, err, sentinel, "backend error must be wrapped, not replaced")
				assert.ErrorContains(t, err, tt.wantErr)
			case <-time.After(5 * time.Second):
				t.Fatal("Stream did not return an error")
			}
		})
	}
}

func TestStream_BoundedOffline(t *testing.T) {
	tests := []struct {
		name  string
		start uint32
		end   uint32
		want  []uint32
	}{
		{"multi ledger", 5, 9, []uint32{5, 6, 7, 8, 9}},
		{"single ledger", 7, 7, []uint32{7}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &fakeBackend{getLedger: func(seq uint32) (xdr.LedgerCloseMeta, error) {
				return ledgerWithSeq(seq), nil
			}}
			src := &LedgerSource{backend: backend}
			ch := make(chan xdr.LedgerCloseMeta, 10)

			errCh := make(chan error, 1)
			go func() {
				errCh <- src.Stream(context.Background(), tt.start, tt.end, ch)
			}()

			var got []uint32
			for ledger := range ch {
				got = append(got, ledger.LedgerSequence())
			}
			require.NoError(t, <-errCh)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStream_CancellationOffline(t *testing.T) {
	backend := &fakeBackend{getLedger: func(seq uint32) (xdr.LedgerCloseMeta, error) {
		return ledgerWithSeq(seq), nil
	}}
	src := &LedgerSource{backend: backend}
	// Unbuffered so the endless stream blocks on send until we read,
	// exercising the ctx.Done branch of Stream's send select.
	ch := make(chan xdr.LedgerCloseMeta)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- src.Stream(ctx, 1, 0, ch)
	}()

	// Read a couple of ledgers from the endless stream, then cancel.
	for range 2 {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for ledger from fake backend")
		}
	}
	cancel()

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("Stream did not return after cancellation")
	}

	// Stream must close the channel on the cancellation return path too.
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "Stream must close the output channel after cancellation")
	case <-time.After(5 * time.Second):
		t.Fatal("Stream did not close the output channel after cancellation")
	}
}
