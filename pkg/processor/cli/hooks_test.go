package cli

import (
	"context"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"

	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
)

// noopSource emits a single dummy ledger and closes its output. Used
// to drive a minimal pipeline through runtime.RunOrigin without
// hitting real Stellar RPC.
type noopSource struct{}

func (s *noopSource) Stream(ctx context.Context, start, end uint32, out chan<- xdr.LedgerCloseMeta) error {
	defer close(out)
	for seq := start; seq <= end; seq++ {
		var ledger xdr.LedgerCloseMeta
		ledger.V = 0
		ledger.V0 = &xdr.LedgerCloseMetaV0{
			LedgerHeader: xdr.LedgerHeaderHistoryEntry{
				Header: xdr.LedgerHeader{LedgerSeq: xdr.Uint32(seq)},
			},
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- ledger:
		}
	}
	return nil
}
func (s *noopSource) Close() error { return nil }
func newNoopSource() *noopSource    { return &noopSource{} }

// noopOrigin is a do-nothing origin used to drive runtime.RunOrigin.
type noopOrigin struct{ name string }

func (o *noopOrigin) Name() string                                                       { return o.name }
func (o *noopOrigin) Type() processor.Type                                               { return processor.TypeOrigin }
func (o *noopOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta)      {}
func newNoopOrigin(name string) *noopOrigin                                              { return &noopOrigin{name: name} }

func TestAttachHooks_NoHooks(t *testing.T) {
	rt := runtime.NewRuntime()
	// Calling with nil or empty slice must be a no-op (not panic).
	attachHooks(rt, nil)
	attachHooks(rt, []runtime.Hooks{})
}

func TestAttachHooks_StackedBundles(t *testing.T) {
	// Verify that attachHooks installs each bundle in order by
	// observing OnStart firings via runtime.RunOrigin against an
	// in-memory source. This is the integration check that
	// OriginConfig.Hooks → attachHooks → rt.Use → rt.RunOrigin
	// actually delivers callbacks at runtime.
	var order []string
	rt := runtime.NewRuntime()
	attachHooks(rt, []runtime.Hooks{
		{OnStart: func(ctx context.Context, _ runtime.PipelineInfo) {
			order = append(order, "first")
		}},
		{OnStart: func(ctx context.Context, _ runtime.PipelineInfo) {
			order = append(order, "second")
		}},
	})

	// Build the in-memory test fixtures from pkg/runtime's test
	// helpers. We can't import them directly (they're in the
	// runtime package's test scope), so we re-use the public API:
	// any LedgerSource + processor.Origin pair will do.
	src := newNoopSource()
	origin := newNoopOrigin("attach-hooks-test")

	err := rt.RunOrigin(context.Background(), src, origin, 1, 1)
	assert.NoError(t, err)
	assert.Equal(t, []string{"first", "second"}, order,
		"hooks must be invoked in registration order")
}
