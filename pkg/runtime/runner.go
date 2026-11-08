// Package runtime provides the core execution engine for nebu pipelines.
package runtime

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/source"
)

// Runtime is the execution engine that wires sources and processors
// together. Pipelines are created by calling [Runtime.RunOrigin];
// observability hooks can be attached via [Runtime.Use] before the
// pipeline starts.
type Runtime struct {
	hooks  []Hooks
	policy Policy
}

// NewRuntime creates a new runtime instance with no hooks registered and the
// best-effort execution policy.
func NewRuntime() *Runtime {
	return &Runtime{}
}

// SetPolicy selects what an unprocessable ledger means for the run. See
// [Policy]. The default is [PolicyBestEffort], which is how nebu behaved
// before policies existed.
//
// SetPolicy must be called before [Runtime.RunOrigin].
func (r *Runtime) SetPolicy(policy Policy) {
	r.policy = policy
}

// Use appends a [Hooks] bundle to the runtime. Multiple Use calls
// stack: each registered bundle is invoked in registration order
// at every firing point, so independent concerns (metrics, tracing,
// progress bars, etc.) can be composed without merging.
//
// Use must be called before [Runtime.RunOrigin]. Registering hooks
// concurrently with a running pipeline is a data race.
func (r *Runtime) Use(h Hooks) {
	r.hooks = append(r.hooks, h)
}

// RunOrigin streams ledgers from a source into an origin processor.
// This is the simplest execution mode — just source → origin.
//
// The function will:
//   - Attach a runtime [processor.Reporter] to the context.
//   - Fire any registered [Hooks.OnStart] callbacks.
//   - Stream ledgers from the source in the range [start, end].
//   - For each ledger: fire [Hooks.BeforeLedger], call
//     origin.ProcessLedger, fire [Hooks.AfterLedger].
//   - Route per-ledger processor warnings through [Hooks.OnWarning]
//     and fatal reports through [Hooks.OnFatal] (streams-never-throw).
//   - Stop and return the first fatal error, source error, context
//     error, or BeforeLedger abort.
//   - Fire [Hooks.OnEnd] with a final [PipelineSummary].
//   - Return nil when the source exhausts its range.
//
// Example usage:
//
//	rt := runtime.NewRuntime()
//	rt.Use(runtime.Hooks{
//	    AfterLedger: func(ctx context.Context, _ xdr.LedgerCloseMeta, s runtime.LedgerStats) {
//	        metrics.ledgerDuration.Observe(s.Duration.Seconds())
//	    },
//	})
//	src, _ := rpc.NewLedgerSource("https://archive-rpc.lightsail.network")
//	origin := &MyOriginProcessor{}
//
//	err := rt.RunOrigin(ctx, src, origin, 100, 200)
func (r *Runtime) RunOrigin(
	ctx context.Context,
	src source.LedgerSource,
	origin processor.Origin,
	start, end uint32,
) (retErr error) {
	// Capture start time up front so OnEnd can report accurate wall
	// clock duration even when the pipeline aborts early.
	runStart := time.Now()
	var ledgersProcessed uint64

	// Install a runtime reporter that observes warnings and fatals
	// from the processor. Fatals halt the pipeline via the fatal
	// channel; warnings are logged and counted but never halt. Both
	// severities also fire any registered Hooks.OnWarning /
	// Hooks.OnFatal callbacks via the reporter.
	//
	// The reporter captures ctx up front so hook callbacks it fires
	// (from inside processor.Report calls) receive a live context.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	reporter := NewReporter(ctx, r.policy, r.hooks)
	ctx = processor.WithReporter(ctx, reporter)

	// OnEnd fires exactly once at pipeline termination, regardless
	// of how we got there. Deferred so it also fires on panic (the
	// hook sees FatalErr=nil but can inspect recover() via ctx if
	// it cares). Runs BEFORE the deferred cancel above because
	// defers are LIFO.
	defer func() {
		summary := PipelineSummary{
			LedgersProcessed: atomic.LoadUint64(&ledgersProcessed),
			Warnings:         reporter.warningCount(),
			Coverage:         reporter.Coverage(),
			FatalErr:         retErr,
			Duration:         time.Since(runStart),
		}
		for _, h := range r.hooks {
			if h.OnEnd != nil {
				h.OnEnd(ctx, summary)
			}
		}
	}()

	// OnStart fires once before any ledgers are fetched.
	for _, h := range r.hooks {
		if h.OnStart != nil {
			h.OnStart(ctx, PipelineInfo{
				ProcessorName: origin.Name(),
				StartLedger:   start,
				EndLedger:     end,
			})
		}
	}

	// Create a channel for ledger streaming. Buffer size of 128
	// gives some breathing room for rate differences.
	ledgerCh := make(chan xdr.LedgerCloseMeta, 128)

	// Source errors channel — surfaced separately from processor
	// errors because a source failure really is fatal to the
	// pipeline.
	sourceErrCh := make(chan error, 1)

	// Start the source streaming in a goroutine.
	go func() {
		err := src.Stream(ctx, start, end, ledgerCh)
		if err != nil && err != context.Canceled {
			sourceErrCh <- fmt.Errorf("source stream error: %w", err)
		}
		close(sourceErrCh)
	}()

	// Process ledgers as they arrive.
	//
	// Once ledgerCh closes, we nil it out so its "always selectable"
	// closed state doesn't starve the other cases. Same for
	// sourceErrCh. The loop terminates cleanly when both channels
	// have been nil'd — meaning we've drained all events and
	// observed the source finishing. Errors, fatals, and context
	// cancellation short-circuit out of the loop as usual.
	for {
		if ledgerCh == nil && sourceErrCh == nil {
			return nil
		}

		// Priority check: if a fatal has been reported (or context
		// has been cancelled), halt immediately without processing
		// any more buffered ledgers. Without this, Go's unbiased
		// select would race fatal-vs-ledgerCh — when the source
		// pre-buffers ledgers ahead of the consumer (which is the
		// common case), several already-queued ledgers could slip
		// through after a fatal report. That's surprising semantics
		// for "this processor has lost its database connection,
		// stop now."
		select {
		case err := <-reporter.Fatal():
			return err
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-reporter.Fatal():
			// Processor reported a fatal error; halt the pipeline.
			return err

		case err, ok := <-sourceErrCh:
			if ok && err != nil {
				return err
			}
			// sourceErrCh closed. Nil it so subsequent iterations
			// stop firing this case — closed channels are always
			// ready in a select and would otherwise monopolize
			// scheduling.
			sourceErrCh = nil

		case ledger, ok := <-ledgerCh:
			if !ok {
				// Ledger channel closed — stream complete. Nil it
				// and keep looping to drain any remaining fatal /
				// source error until sourceErrCh also closes.
				ledgerCh = nil
				continue
			}

			// BeforeLedger hooks — any can abort the pipeline by
			// returning a non-nil error.
			for _, h := range r.hooks {
				if h.BeforeLedger == nil {
					continue
				}
				if err := h.BeforeLedger(ctx, ledger); err != nil {
					return fmt.Errorf("hook aborted before ledger %d: %w",
						ledger.LedgerSequence(), err)
				}
			}

			// Process the ledger. The processor may emit events,
			// report warnings, or report fatal — none of which
			// return through this call. We check the fatal channel
			// on the next loop iteration.
			//
			// BeginLedger first, so any report the processor makes
			// is attributed to this ledger in the run's coverage.
			reporter.BeginLedger(ledger.LedgerSequence())
			ledgerStart := time.Now()
			origin.ProcessLedger(ctx, ledger)
			duration := time.Since(ledgerStart)
			atomic.AddUint64(&ledgersProcessed, 1)

			// AfterLedger hooks — observational.
			if len(r.hooks) > 0 {
				stats := LedgerStats{Duration: duration}
				for _, h := range r.hooks {
					if h.AfterLedger != nil {
						h.AfterLedger(ctx, ledger, stats)
					}
				}
			}
		}
	}
}
