package runtime

import (
	"context"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/withObsrvr/nebu/pkg/processor"
)

// Hooks is a bundle of optional callbacks the runtime fires at
// specific points in a pipeline's lifecycle. Use [Runtime.Use] to
// register one or more Hooks bundles; each callback runs in the
// order the bundles were added.
//
// Nil callbacks are skipped. A zero-valued Hooks{} is a no-op and
// can be passed for conditional registration.
//
// # Firing order
//
// For a pipeline that processes N ledgers and then terminates
// cleanly, the hooks fire in this order:
//
//  1. OnStart                              (once)
//  2. BeforeLedger → AfterLedger           (N times)
//  3. OnEnd                                (once)
//
// Interleaved with (2), [OnWarning] fires once per
// [processor.ReportWarning] call and [OnFatal] fires once if the
// processor reports a fatal error.
//
// # Abort semantics
//
// Only [Hooks.BeforeLedger] can abort the pipeline: returning a
// non-nil error from BeforeLedger causes [Runtime.RunOrigin] to
// return immediately with that error wrapped in context.
// Observational hooks ([Hooks.AfterLedger], [Hooks.OnWarning],
// [Hooks.OnFatal], [Hooks.OnEnd]) cannot influence control flow —
// they observe decisions the runtime has already made.
//
// If a hook wants to turn a warning into a pipeline-halting fatal,
// it can call [processor.ReportFatal] with the same ctx — the call
// routes back through the same reporter and halts the runtime on
// the next iteration of the main loop.
//
// # Thread safety
//
// Hooks may be called from multiple goroutines during a pipeline
// run; implementations must be safe for concurrent use. In the
// current runtime, all hooks are invoked from the same goroutine as
// [Runtime.RunOrigin], but that may change in future versions —
// treat the guarantee as "the runtime reserves the right to invoke
// hooks concurrently, so code defensively."
type Hooks struct {
	// OnStart fires once before the first ledger is fetched from
	// the source. Use it for per-pipeline setup — allocate metrics
	// counters, start a trace span, open a progress bar, record
	// the start time for custom latency measurements.
	OnStart func(ctx context.Context, info PipelineInfo)

	// BeforeLedger fires just before origin.ProcessLedger is called
	// for each ledger. Returning a non-nil error aborts the pipeline:
	// the error is wrapped ("hook aborted before ledger N: ...") and
	// returned from RunOrigin.
	//
	// Use this for rate limiting, quota enforcement, agent approval
	// loops, or any gating decision that should prevent a specific
	// ledger from being processed. For ledger filtering that doesn't
	// warrant aborting the whole pipeline, skip inside the processor
	// itself rather than here.
	BeforeLedger func(ctx context.Context, ledger xdr.LedgerCloseMeta) error

	// AfterLedger fires just after origin.ProcessLedger returns,
	// carrying timing information about the processing step.
	// Observational only — cannot influence control flow.
	//
	// Typical uses: metrics histograms, span annotations, progress
	// bar ticks, checkpoint persistence on a sequence-number cadence.
	AfterLedger func(ctx context.Context, ledger xdr.LedgerCloseMeta, stats LedgerStats)

	// OnWarning fires for every processor.ReportWarning call made
	// during processing. The pipeline has already continued past
	// the warning by the time this hook runs; observation only.
	OnWarning func(ctx context.Context, report processor.ErrorReport)

	// OnFatal fires for every processor.ReportFatal call. The
	// runtime has already decided to halt the pipeline when this
	// hook fires — observation only. If more than one fatal is
	// reported (unlikely in practice), OnFatal is invoked for each.
	OnFatal func(ctx context.Context, report processor.ErrorReport)

	// OnEnd fires exactly once as the pipeline terminates, for any
	// reason: clean completion, source stream error, fatal processor
	// report, context cancellation, or BeforeLedger abort.
	//
	// OnEnd runs while ctx is still live — hooks that need to make
	// outbound network calls (metrics flush, span export, checkpoint
	// save) can still do so. It runs before the runtime's internal
	// context cancel.
	OnEnd func(ctx context.Context, summary PipelineSummary)
}

// PipelineInfo describes a pipeline at [Hooks.OnStart] time.
type PipelineInfo struct {
	// ProcessorName is the name the origin processor reports via
	// [processor.Processor.Name].
	ProcessorName string

	// StartLedger is the first ledger sequence in the range.
	StartLedger uint32

	// EndLedger is the last ledger sequence in the range, or 0 if
	// the pipeline is running in unbounded (continuous streaming)
	// mode.
	EndLedger uint32
}

// LedgerStats describes a single ledger's processing at
// [Hooks.AfterLedger] time.
type LedgerStats struct {
	// Duration is how long origin.ProcessLedger took to return for
	// this ledger. Includes time spent emitting events to the
	// processor's internal Emitter but not time spent waiting for
	// downstream consumers to drain the emitter.
	Duration time.Duration
}

// PipelineSummary describes the final state of a pipeline at
// [Hooks.OnEnd] time.
type PipelineSummary struct {
	// LedgersProcessed is the total number of ledgers for which
	// origin.ProcessLedger was invoked. Does NOT include ledgers
	// that BeforeLedger aborted or that ctx cancellation skipped.
	LedgersProcessed uint64

	// Warnings is the total number of processor.ReportWarning calls
	// during the pipeline run.
	Warnings int64

	// FatalErr is the error RunOrigin is about to return, or nil
	// for clean completion. Set for fatal processor reports, source
	// stream errors, context.Canceled / context.DeadlineExceeded,
	// and BeforeLedger aborts.
	FatalErr error

	// Duration is the total wall-clock time from RunOrigin entry to
	// OnEnd invocation.
	Duration time.Duration
}
