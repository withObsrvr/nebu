package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/source"
)

// mockOrigin is a simple origin processor for testing
type mockOrigin struct {
	name           string
	processedCount int
	mu             sync.Mutex
}

func newMockOrigin(name string) *mockOrigin {
	return &mockOrigin{name: name}
}

func (m *mockOrigin) Name() string {
	return m.name
}

func (m *mockOrigin) Type() processor.Type {
	return processor.TypeOrigin
}

func (m *mockOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processedCount++
	return nil
}

func (m *mockOrigin) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.processedCount
}

func TestRuntime_RunOrigin(t *testing.T) {
	rt := NewRuntime()
	src, err := source.NewRPCLedgerSource("https://mainnet.sorobanrpc.com")
	require.NoError(t, err)
	defer src.Close()

	origin := newMockOrigin("test-origin")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run origin with a small range
	err = rt.RunOrigin(ctx, src, origin, 60200000, 60200004)
	require.NoError(t, err)

	// Verify all ledgers were processed
	assert.Equal(t, 5, origin.Count(), "should have processed 5 ledgers")
}

func TestRuntime_RunOrigin_Cancellation(t *testing.T) {
	rt := NewRuntime()
	src, err := source.NewRPCLedgerSource("https://mainnet.sorobanrpc.com")
	require.NoError(t, err)
	defer src.Close()

	origin := newMockOrigin("test-origin")

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a delay that allows some processing
	go func() {
		time.Sleep(2 * time.Second)
		cancel()
	}()

	// Try to run with a larger range that will be cancelled
	err = rt.RunOrigin(ctx, src, origin, 60200000, 60200050)

	// Should get a context error
	assert.ErrorIs(t, err, context.Canceled)

	// Should have processed some ledgers before cancellation
	// (might be 0 if cancellation happens very fast, so we just check it didn't complete)
	assert.Less(t, origin.Count(), 51, "should not have processed all ledgers")
}

func TestRuntime_RunOrigin_InvalidRange(t *testing.T) {
	rt := NewRuntime()
	src, err := source.NewRPCLedgerSource("https://mainnet.sorobanrpc.com")
	require.NoError(t, err)
	defer src.Close()

	origin := newMockOrigin("test-origin")

	ctx := context.Background()

	// Invalid range should cause source to return error
	err = rt.RunOrigin(ctx, src, origin, 0, 100)
	assert.Error(t, err, "should reject invalid range")
}
