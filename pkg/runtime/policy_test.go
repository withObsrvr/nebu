package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/processor"
)

func TestParsePolicy(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    Policy
		wantErr bool
	}{
		// An empty value is the pre-policy behavior, so adding the flag
		// cannot change what an existing invocation does.
		{name: "empty defaults to best effort", value: "", want: PolicyBestEffort},
		{name: "best-effort", value: "best-effort", want: PolicyBestEffort},
		{name: "strict", value: "strict", want: PolicyStrict},
		// Not silently defaulted: the flag exists to make the completeness
		// guarantee explicit, so a typo must not quietly weaken it.
		{name: "unknown is rejected", value: "lenient", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePolicy(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorContains(t, err, "lenient")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPolicyStringRoundTrips(t *testing.T) {
	for _, policy := range []Policy{PolicyBestEffort, PolicyStrict} {
		parsed, err := ParsePolicy(policy.String())
		require.NoError(t, err)
		assert.Equal(t, policy, parsed, "%s must parse back to itself", policy)
	}
}

func warning(name string, err error) processor.ErrorReport {
	return processor.ErrorReport{Processor: name, Severity: processor.SeverityWarning, Err: err}
}

func TestReporterBestEffortCountsWithoutHalting(t *testing.T) {
	reporter := NewReporter(context.Background(), PolicyBestEffort, nil)

	reporter.BeginLedger(10)
	reporter.Report(warning("p", errors.New("bad field")))
	reporter.Report(warning("p", errors.New("another bad field")))
	reporter.BeginLedger(11)
	reporter.BeginLedger(12)
	reporter.Report(warning("p", errors.New("bad field")))

	assert.NoError(t, reporter.Err(), "best effort must not halt the run")

	coverage := reporter.Coverage()
	assert.Equal(t, uint64(3), coverage.Ledgers)
	// Two warnings on ledger 10 is one incomplete ledger, not two: the
	// question coverage answers is how much of the input is missing.
	assert.Equal(t, uint64(2), coverage.IncompleteLedgers)
	assert.Equal(t, uint32(10), coverage.FirstIncompleteLedger)
	assert.Equal(t, int64(3), coverage.Warnings)
	assert.False(t, coverage.Complete())
}

func TestReporterCleanRunIsComplete(t *testing.T) {
	reporter := NewReporter(context.Background(), PolicyStrict, nil)

	reporter.BeginLedger(10)
	reporter.BeginLedger(11)

	coverage := reporter.Coverage()
	assert.True(t, coverage.Complete())
	assert.Equal(t, uint64(2), coverage.Ledgers)
	assert.Zero(t, coverage.FirstIncompleteLedger)
	assert.Contains(t, coverage.String(), "complete")
	assert.NoError(t, reporter.Err())
}

func TestReporterStrictHaltsOnFirstWarning(t *testing.T) {
	reporter := NewReporter(context.Background(), PolicyStrict, nil)
	cause := errors.New("undecodable transaction")

	reporter.BeginLedger(4242)
	reporter.Report(warning("p", cause))

	err := reporter.Err()
	require.Error(t, err, "strict must halt on a warning")
	assert.ErrorIs(t, err, cause, "the original cause must survive")
	assert.ErrorContains(t, err, "4242", "the halting error must name the ledger")

	// The exit code has to distinguish "your dataset has holes" from "the
	// source was unreachable", which a consumer would retry.
	assert.Equal(t, 8, nebuErrors.ExitCode(err))

	select {
	case got := <-reporter.Fatal():
		assert.Equal(t, err, got, "the halting error must also reach a runner's select")
	default:
		t.Fatal("strict policy did not signal the fatal channel")
	}
}

func TestReporterStrictKeepsTheFirstError(t *testing.T) {
	reporter := NewReporter(context.Background(), PolicyStrict, nil)

	reporter.BeginLedger(10)
	reporter.Report(warning("p", errors.New("first")))
	reporter.BeginLedger(11)
	reporter.Report(warning("p", errors.New("second")))

	// The first failure is the one that explains the run's outcome.
	assert.ErrorContains(t, reporter.Err(), "first")
}

func TestReporterFatalHaltsUnderEveryPolicy(t *testing.T) {
	for _, policy := range []Policy{PolicyBestEffort, PolicyStrict} {
		t.Run(policy.String(), func(t *testing.T) {
			reporter := NewReporter(context.Background(), policy, nil)
			cause := errors.New("database connection lost")

			reporter.BeginLedger(10)
			reporter.Report(processor.ErrorReport{
				Processor: "p", Severity: processor.SeverityFatal, Err: cause,
			})

			require.Error(t, reporter.Err())
			assert.ErrorIs(t, reporter.Err(), cause)
			// A fatal is unrecoverable, not an incompleteness verdict, so it
			// keeps the generic failure code.
			assert.Equal(t, 1, nebuErrors.ExitCode(reporter.Err()))
		})
	}
}

func TestReporterFiresHooks(t *testing.T) {
	var warnings, fatals int
	reporter := NewReporter(context.Background(), PolicyBestEffort, []Hooks{{
		OnWarning: func(context.Context, processor.ErrorReport) { warnings++ },
		OnFatal:   func(context.Context, processor.ErrorReport) { fatals++ },
	}})

	reporter.BeginLedger(10)
	reporter.Report(warning("p", errors.New("boom")))
	reporter.Report(processor.ErrorReport{
		Processor: "p", Severity: processor.SeverityFatal, Err: errors.New("dead"),
	})

	assert.Equal(t, 1, warnings)
	assert.Equal(t, 1, fatals)
}

func TestReporterWithoutBeginLedgerStillCounts(t *testing.T) {
	// Paths that do not announce ledgers lose per-ledger coverage but must
	// not lose the reports themselves.
	reporter := NewReporter(context.Background(), PolicyBestEffort, nil)
	reporter.Report(warning("p", errors.New("boom")))

	coverage := reporter.Coverage()
	assert.Equal(t, int64(1), coverage.Warnings)
	assert.Zero(t, coverage.Ledgers)
}

// strictOrigin reports a warning for one ledger so an end-to-end run can
// show what each policy does with it.
type strictOrigin struct {
	warnOn uint32
	seen   []uint32
}

func (o *strictOrigin) Name() string         { return "strict-origin" }
func (o *strictOrigin) Type() processor.Type { return processor.TypeOrigin }
func (o *strictOrigin) Version() string      { return "test" }

func (o *strictOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
	sequence := ledger.LedgerSequence()
	o.seen = append(o.seen, sequence)
	if sequence == o.warnOn {
		processor.ReportWarning(ctx, o.Name(), errors.New("synthetic failure"))
	}
}

// sequenceSource streams header-only ledgers over the requested range.
type sequenceSource struct{}

func (sequenceSource) Stream(ctx context.Context, start, end uint32, out chan<- xdr.LedgerCloseMeta) error {
	defer close(out)
	for seq := start; seq <= end; seq++ {
		ledger := xdr.LedgerCloseMeta{
			V: 0,
			V0: &xdr.LedgerCloseMetaV0{
				LedgerHeader: xdr.LedgerHeaderHistoryEntry{
					Header: xdr.LedgerHeader{LedgerSeq: xdr.Uint32(seq)},
				},
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

func (sequenceSource) Close() error { return nil }

func TestRunOriginBestEffortFinishesWithCoverage(t *testing.T) {
	origin := &strictOrigin{warnOn: 11}

	var summary PipelineSummary
	rt := NewRuntime()
	rt.Use(Hooks{OnEnd: func(_ context.Context, s PipelineSummary) { summary = s }})

	require.NoError(t, rt.RunOrigin(context.Background(), sequenceSource{}, origin, 10, 12))

	assert.Equal(t, []uint32{10, 11, 12}, origin.seen, "the run must cover the whole range")
	assert.Equal(t, uint64(3), summary.Coverage.Ledgers)
	assert.Equal(t, uint64(1), summary.Coverage.IncompleteLedgers)
	assert.Equal(t, uint32(11), summary.Coverage.FirstIncompleteLedger)
	// Finished successfully, but the summary says the dataset has a hole —
	// which is the whole point of tracking coverage.
	assert.False(t, summary.Coverage.Complete())
	assert.NoError(t, summary.FatalErr)
}

func TestRunOriginStrictAbortsAndSaysWhich(t *testing.T) {
	origin := &strictOrigin{warnOn: 11}

	var summary PipelineSummary
	rt := NewRuntime()
	rt.SetPolicy(PolicyStrict)
	rt.Use(Hooks{OnEnd: func(_ context.Context, s PipelineSummary) { summary = s }})

	err := rt.RunOrigin(context.Background(), sequenceSource{}, origin, 10, 12)

	require.Error(t, err)
	assert.ErrorContains(t, err, "11")
	assert.Equal(t, 8, nebuErrors.ExitCode(err))
	assert.NotContains(t, origin.seen, uint32(12),
		"strict must not keep processing after the halt")
	assert.Equal(t, err, summary.FatalErr)
}
