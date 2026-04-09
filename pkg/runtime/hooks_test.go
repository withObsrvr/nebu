package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/withObsrvr/nebu/pkg/processor"
)

// fakeSource is an in-memory LedgerSource that emits a fixed sequence
// of dummy ledgers, then closes the output channel. It does NOT hit
// real Stellar RPC, so tests stay fast and deterministic.
type fakeSource struct {
	// streamErr, if non-nil, is returned from Stream after emitting
	// len(ledgers) events. Use to simulate source failures.
	streamErr error
}

func (s *fakeSource) Stream(ctx context.Context, start, end uint32, out chan<- xdr.LedgerCloseMeta) error {
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
	return s.streamErr
}

func (s *fakeSource) Close() error { return nil }

// recordingOrigin is a minimal processor.Origin implementation that
// records every ledger it receives. The onProcess callback lets
// individual tests inject behavior (report warnings, report fatals,
// sleep to make duration observable, etc.).
type recordingOrigin struct {
	name       string
	mu         sync.Mutex
	seen       []uint32
	onProcess  func(ctx context.Context, ledger xdr.LedgerCloseMeta)
}

func (o *recordingOrigin) Name() string         { return o.name }
func (o *recordingOrigin) Type() processor.Type { return processor.TypeOrigin }
func (o *recordingOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
	o.mu.Lock()
	o.seen = append(o.seen, ledger.LedgerSequence())
	o.mu.Unlock()
	if o.onProcess != nil {
		o.onProcess(ctx, ledger)
	}
}

// ---- Test cases ----

func TestHooks_OnStartAndOnEndFireOnce(t *testing.T) {
	var startCount, endCount int32
	var gotInfo PipelineInfo
	var gotSummary PipelineSummary

	rt := NewRuntime()
	rt.Use(Hooks{
		OnStart: func(ctx context.Context, info PipelineInfo) {
			atomic.AddInt32(&startCount, 1)
			gotInfo = info
		},
		OnEnd: func(ctx context.Context, summary PipelineSummary) {
			atomic.AddInt32(&endCount, 1)
			gotSummary = summary
		},
	})

	origin := &recordingOrigin{name: "test-origin"}
	err := rt.RunOrigin(context.Background(), &fakeSource{}, origin, 100, 103)
	require.NoError(t, err)

	assert.Equal(t, int32(1), atomic.LoadInt32(&startCount))
	assert.Equal(t, int32(1), atomic.LoadInt32(&endCount))
	assert.Equal(t, "test-origin", gotInfo.ProcessorName)
	assert.Equal(t, uint32(100), gotInfo.StartLedger)
	assert.Equal(t, uint32(103), gotInfo.EndLedger)
	assert.Equal(t, uint64(4), gotSummary.LedgersProcessed)
	assert.NoError(t, gotSummary.FatalErr)
	assert.Greater(t, gotSummary.Duration, time.Duration(0))
}

func TestHooks_BeforeAndAfterLedgerFirePerLedger(t *testing.T) {
	var beforeSeqs, afterSeqs []uint32
	var mu sync.Mutex

	rt := NewRuntime()
	rt.Use(Hooks{
		BeforeLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
			mu.Lock()
			defer mu.Unlock()
			beforeSeqs = append(beforeSeqs, ledger.LedgerSequence())
			return nil
		},
		AfterLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta, stats LedgerStats) {
			mu.Lock()
			defer mu.Unlock()
			afterSeqs = append(afterSeqs, ledger.LedgerSequence())
			assert.GreaterOrEqual(t, stats.Duration, time.Duration(0))
		},
	})

	origin := &recordingOrigin{name: "p"}
	err := rt.RunOrigin(context.Background(), &fakeSource{}, origin, 10, 12)
	require.NoError(t, err)

	assert.Equal(t, []uint32{10, 11, 12}, beforeSeqs)
	assert.Equal(t, []uint32{10, 11, 12}, afterSeqs)
}

func TestHooks_BeforeLedgerErrorAbortsPipeline(t *testing.T) {
	wantErr := errors.New("quota exceeded")
	var processedAfterAbort int32

	rt := NewRuntime()
	rt.Use(Hooks{
		BeforeLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
			// Abort on the second ledger.
			if ledger.LedgerSequence() == 11 {
				return wantErr
			}
			return nil
		},
		AfterLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta, stats LedgerStats) {
			if ledger.LedgerSequence() >= 11 {
				atomic.AddInt32(&processedAfterAbort, 1)
			}
		},
	})

	origin := &recordingOrigin{name: "p"}
	err := rt.RunOrigin(context.Background(), &fakeSource{}, origin, 10, 15)

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr, "error chain should include the hook error")
	assert.Contains(t, err.Error(), "hook aborted before ledger 11")

	// Only ledger 10 was processed; 11 was aborted, 12+ never reached.
	assert.Equal(t, []uint32{10}, origin.seen)
	assert.Equal(t, int32(0), atomic.LoadInt32(&processedAfterAbort),
		"AfterLedger should not fire for the aborted ledger or anything after it")
}

func TestHooks_OnEndFiresOnAbort(t *testing.T) {
	var summary PipelineSummary
	endFired := false

	rt := NewRuntime()
	rt.Use(Hooks{
		BeforeLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
			if ledger.LedgerSequence() == 11 {
				return errors.New("nope")
			}
			return nil
		},
		OnEnd: func(ctx context.Context, s PipelineSummary) {
			endFired = true
			summary = s
		},
	})

	origin := &recordingOrigin{name: "p"}
	_ = rt.RunOrigin(context.Background(), &fakeSource{}, origin, 10, 15)

	assert.True(t, endFired, "OnEnd must fire even when pipeline aborts")
	assert.Error(t, summary.FatalErr, "FatalErr must reflect the abort reason")
	assert.Equal(t, uint64(1), summary.LedgersProcessed, "only ledger 10 was processed")
}

func TestHooks_OnWarningFiresOnProcessorReport(t *testing.T) {
	var warnings []processor.ErrorReport
	var mu sync.Mutex

	rt := NewRuntime()
	rt.Use(Hooks{
		OnWarning: func(ctx context.Context, r processor.ErrorReport) {
			mu.Lock()
			defer mu.Unlock()
			warnings = append(warnings, r)
		},
	})

	origin := &recordingOrigin{
		name: "warn-origin",
		onProcess: func(ctx context.Context, ledger xdr.LedgerCloseMeta) {
			processor.ReportWarning(ctx, "warn-origin",
				errors.New("bad field"))
		},
	}

	err := rt.RunOrigin(context.Background(), &fakeSource{}, origin, 20, 22)
	require.NoError(t, err)

	assert.Len(t, warnings, 3)
	for _, w := range warnings {
		assert.Equal(t, processor.SeverityWarning, w.Severity)
		assert.Equal(t, "warn-origin", w.Processor)
	}
}

func TestHooks_OnFatalFiresAndHaltsPipeline(t *testing.T) {
	var fatals []processor.ErrorReport
	rt := NewRuntime()
	rt.Use(Hooks{
		OnFatal: func(ctx context.Context, r processor.ErrorReport) {
			fatals = append(fatals, r)
		},
	})

	origin := &recordingOrigin{
		name: "fatal-origin",
		onProcess: func(ctx context.Context, ledger xdr.LedgerCloseMeta) {
			if ledger.LedgerSequence() == 31 {
				processor.ReportFatal(ctx, "fatal-origin",
					errors.New("db dropped"))
			}
		},
	}

	err := rt.RunOrigin(context.Background(), &fakeSource{}, origin, 30, 35)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db dropped")

	assert.Len(t, fatals, 1)
	assert.Equal(t, processor.SeverityFatal, fatals[0].Severity)

	// Exactly 2 ledgers were processed: 30 (clean) and 31 (which
	// reported the fatal). The runtime's fatal-priority check halts
	// the loop before any further buffered ledgers can be drained.
	assert.Equal(t, []uint32{30, 31}, origin.seen,
		"pipeline must halt deterministically after the fatal ledger")
}

func TestHooks_SourceErrorFiresOnEndWithFatalErr(t *testing.T) {
	var summary PipelineSummary
	rt := NewRuntime()
	rt.Use(Hooks{
		OnEnd: func(ctx context.Context, s PipelineSummary) { summary = s },
	})

	src := &fakeSource{streamErr: errors.New("rpc timeout")}
	origin := &recordingOrigin{name: "p"}
	err := rt.RunOrigin(context.Background(), src, origin, 100, 101)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rpc timeout")
	assert.Error(t, summary.FatalErr)
	assert.Contains(t, summary.FatalErr.Error(), "rpc timeout")
}

func TestHooks_ContextCancelFiresOnEndWithCtxErr(t *testing.T) {
	var summary PipelineSummary
	endFired := make(chan struct{})
	rt := NewRuntime()
	rt.Use(Hooks{
		OnEnd: func(ctx context.Context, s PipelineSummary) {
			summary = s
			close(endFired)
		},
		BeforeLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
			// Slow down enough that ctx cancel has a chance to fire
			// mid-pipeline.
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	origin := &recordingOrigin{name: "p"}
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()

	err := rt.RunOrigin(ctx, &fakeSource{}, origin, 1, 1000)
	require.ErrorIs(t, err, context.Canceled)

	select {
	case <-endFired:
	case <-time.After(time.Second):
		t.Fatal("OnEnd did not fire within timeout")
	}
	assert.ErrorIs(t, summary.FatalErr, context.Canceled)
}

func TestHooks_MultipleUseCallsCompose(t *testing.T) {
	// Two independent hook bundles — verify both fire on each event
	// in registration order.
	var order []string
	var mu sync.Mutex
	record := func(name string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, name)
	}

	rt := NewRuntime()
	rt.Use(Hooks{
		OnStart: func(ctx context.Context, _ PipelineInfo) { record("hook1:start") },
		OnEnd:   func(ctx context.Context, _ PipelineSummary) { record("hook1:end") },
	})
	rt.Use(Hooks{
		OnStart: func(ctx context.Context, _ PipelineInfo) { record("hook2:start") },
		OnEnd:   func(ctx context.Context, _ PipelineSummary) { record("hook2:end") },
	})

	origin := &recordingOrigin{name: "p"}
	err := rt.RunOrigin(context.Background(), &fakeSource{}, origin, 1, 1)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"hook1:start", "hook2:start",
		"hook1:end", "hook2:end",
	}, order)
}

func TestHooks_ZeroValueBundleIsNoop(t *testing.T) {
	// Registering a zero-value Hooks{} must not panic at any hook
	// firing point.
	rt := NewRuntime()
	rt.Use(Hooks{})

	origin := &recordingOrigin{
		name: "p",
		onProcess: func(ctx context.Context, ledger xdr.LedgerCloseMeta) {
			processor.ReportWarning(ctx, "p", errors.New("ignore me"))
		},
	}
	err := rt.RunOrigin(context.Background(), &fakeSource{}, origin, 1, 3)
	require.NoError(t, err)
	assert.Equal(t, []uint32{1, 2, 3}, origin.seen)
}

func TestHooks_NoHooksRegisteredStillWorks(t *testing.T) {
	// Sanity check: a runtime with zero hooks must still run
	// pipelines correctly — this validates we didn't introduce a
	// nil-slice regression in RunOrigin.
	rt := NewRuntime()
	origin := &recordingOrigin{name: "p"}
	err := rt.RunOrigin(context.Background(), &fakeSource{}, origin, 50, 52)
	require.NoError(t, err)
	assert.Equal(t, []uint32{50, 51, 52}, origin.seen)
}

func TestHooks_PipelineSummaryCountsWarnings(t *testing.T) {
	var summary PipelineSummary
	rt := NewRuntime()
	rt.Use(Hooks{
		OnEnd: func(ctx context.Context, s PipelineSummary) { summary = s },
	})

	origin := &recordingOrigin{
		name: "p",
		onProcess: func(ctx context.Context, ledger xdr.LedgerCloseMeta) {
			// Emit 2 warnings per ledger.
			processor.ReportWarning(ctx, "p", errors.New("w1"))
			processor.ReportWarning(ctx, "p", errors.New("w2"))
		},
	}

	err := rt.RunOrigin(context.Background(), &fakeSource{}, origin, 1, 5)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), summary.LedgersProcessed)
	assert.Equal(t, int64(10), summary.Warnings)
}
