package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/withObsrvr/nebu/pkg/runtime"
)

// progressHook is a self-contained nebu runtime.Hooks bundle that
// shows live ledger-processing progress on stderr. It's a reference
// example of how to wire observability into a pipeline via the
// OriginConfig.Hooks field, without modifying the processor or the
// runtime.
//
// The hook prints to stderr at most once per second (or every 100
// ledgers, whichever comes first) so high-throughput pipelines don't
// drown the output. stdout (the JSON event stream) is untouched, so
// piping to jq / duckdb / sinks works normally.
//
// Output is automatically suppressed when stderr is not a terminal —
// e.g., when redirected to a log file or /dev/null — so the hook is
// a no-op in non-interactive contexts.
//
// Bounded pipelines (--end-ledger > 0) show a percentage and ETA;
// unbounded pipelines (continuous streaming) show a spinner-style
// "ledgers processed" counter with elapsed time.
//
// Copy this file into your own processor's cmd/<name>/ directory
// and tweak the print format to taste — that's the whole pattern.
func progressHook() runtime.Hooks {
	if !isStderrTerminal() {
		// Returning a zero-value Hooks{} is a no-op — every callback
		// is nil. The runtime's hook iteration skips nil functions.
		return runtime.Hooks{}
	}

	var (
		startedAt time.Time
		startSeq  uint32
		total     uint32
		lastPrint time.Time
	)

	return runtime.Hooks{
		OnStart: func(ctx context.Context, info runtime.PipelineInfo) {
			startedAt = time.Now()
			startSeq = info.StartLedger
			if info.EndLedger > 0 {
				total = info.EndLedger - info.StartLedger + 1
			}
		},

		AfterLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta, _ runtime.LedgerStats) {
			seq := ledger.LedgerSequence()
			done := seq - startSeq + 1

			// Throttle: print every 100 ledgers OR at most once per
			// second, whichever comes first. Avoids drowning stderr
			// on high-throughput backfills.
			if done%100 != 0 && time.Since(lastPrint) < time.Second {
				return
			}
			lastPrint = time.Now()

			elapsed := time.Since(startedAt)
			rate := float64(done) / elapsed.Seconds()
			if total > 0 {
				pct := float64(done) / float64(total) * 100
				var eta time.Duration
				if rate > 0 {
					eta = time.Duration(float64(total-done)/rate) * time.Second
				}
				fmt.Fprintf(os.Stderr, "\r[%6.2f%%] %d/%d ledgers, %.0f/sec, eta %s     ",
					pct, done, total, rate, eta.Round(time.Second))
			} else {
				fmt.Fprintf(os.Stderr, "\r[stream] %d ledgers, %.0f/sec, %s elapsed     ",
					done, rate, elapsed.Round(time.Second))
			}
		},

		OnEnd: func(ctx context.Context, s runtime.PipelineSummary) {
			fmt.Fprintf(os.Stderr, "\n[done] %d ledgers in %s",
				s.LedgersProcessed, s.Duration.Round(time.Millisecond))
			if s.Warnings > 0 {
				fmt.Fprintf(os.Stderr, " (%d warnings)", s.Warnings)
			}
			if s.FatalErr != nil {
				fmt.Fprintf(os.Stderr, " — fatal: %v", s.FatalErr)
			}
			fmt.Fprintln(os.Stderr)
		},
	}
}

// isStderrTerminal reports whether stderr is connected to a terminal.
// Used to suppress progress output when stderr is piped to a file.
func isStderrTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
