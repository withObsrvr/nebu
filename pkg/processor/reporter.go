package processor

import (
	"context"
	"fmt"
	"os"
)

// Severity describes the operational impact of a reported error.
type Severity int

const (
	// SeverityWarning reports a per-event error. The pipeline continues
	// processing subsequent events. Use this for malformed input, skipped
	// records, or any error that is specific to one event and does not
	// threaten the health of the processor.
	SeverityWarning Severity = iota

	// SeverityFatal reports an unrecoverable error. The runtime will cancel
	// the pipeline and return this error from its Run method. Processors
	// should return from their Process method immediately after calling
	// ReportFatal; continuing to emit events after a fatal report is
	// undefined behavior.
	SeverityFatal
)

// String returns a human-readable name for the severity.
func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "warning"
	case SeverityFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// ErrorReport is a structured error observed during processing.
//
// Reports are observability records, not event replay tools: a report
// carries an opaque EventID hint identifying the failed event (e.g.,
// "ledger 60200432" or "tx abc123"), not a reference to the event
// itself. If you need to capture failed events for reprocessing, write
// a sink that subscribes to warnings and persists them.
type ErrorReport struct {
	// Processor is the name of the reporting processor (Processor.Name()).
	Processor string

	// Severity is the operational impact of the error.
	Severity Severity

	// Err is the underlying error. Wrapping is preserved; downstream
	// reporters can unwrap with errors.Is / errors.As.
	Err error

	// EventID is an optional opaque hint identifying the failed event.
	// Examples: "ledger 60200432", "tx abc123", "batch 47".
	EventID string
}

// Reporter receives structured error reports from processors.
// Implementations must be safe for concurrent use: processors may
// be called from multiple goroutines simultaneously.
type Reporter interface {
	Report(ErrorReport)
}

// ReporterFunc is an adapter that allows using an ordinary function
// as a Reporter. Useful in tests and simple wiring.
type ReporterFunc func(ErrorReport)

// Report implements Reporter by invoking f.
func (f ReporterFunc) Report(r ErrorReport) { f(r) }

// reporterContextKey is an unexported type to prevent collisions
// with other packages that stash values in the same context.
type reporterContextKey struct{}

// WithReporter returns a new context carrying the given reporter.
// The runtime attaches a reporter before calling processor methods;
// tests should do the same.
func WithReporter(ctx context.Context, r Reporter) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, reporterContextKey{}, r)
}

// ReporterFrom returns the reporter attached to ctx, or the package
// default reporter if none is attached.
//
// The default reporter logs warnings to stderr and exits the process
// on fatal reports. This is the right behavior for standalone CLI
// mode: a warning is a diagnostic; a fatal error should kill the
// binary. Runtime-managed pipelines always attach their own reporter
// before calling into processors, so the default is only used when
// a processor runs outside a runtime (or when a caller forgot to
// wire one up).
func ReporterFrom(ctx context.Context) Reporter {
	if r, ok := ctx.Value(reporterContextKey{}).(Reporter); ok {
		return r
	}
	return defaultReporter{}
}

// ReportWarning is a convenience helper that reports a per-event
// error through the reporter attached to ctx. The pipeline continues
// after this call.
func ReportWarning(ctx context.Context, processorName string, err error) {
	ReporterFrom(ctx).Report(ErrorReport{
		Processor: processorName,
		Severity:  SeverityWarning,
		Err:       err,
	})
}

// ReportFatal is a convenience helper that reports an unrecoverable
// error through the reporter attached to ctx. The runtime will halt
// the pipeline after receiving a fatal report. Processors should
// return from their Process method immediately after calling this.
func ReportFatal(ctx context.Context, processorName string, err error) {
	ReporterFrom(ctx).Report(ErrorReport{
		Processor: processorName,
		Severity:  SeverityFatal,
		Err:       err,
	})
}

// defaultExitFunc is the process exit hook used by the default
// reporter on fatal reports. Tests can replace it to verify fatal
// behavior without killing the test process.
var defaultExitFunc = func() { os.Exit(1) }

// defaultReporter logs reports to stderr and exits on fatal reports.
// It is used when ReporterFrom is called with a context that has no
// reporter attached — typically because the processor is running
// standalone rather than inside a runtime.
type defaultReporter struct{}

// Report implements Reporter.
func (defaultReporter) Report(r ErrorReport) {
	prefix := r.Processor
	if prefix == "" {
		prefix = "nebu"
	}
	if r.EventID != "" {
		fmt.Fprintf(os.Stderr, "[%s] %s (%s): %v\n", prefix, r.Severity, r.EventID, r.Err)
	} else {
		fmt.Fprintf(os.Stderr, "[%s] %s: %v\n", prefix, r.Severity, r.Err)
	}
	if r.Severity == SeverityFatal {
		defaultExitFunc()
	}
}
