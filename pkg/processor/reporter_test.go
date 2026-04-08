package processor

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeverity_String(t *testing.T) {
	assert.Equal(t, "warning", SeverityWarning.String())
	assert.Equal(t, "fatal", SeverityFatal.String())
	assert.Equal(t, "unknown", Severity(99).String())
}

func TestReporterFunc_Report(t *testing.T) {
	var got ErrorReport
	r := ReporterFunc(func(rep ErrorReport) { got = rep })

	r.Report(ErrorReport{
		Processor: "test",
		Severity:  SeverityWarning,
		Err:       errors.New("boom"),
		EventID:   "ledger 1",
	})

	assert.Equal(t, "test", got.Processor)
	assert.Equal(t, SeverityWarning, got.Severity)
	assert.Equal(t, "boom", got.Err.Error())
	assert.Equal(t, "ledger 1", got.EventID)
}

func TestWithReporter_Roundtrip(t *testing.T) {
	sentinel := ReporterFunc(func(ErrorReport) {})
	ctx := WithReporter(context.Background(), sentinel)

	got := ReporterFrom(ctx)
	// The wrapped ReporterFunc value can't be compared with ==, so check
	// by invoking and observing the side effect.
	called := false
	r := ReporterFunc(func(ErrorReport) { called = true })
	ctx = WithReporter(context.Background(), r)
	ReporterFrom(ctx).Report(ErrorReport{})
	assert.True(t, called)
	_ = got // silence unused warning for the sentinel check above
}

func TestWithReporter_NilIsIgnored(t *testing.T) {
	// Passing a nil reporter should not wrap the context; subsequent
	// ReporterFrom calls should return the default.
	ctx := WithReporter(context.Background(), nil)
	r := ReporterFrom(ctx)
	_, ok := r.(defaultReporter)
	assert.True(t, ok, "nil reporter should fall through to default")
}

func TestReporterFrom_DefaultWhenMissing(t *testing.T) {
	r := ReporterFrom(context.Background())
	_, ok := r.(defaultReporter)
	assert.True(t, ok, "missing reporter should return default")
}

func TestReportWarning_RoutesThroughContext(t *testing.T) {
	var mu sync.Mutex
	var reports []ErrorReport

	r := ReporterFunc(func(rep ErrorReport) {
		mu.Lock()
		defer mu.Unlock()
		reports = append(reports, rep)
	})
	ctx := WithReporter(context.Background(), r)

	ReportWarning(ctx, "proc-1", errors.New("bad event"))

	require.Len(t, reports, 1)
	assert.Equal(t, "proc-1", reports[0].Processor)
	assert.Equal(t, SeverityWarning, reports[0].Severity)
	assert.Equal(t, "bad event", reports[0].Err.Error())
}

func TestReportFatal_RoutesThroughContext(t *testing.T) {
	var got ErrorReport
	r := ReporterFunc(func(rep ErrorReport) { got = rep })
	ctx := WithReporter(context.Background(), r)

	ReportFatal(ctx, "proc-2", errors.New("db down"))

	assert.Equal(t, "proc-2", got.Processor)
	assert.Equal(t, SeverityFatal, got.Severity)
	assert.Equal(t, "db down", got.Err.Error())
}

func TestDefaultReporter_WarningDoesNotExit(t *testing.T) {
	// Swap the exit hook so we can verify it's not called on warnings.
	orig := defaultExitFunc
	defer func() { defaultExitFunc = orig }()

	exited := false
	defaultExitFunc = func() { exited = true }

	defaultReporter{}.Report(ErrorReport{
		Processor: "p",
		Severity:  SeverityWarning,
		Err:       errors.New("warn"),
	})
	assert.False(t, exited, "warning must not trigger exit")
}

func TestDefaultReporter_FatalCallsExitHook(t *testing.T) {
	orig := defaultExitFunc
	defer func() { defaultExitFunc = orig }()

	exited := false
	defaultExitFunc = func() { exited = true }

	defaultReporter{}.Report(ErrorReport{
		Processor: "p",
		Severity:  SeverityFatal,
		Err:       errors.New("boom"),
	})
	assert.True(t, exited, "fatal must trigger exit")
}

func TestDefaultReporter_UsedWhenContextHasNone(t *testing.T) {
	// ReportWarning on a bare context should go to the default reporter,
	// which should NOT exit. This is the test-safety guarantee.
	orig := defaultExitFunc
	defer func() { defaultExitFunc = orig }()

	exited := false
	defaultExitFunc = func() { exited = true }

	ReportWarning(context.Background(), "p", errors.New("x"))
	assert.False(t, exited)
}
