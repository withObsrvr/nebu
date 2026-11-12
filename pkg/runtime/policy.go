package runtime

import (
	"context"
	"fmt"
	"os"
	"sync"

	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/processor"
)

// Policy decides what an unprocessable ledger means for the run as a whole.
//
// "Streams never throw" settles who reports a problem: the processor, through
// the [processor.Reporter]. It deliberately does not settle whether a run
// that hit problems produced usable output — that depends on what the output
// is for. A warning on three ledgers out of ten million is noise for an
// exploratory query and disqualifying for a reconciliation dataset, and the
// processor author cannot know which one the operator is doing. Policy moves
// that decision to the invocation.
type Policy int

const (
	// PolicyBestEffort reports each problem and carries on. The run can
	// still finish successfully; [Coverage] records what was missed so the
	// gaps are visible rather than implied.
	PolicyBestEffort Policy = iota

	// PolicyStrict halts the run the first time a ledger cannot be
	// processed completely, so a partial dataset is never presented as a
	// complete one. The command exits with the "incomplete" code.
	PolicyStrict
)

// String returns the flag spelling of the policy.
func (p Policy) String() string {
	switch p {
	case PolicyBestEffort:
		return "best-effort"
	case PolicyStrict:
		return "strict"
	default:
		return "unknown"
	}
}

// ParsePolicy maps a flag value to a [Policy]. An empty value is
// [PolicyBestEffort], which is the behavior nebu had before policies existed.
func ParsePolicy(value string) (Policy, error) {
	switch value {
	case "", "best-effort":
		return PolicyBestEffort, nil
	case "strict":
		return PolicyStrict, nil
	default:
		return PolicyBestEffort, fmt.Errorf(
			"unknown ledger-error policy %q (want %q or %q)",
			value, PolicyBestEffort, PolicyStrict)
	}
}

// Coverage describes how completely a run covered the input it was given.
//
// A warning count alone cannot answer "is this dataset whole?" — it does not
// say how many ledgers were affected, or which. Coverage does, so a
// best-effort run is auditable instead of merely finished.
type Coverage struct {
	// Ledgers is the number of ledgers handed to the processor. It is
	// supplied by the caller rather than counted here: the reporter learns
	// which ledger is in flight before the processor is handed it, so a
	// ledger that something aborted before processing would otherwise be
	// counted as covered.
	Ledgers uint64

	// IncompleteLedgers is the number of those ledgers that reported at
	// least one problem. It counts ledgers, not reports: ten warnings on
	// one ledger is one incomplete ledger.
	IncompleteLedgers uint64

	// FirstIncompleteLedger is the lowest ledger sequence that reported a
	// problem, or 0 when there was none. It is the sequence to replay.
	FirstIncompleteLedger uint32

	// Warnings is the total number of warning reports.
	Warnings int64
}

// Complete reports whether every ledger handed to the processor was processed
// without a reported problem.
func (c Coverage) Complete() bool { return c.IncompleteLedgers == 0 }

// String renders coverage for an operator reading stderr.
func (c Coverage) String() string {
	if c.Complete() {
		return fmt.Sprintf("%d ledgers, complete", c.Ledgers)
	}
	return fmt.Sprintf("%d ledgers, %d incomplete (first: %d), %d warnings",
		c.Ledgers, c.IncompleteLedgers, c.FirstIncompleteLedger, c.Warnings)
}

// Reporter is the [processor.Reporter] nebu attaches to the context before
// calling into a processor. It logs reports to stderr, counts them, fires any
// registered [Hooks], tracks [Coverage], and applies the [Policy].
//
// It is exported because every execution path has to attach one. A processor
// run from a pipe gets its ledgers from stdin rather than from a
// [Runtime.RunOrigin] loop, and without a reporter of its own that path falls
// through to the package default, which counts nothing and exits the process
// on a fatal report — so warnings were invisible and policy unenforceable in
// exactly the mode Unix composition uses.
//
// Safe for concurrent use.
type Reporter struct {
	ctx    context.Context
	policy Policy
	hooks  []Hooks

	// fatal carries the first halting error to a runner's select loop. It
	// is buffered with room for one: the first error is the one that
	// stopped the run.
	fatal chan error

	mu                    sync.Mutex
	err                   error
	warnings              int64
	incompleteLedgers     uint64
	firstIncompleteLedger uint32
	currentLedger         uint32
	currentLedgerFlagged  bool
}

// NewReporter creates a reporter that applies policy and fires hooks.
//
// ctx is captured for hook invocation, matching the lifetime of the pipeline
// the reporter belongs to.
func NewReporter(ctx context.Context, policy Policy, hooks []Hooks) *Reporter {
	return &Reporter{
		ctx:    ctx,
		policy: policy,
		hooks:  hooks,
		fatal:  make(chan error, 1),
	}
}

// BeginLedger tells the reporter which ledger reports should now be
// attributed to.
//
// Call it before anything that can report — which means before the hooks and
// callbacks that run ahead of the processor, not just before the processor
// itself. A warning raised before the attribution moves lands on the previous
// ledger, which both mislabels the gap and inflates the count of incomplete
// ledgers.
//
// It does not count the ledger: see [Coverage.Ledgers]. Callers that never
// call BeginLedger still get working reports and warning counts; they just
// get no per-ledger attribution.
func (r *Reporter) BeginLedger(sequence uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.currentLedger = sequence
	r.currentLedgerFlagged = false
}

// Fatal returns the channel a runner selects on to learn that the pipeline
// must stop. Under [PolicyStrict] a warning arrives here too.
func (r *Reporter) Fatal() <-chan error { return r.fatal }

// Err returns the error that halted the run, or nil. Paths without a select
// loop poll this between ledgers.
func (r *Reporter) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// Coverage returns the coverage accumulated so far, over the number of
// ledgers the caller actually handed to the processor.
func (r *Reporter) Coverage(ledgersProcessed uint64) Coverage {
	r.mu.Lock()
	defer r.mu.Unlock()

	return Coverage{
		Ledgers:               ledgersProcessed,
		IncompleteLedgers:     r.incompleteLedgers,
		FirstIncompleteLedger: r.firstIncompleteLedger,
		Warnings:              r.warnings,
	}
}

// Report implements [processor.Reporter].
func (r *Reporter) Report(report processor.ErrorReport) {
	name := report.Processor
	if name == "" {
		name = "nebu"
	}

	switch report.Severity {
	case processor.SeverityWarning:
		sequence, strict := r.recordWarning()
		logReport(name, "warning", report)
		for _, hook := range r.hooks {
			if hook.OnWarning != nil {
				hook.OnWarning(r.ctx, report)
			}
		}
		if strict {
			// Under strict policy the warning IS the halting condition:
			// the processor could not handle this ledger completely, and
			// the operator asked for that to end the run.
			r.halt(nebuErrors.IncompleteRun(sequence, report.Err))
		}

	case processor.SeverityFatal:
		// A fatal stops the processor mid-ledger, so that ledger is
		// incomplete too. Without this the run dies partway through a
		// ledger and still reports complete coverage, which is the one
		// thing coverage exists to prevent. It is not a warning, so the
		// warning count and the fatal exit classification are untouched.
		r.markCurrentLedgerIncomplete()
		logReport(name, "fatal", report)
		r.halt(fmt.Errorf("%s: %w", name, report.Err))
		for _, hook := range r.hooks {
			if hook.OnFatal != nil {
				hook.OnFatal(r.ctx, report)
			}
		}
	}
}

// recordWarning counts a warning against the ledger in flight and reports
// the ledger sequence along with whether the policy requires a halt.
func (r *Reporter) recordWarning() (uint32, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.warnings++
	r.flagCurrentLedgerLocked()
	return r.currentLedger, r.policy == PolicyStrict
}

// markCurrentLedgerIncomplete flags the ledger in flight without counting a
// warning against it.
func (r *Reporter) markCurrentLedgerIncomplete() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flagCurrentLedgerLocked()
}

// flagCurrentLedgerLocked counts the ledger in flight as incomplete, once.
// Ten reports on one ledger is one hole in the output, not ten.
//
// Callers must hold r.mu.
func (r *Reporter) flagCurrentLedgerLocked() {
	if r.currentLedgerFlagged {
		return
	}
	r.currentLedgerFlagged = true
	r.incompleteLedgers++
	if r.firstIncompleteLedger == 0 {
		r.firstIncompleteLedger = r.currentLedger
	}
}

// halt records the first halting error and signals it. Later calls are
// dropped: the first error is the one that explains the run's outcome.
//
// Recording and signalling happen under the same lock on purpose. Releasing
// between them lets a second caller win the channel while the first wins the
// field, so Err() and Fatal() would hand the runtime different causes — and
// different exit classifications. Holding the lock across a non-blocking send
// on a buffered channel cannot block.
func (r *Reporter) halt(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.err != nil {
		return
	}
	r.err = err

	select {
	case r.fatal <- err:
	default:
	}
}

func logReport(name, severity string, report processor.ErrorReport) {
	if report.EventID != "" {
		fmt.Fprintf(os.Stderr, "[%s] %s (%s): %v\n", name, severity, report.EventID, report.Err)
		return
	}
	fmt.Fprintf(os.Stderr, "[%s] %s: %v\n", name, severity, report.Err)
}

// warningCount exists so the runner can report warnings without taking the
// reporter's lock on a path that already holds its own counters.
func (r *Reporter) warningCount() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.warnings
}

// assert at compile time that Reporter satisfies the processor contract.
var _ processor.Reporter = (*Reporter)(nil)
