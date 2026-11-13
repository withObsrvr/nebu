package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/xdr"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
	"github.com/withObsrvr/nebu/pkg/source"
	"github.com/withObsrvr/nebu/pkg/source/rpc"
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
	ledgerCount := uint64(0)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Decide that the stream has ended by looking for the absence of a
		// next record, not by inspecting the decoder's error.
		//
		// The previous test accepted any error whose text contained "EOF"
		// once at least one ledger had been read, which is exactly what
		// the decoder returns when it runs out of input *mid-record*. A
		// complete ledger followed by a truncated one was therefore
		// reported as a clean, complete run — under the strict policy too,
		// which is the one case that must never happen. Peeking one byte
		// separates "no next record" from "a next record that does not
		// decode", so every decode failure can now be treated as one.
		if _, err := reader.Peek(1); err != nil {
			if errors.Is(err, io.EOF) {
				reportCoverage(reporter.Coverage(ledgerCount))
				return nil
			}
			return nebuErrors.XDRDecodeFailed(int(ledgerCount)+1, err)
		}

		var ledger xdr.LedgerCloseMeta
		if _, err := xdr.Unmarshal(reader, &ledger); err != nil {
			return nebuErrors.XDRDecodeFailed(int(ledgerCount)+1, err)
		}

		// Attribute reports to this ledger before anything that can
		// report. resolver.observe warns on a network mismatch, which
		// would otherwise be recorded against the previous ledger.
		reporter.BeginLedger(ledger.LedgerSequence())

		resolver.observe(ctx, ledger)
		origin.ProcessLedger(ctx, ledger)
		ledgerCount++

		// A fatal report — or any report at all under the strict policy —
		// halts the run here. The runtime path learns this from its select
		// loop; this path has to ask.
		if err := reporter.Err(); err != nil {
			reportCoverage(reporter.Coverage(ledgerCount))
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

// lastFlagName is the flag every origin CLI exposes to ask for the most
// recent N ledgers without having to look a sequence number up first.
const lastFlagName = "last"

// lastEnvVar is the environment fallback for lastFlagName.
const lastEnvVar = "NEBU_LAST"

// registerLastFlag adds the recent-ledgers flag to cmd.
func registerLastFlag(cmd *cobra.Command, value *uint32) {
	cmd.Flags().Uint32Var(value, lastFlagName, 0,
		"Process the most recent N ledgers instead of an explicit range (or set "+lastEnvVar+")")
}

// resolveLast returns the flag value, or the environment fallback.
func resolveLast(flagValue uint32) (uint32, error) {
	if flagValue != 0 {
		return flagValue, nil
	}
	raw := os.Getenv(lastEnvVar)
	if raw == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, nebuErrors.Usage(fmt.Errorf("%s must be a ledger count: %w", lastEnvVar, err))
	}
	return uint32(parsed), nil
}

// lastLedgerRange turns --last N into a concrete bounded range by asking the
// RPC endpoint which ledgers it holds.
//
// The range is always the full N that was asked for. A source that cannot
// serve it fails the run rather than returning a shorter result that looks
// complete, which is why the oldest ledger the endpoint reports produces a
// note rather than a refusal — archive-backed endpoints report a window of
// their last few dozen ledgers and serve history millions deeper.
func lastLedgerRange(rpcURL, authHeader string, last uint32) (uint32, uint32, error) {
	headers := map[string]string{}
	if authHeader != "" {
		headers["Authorization"] = authHeader
	}

	src, err := rpc.NewLedgerSourceWithHeaders(rpcURL, headers)
	if err != nil {
		return 0, 0, nebuErrors.FailedToCreateSource("RPC", err)
	}
	defer func() { _ = src.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	window, err := src.LedgerWindow(ctx)
	if err != nil {
		return 0, 0, nebuErrors.FailedToCreateSource("RPC", err)
	}

	start, end, err := source.ResolveLast(window, last)
	if err != nil {
		return 0, 0, &nebuErrors.NebuError{
			Kind:       nebuErrors.KindNotFound,
			Cause:      err,
			Message:    err.Error(),
			Suggestion: fmt.Sprintf("The RPC source reports ledgers %s.", window),
		}
	}

	fmt.Fprintf(os.Stderr, "Resolved --last %d to ledgers %d-%d (source reports %s)\n",
		last, start, end, window)
	if !window.Covers(start) {
		fmt.Fprintf(os.Stderr,
			"Note: ledger %d is below the oldest the source reported (%d). Archive-backed "+
				"endpoints routinely serve deeper than they report; if this one does not, "+
				"the run will fail rather than process a partial range.\n", start, window.Oldest)
	}
	return start, end, nil
}
