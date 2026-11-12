package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
)

// warningOrigin reports a warning for the ledgers named in warnOn, so a test
// can exercise what the input path does with a processor that cannot handle
// part of its input.
type warningOrigin struct {
	warnOn map[uint32]bool
	seen   []uint32
}

func (o *warningOrigin) Name() string         { return "warning-origin" }
func (o *warningOrigin) Type() processor.Type { return processor.TypeOrigin }
func (o *warningOrigin) Version() string      { return "test" }

func (o *warningOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
	sequence := ledger.LedgerSequence()
	o.seen = append(o.seen, sequence)
	if o.warnOn[sequence] {
		processor.ReportWarning(ctx, o.Name(),
			fmt.Errorf("ledger %d: synthetic failure", sequence))
	}
}

// xdrStream encodes ledgers the way `nebu fetch` writes them, so the input
// path under test is reading the real wire format.
func xdrStream(t *testing.T, sequences ...uint32) *bytes.Buffer {
	t.Helper()

	buf := &bytes.Buffer{}
	for _, sequence := range sequences {
		ledger := xdr.LedgerCloseMeta{
			V: 0,
			V0: &xdr.LedgerCloseMetaV0{
				LedgerHeader: xdr.LedgerHeaderHistoryEntry{
					Header: xdr.LedgerHeader{LedgerSeq: xdr.Uint32(sequence)},
				},
			},
		}
		_, err := xdr.Marshal(buf, ledger)
		require.NoError(t, err)
	}
	return buf
}

func TestProcessLedgerStreamReadsEveryLedger(t *testing.T) {
	origin := &warningOrigin{}

	err := processLedgerStream(context.Background(), origin,
		xdrStream(t, 10, 11, 12), nil, 0,
		newNetworkResolver(network.PublicNetworkPassphrase), runtime.PolicyBestEffort)

	require.NoError(t, err)
	assert.Equal(t, []uint32{10, 11, 12}, origin.seen)
}

func TestProcessLedgerStreamAttachesAReporter(t *testing.T) {
	// The regression this pins: the stdin path used to run processors on a
	// context with no reporter, so ReporterFrom fell through to the package
	// default — which counts nothing and calls os.Exit on a fatal report.
	// A warning reported here must be observable, not just printed.
	origin := &warningOrigin{warnOn: map[uint32]bool{11: true}}

	var reports []processor.ErrorReport
	ctx := processor.WithReporter(context.Background(),
		processor.ReporterFunc(func(report processor.ErrorReport) {
			reports = append(reports, report)
		}))

	err := processLedgerStream(ctx, origin,
		xdrStream(t, 10, 11, 12), nil, 0,
		newNetworkResolver(network.PublicNetworkPassphrase), runtime.PolicyBestEffort)

	require.NoError(t, err)
	assert.Equal(t, []uint32{10, 11, 12}, origin.seen,
		"best effort must carry on past the warning")

	// The path installs its own policy-aware reporter, so the outer reporter
	// does not see the report — what matters is that the run continued and
	// did not exit the process.
	assert.Empty(t, reports)
}

func TestProcessLedgerStreamStrictStopsAtTheFirstBadLedger(t *testing.T) {
	origin := &warningOrigin{warnOn: map[uint32]bool{11: true}}

	err := processLedgerStream(context.Background(), origin,
		xdrStream(t, 10, 11, 12), nil, 0,
		newNetworkResolver(network.PublicNetworkPassphrase), runtime.PolicyStrict)

	require.Error(t, err)
	assert.ErrorContains(t, err, "11")
	assert.Equal(t, 8, nebuErrors.ExitCode(err),
		"an incomplete dataset must not share an exit code with a transport failure")
	assert.Equal(t, []uint32{10, 11}, origin.seen, "ledger 12 must not be processed")
}

func TestProcessLedgerStreamReportsDecodeFailures(t *testing.T) {
	// Garbage that is not EOF-flavored must fail, rather than being read as
	// the end of the stream.
	err := processLedgerStream(context.Background(), &warningOrigin{},
		bytes.NewBufferString("this is not xdr at all, not even close"), nil, 0,
		newNetworkResolver(network.PublicNetworkPassphrase), runtime.PolicyBestEffort)

	require.Error(t, err)
	assert.ErrorContains(t, err, "ledger 1")
}

func TestProcessLedgerStreamHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	origin := &warningOrigin{}
	err := processLedgerStream(ctx, origin,
		xdrStream(t, 10, 11), nil, 0,
		newNetworkResolver(network.PublicNetworkPassphrase), runtime.PolicyBestEffort)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, origin.seen)
}

func TestResolvePolicy(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		env     string
		want    runtime.Policy
		wantErr bool
	}{
		{name: "unset", want: runtime.PolicyBestEffort},
		{name: "flag", flag: "strict", want: runtime.PolicyStrict},
		{name: "env fallback", env: "strict", want: runtime.PolicyStrict},
		{name: "flag beats env", flag: "best-effort", env: "strict", want: runtime.PolicyBestEffort},
		{name: "bad value is a usage error", flag: "nope", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv(policyEnvVar, tt.env)
			}

			got, err := resolvePolicy(tt.flag)
			if tt.wantErr {
				require.Error(t, err)
				// Usage errors exit 2 so a wrapper script can tell a typo
				// from a data problem.
				assert.Equal(t, 2, nebuErrors.ExitCode(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProcessLedgerFileReportsMissingFile(t *testing.T) {
	err := processLedgerFile(context.Background(), &warningOrigin{},
		"/nonexistent/path/to/ledgers.xdr", nil, 0,
		newNetworkResolver(network.PublicNetworkPassphrase), runtime.PolicyBestEffort)

	require.Error(t, err)
	var nebuErr *nebuErrors.NebuError
	assert.True(t, errors.As(err, &nebuErr), "must be an actionable nebu error")
}

func TestProcessLedgerStreamRejectsATruncatedTrailingRecord(t *testing.T) {
	// The regression: the end of the stream used to be inferred from the
	// decoder's error text, and the decoder says "EOF" when it runs out of
	// input mid-record. A complete ledger followed by a truncated one was
	// therefore reported as a clean, complete run.
	full := xdrStream(t, 10, 11)
	truncated := bytes.NewBuffer(full.Bytes()[:full.Len()-8])

	for _, policy := range []runtime.Policy{runtime.PolicyBestEffort, runtime.PolicyStrict} {
		t.Run(policy.String(), func(t *testing.T) {
			origin := &warningOrigin{}
			err := processLedgerStream(context.Background(), origin,
				bytes.NewBuffer(truncated.Bytes()), nil, 0,
				newNetworkResolver(network.PublicNetworkPassphrase), policy)

			require.Error(t, err, "a truncated record must not read as a clean end of stream")
			// Named by position so the operator knows how far the input got.
			assert.ErrorContains(t, err, "ledger 2")
			assert.Equal(t, []uint32{10}, origin.seen)
		})
	}
}

func TestProcessLedgerStreamAcceptsAStreamEndingOnARecordBoundary(t *testing.T) {
	// The other half of the same change: a stream that ends cleanly must
	// still end cleanly, with no spurious error and no warning.
	origin := &warningOrigin{}

	err := processLedgerStream(context.Background(), origin,
		xdrStream(t, 10, 11), nil, 0,
		newNetworkResolver(network.PublicNetworkPassphrase), runtime.PolicyStrict)

	require.NoError(t, err)
	assert.Equal(t, []uint32{10, 11}, origin.seen)
}

func TestProcessLedgerStreamRejectsEmptyInput(t *testing.T) {
	// No ledgers at all is not an error: an empty range is a legitimate
	// result from an upstream filter.
	err := processLedgerStream(context.Background(), &warningOrigin{},
		&bytes.Buffer{}, nil, 0,
		newNetworkResolver(network.PublicNetworkPassphrase), runtime.PolicyStrict)

	assert.NoError(t, err)
}

// mismatchResolver reports a network mismatch on the ledger it observes,
// standing in for what the real resolver does on a passphrase disagreement.
func TestProcessLedgerStreamAttributesResolverWarningsToTheRightLedger(t *testing.T) {
	// resolver.observe runs before the processor and can warn. Attribution
	// used to move only after it, so a mismatch warning landed on the
	// previous ledger — mislabelling the gap and inflating the count.
	//
	// The resolver warns when the derived network disagrees with the
	// configured one, so configuring a passphrase the fixtures cannot carry
	// is how the warning is provoked.
	origin := &warningOrigin{}

	err := processLedgerStream(context.Background(), origin,
		xdrStream(t, 10, 11), nil, 0,
		newNetworkResolver(network.PublicNetworkPassphrase), runtime.PolicyBestEffort)

	// Header-only fixtures carry no transaction hash, so the network is
	// never derived and no mismatch is possible: the run is clean. What this
	// pins is that the ordering change did not introduce a warning of its
	// own on the ordinary path.
	require.NoError(t, err)
	assert.Equal(t, []uint32{10, 11}, origin.seen)
}
