package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/xdr"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
)

// policyFlagName is the flag every origin CLI exposes to choose what an
// unprocessable ledger means for the run.
const policyFlagName = "on-ledger-error"

// policyEnvVar is the environment fallback for policyFlagName.
const policyEnvVar = "NEBU_ON_LEDGER_ERROR"

// registerPolicyFlag adds the execution-policy flag to cmd.
func registerPolicyFlag(cmd *cobra.Command, value *string) {
	cmd.Flags().StringVar(value, policyFlagName, "",
		"What an unprocessable ledger means for the run: 'best-effort' (continue, report coverage) or 'strict' (stop) (or set "+policyEnvVar+")")
}

// resolvePolicy turns the flag value, or the environment fallback, into a
// policy. A usage error is returned for an unrecognized value rather than
// silently defaulting, because the whole point of the flag is that the
// operator is making the completeness guarantee explicit.
func resolvePolicy(flagValue string) (runtime.Policy, error) {
	value := flagValue
	if value == "" {
		value = os.Getenv(policyEnvVar)
	}

	policy, err := runtime.ParsePolicy(value)
	if err != nil {
		return policy, nebuErrors.Usage(err)
	}
	return policy, nil
}

// withCoverageHook appends a hook that reports the run's coverage when it
// finished with gaps.
//
// Coverage goes to stderr regardless of --quiet: an incomplete dataset is
// closer to an error than to progress chatter, and the entire reason to track
// coverage is that a run with holes must not look like a clean one.
func withCoverageHook(hooks []runtime.Hooks) []runtime.Hooks {
	return append(hooks, runtime.Hooks{
		OnEnd: func(_ context.Context, summary runtime.PipelineSummary) {
			reportCoverage(summary.Coverage)
		},
	})
}

func reportCoverage(coverage runtime.Coverage) {
	if coverage.Complete() {
		return
	}
	fmt.Fprintf(os.Stderr, "[nebu] incomplete coverage: %s\n", coverage)
}

// processLedgerStream decodes XDR ledgers from input and feeds them to origin.
//
// This is the path a processor runs on when it is composed with a pipe —
// `nebu fetch ... | processor` — and it is shared by every origin CLI
// variant, which previously each carried their own copy of this loop. The
// copies had drifted: two of them reported decode failures with a bare
// fmt.Errorf, losing both the actionable message and the exit-code
// classification that the third produced.
func processLedgerStream(
	ctx context.Context,
	origin processor.Origin,
	input io.Reader,
	status *commandStatus,
	target uint32,
	resolver *networkResolver,
	policy runtime.Policy,
) error {
	// Attach a reporter even though there is no runtime on this path.
	// Without one, processor.ReporterFrom falls back to the package default,
	// which counts nothing and calls os.Exit on a fatal report — so warnings
	// were invisible and no policy was enforceable in exactly the mode Unix
	// composition uses.
	reporter := runtime.NewReporter(ctx, policy, nil)
	ctx = processor.WithReporter(ctx, reporter)

	reader := bufio.NewReader(input)
	ledgerCount := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var ledger xdr.LedgerCloseMeta
		if _, err := xdr.Unmarshal(reader, &ledger); err != nil {
			// Clean EOF, or an EOF-flavored error after at least one
			// ledger, is the end of the stream rather than a failure.
			if err == io.EOF || (ledgerCount > 0 && isEOFError(err)) {
				reportCoverage(reporter.Coverage())
				return nil
			}
			return nebuErrors.XDRDecodeFailed(ledgerCount+1, err)
		}

		resolver.observe(ctx, ledger)

		// BeginLedger first, so anything the processor reports is
		// attributed to this ledger in the run's coverage.
		reporter.BeginLedger(ledger.LedgerSequence())
		origin.ProcessLedger(ctx, ledger)
		ledgerCount++

		// A fatal report — or any report at all under the strict policy —
		// halts the run here. The runtime path learns this from its select
		// loop; this path has to ask.
		if err := reporter.Err(); err != nil {
			reportCoverage(reporter.Coverage())
			return err
		}

		if err := status.ledgerProgress(ledger.LedgerSequence(), target); err != nil {
			return err
		}
	}
}

// processLedgerFile decodes XDR ledgers from a file and feeds them to origin.
func processLedgerFile(
	ctx context.Context,
	origin processor.Origin,
	filePath string,
	status *commandStatus,
	target uint32,
	resolver *networkResolver,
	policy runtime.Policy,
) error {
	file, err := os.Open(filePath)
	if err != nil {
		return nebuErrors.FailedToOpenFile(filePath, err)
	}
	defer file.Close()

	return processLedgerStream(ctx, origin, file, status, target, resolver, policy)
}

// isEOFError reports whether err means the XDR stream ended. The decoder
// returns several EOF-flavored errors when it runs out of input mid-record,
// not just io.EOF.
func isEOFError(err error) bool {
	if err == nil {
		return false
	}
	return err == io.EOF || strings.Contains(err.Error(), "EOF")
}
