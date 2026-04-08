// Package runtime provides the core execution engine for nebu pipelines.
package runtime

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/source"
)

// Runtime is the execution engine that wires sources and processors together.
type Runtime struct {
	// Future: add logger, metrics, config options
}

// NewRuntime creates a new runtime instance.
func NewRuntime() *Runtime {
	return &Runtime{}
}

// RunOrigin streams ledgers from a source into an origin processor.
// This is the simplest execution mode — just source → origin.
//
// The function will:
//   - Attach a runtime [processor.Reporter] to the context.
//   - Stream ledgers from the source in the range [start, end].
//   - Call origin.ProcessLedger for each ledger. Per-ledger errors
//     the processor reports as warnings are logged but do not halt
//     the pipeline (streams-never-throw).
//   - Stop and return the first fatal error reported by the processor,
//     or the first source error, or the context error.
//   - Return nil when the source exhausts its range.
//
// Example usage:
//
//	rt := runtime.NewRuntime()
//	src, _ := rpc.NewLedgerSource("https://archive-rpc.lightsail.network")
//	origin := &MyOriginProcessor{}
//
//	err := rt.RunOrigin(ctx, src, origin, 100, 200)
func (r *Runtime) RunOrigin(
	ctx context.Context,
	src source.LedgerSource,
	origin processor.Origin,
	start, end uint32,
) error {
	// Install a runtime reporter that observes warnings and fatals from
	// the processor. Fatals halt the pipeline via the fatal channel;
	// warnings are logged and counted but never halt.
	reporter := newRuntimeReporter()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx = processor.WithReporter(ctx, reporter)

	// Create a channel for ledger streaming. Buffer size of 128 gives
	// some breathing room for rate differences.
	ledgerCh := make(chan xdr.LedgerCloseMeta, 128)

	// Source errors channel — surfaced separately from processor errors
	// because a source failure really is fatal to the pipeline.
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
	// closed state doesn't starve the other cases. Same for sourceErrCh.
	// The loop terminates cleanly when both channels have been nil'd —
	// meaning we've drained all events and observed the source finishing.
	// Errors, fatals, and context cancellation short-circuit out of the
	// loop as usual.
	for {
		if ledgerCh == nil && sourceErrCh == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-reporter.fatal:
			// Processor reported a fatal error; halt the pipeline.
			return err

		case err, ok := <-sourceErrCh:
			if ok && err != nil {
				return err
			}
			// sourceErrCh closed. Nil it so subsequent iterations stop
			// firing this case — closed channels are always ready in a
			// select and would otherwise monopolize scheduling.
			sourceErrCh = nil

		case ledger, ok := <-ledgerCh:
			if !ok {
				// Ledger channel closed — stream complete. Nil it and
				// keep looping to drain any remaining fatal / source
				// error until sourceErrCh also closes.
				ledgerCh = nil
				continue
			}

			// Process the ledger. The processor may emit events, report
			// warnings, or report fatal — none of which return through
			// this call. We check the fatal channel on the next loop
			// iteration.
			origin.ProcessLedger(ctx, ledger)
		}
	}
}

// runtimeReporter is the Reporter that nebu's runtime attaches to the
// context before calling processor methods. It captures the first
// fatal report through a buffered channel (subsequent fatals are
// dropped since only the first matters) and logs warnings to stderr
// while counting them for metrics.
type runtimeReporter struct {
	fatal    chan error // buffered, size 1
	warnings int64      // atomic
}

func newRuntimeReporter() *runtimeReporter {
	return &runtimeReporter{
		fatal: make(chan error, 1),
	}
}

// Report implements processor.Reporter.
func (r *runtimeReporter) Report(report processor.ErrorReport) {
	prefix := report.Processor
	if prefix == "" {
		prefix = "nebu"
	}

	switch report.Severity {
	case processor.SeverityWarning:
		atomic.AddInt64(&r.warnings, 1)
		if report.EventID != "" {
			fmt.Fprintf(os.Stderr, "[%s] warning (%s): %v\n", prefix, report.EventID, report.Err)
		} else {
			fmt.Fprintf(os.Stderr, "[%s] warning: %v\n", prefix, report.Err)
		}

	case processor.SeverityFatal:
		if report.EventID != "" {
			fmt.Fprintf(os.Stderr, "[%s] fatal (%s): %v\n", prefix, report.EventID, report.Err)
		} else {
			fmt.Fprintf(os.Stderr, "[%s] fatal: %v\n", prefix, report.Err)
		}
		// Non-blocking send: first fatal wins, later fatals are dropped.
		select {
		case r.fatal <- fmt.Errorf("%s: %w", prefix, report.Err):
		default:
		}
	}
}
