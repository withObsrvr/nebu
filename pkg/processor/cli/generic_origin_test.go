package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
)

// RunGenericOriginCLI had never been run by anything. Two faults sat in it:
// it did not register --describe-json, and it closed the origin in a deferred
// call, so the writer goroutine ranging over Out() waited for a close that
// could not happen until RunE returned, and RunE was waiting for the writer.
// A processor on this helper read its ledgers, wrote every event, and then
// hung forever.
//
// Both are reachable only by running the helper, which builds its own cobra
// command and calls os.Exit. So these tests re-exec the test binary: TestMain
// notices the environment variable and becomes the processor instead of
// running tests. The "no ledgers at all" case is the one that matters for the
// hang — the deadlock is in the shutdown path and does not need any input to
// reproduce.
const helperEnv = "NEBU_GENERIC_ORIGIN_HELPER"

type countingOrigin struct {
	out chan *ledgerCount
}

type ledgerCount struct {
	LedgerSequence uint32 `json:"ledger_sequence"`
}

func (o *countingOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
	select {
	case <-ctx.Done():
	case o.out <- &ledgerCount{LedgerSequence: ledger.LedgerSequence()}:
	}
}

func (o *countingOrigin) Out() <-chan *ledgerCount { return o.out }
func (o *countingOrigin) Close()                   { close(o.out) }
func (o *countingOrigin) Name() string             { return "generic-origin-under-test" }
func (o *countingOrigin) Type() processor.Type     { return processor.TypeOrigin }

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "1" {
		RunGenericOriginCLI(
			OriginConfig{
				Name:        "generic-origin-under-test",
				Description: "exercises RunGenericOriginCLI",
				Version:     "0.0.1",
				SchemaID:    "nebu.generic_origin_under_test.v1",
			},
			func(string) GenericOriginProcessor[ledgerCount] {
				return &countingOrigin{out: make(chan *ledgerCount, 8)}
			},
		)
		return
	}
	os.Exit(m.Run())
}

// runHelper re-execs the test binary as the processor and returns its stdout.
func runHelper(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("the helper did not exit within 20s — it is waiting on something.\nstderr: %s", stderr.String())
	}

	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("running the helper: %v (stderr: %s)", err, stderr.String())
	}
	return stdout.String(), stderr.String(), code
}

func TestGenericOriginCLITerminates(t *testing.T) {
	// Empty stdin: no ledgers, nothing to emit, and the only thing left is
	// the shutdown path that used to deadlock.
	stdout, stderr, code := runHelper(t, "", "--quiet")

	if code != 0 {
		t.Errorf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if stdout != "" {
		t.Errorf("emitted events from an empty stream: %q", stdout)
	}
}

func TestGenericOriginCLIDescribeJSON(t *testing.T) {
	stdout, stderr, code := runHelper(t, "", "--describe-json")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	// The protocol is strict about this: JSON only on stdout, and it must
	// work without any of the processor's other flags set.
	if stderr != "" {
		t.Errorf("wrote to stderr during describe: %q", stderr)
	}

	var envelope processor.DescribeEnvelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("stdout is not a describe envelope: %v\n%s", err, stdout)
	}
	if envelope.Name != "generic-origin-under-test" {
		t.Errorf("envelope name = %q", envelope.Name)
	}
	if envelope.Type != processor.TypeOrigin.String() {
		t.Errorf("envelope type = %q, want %q", envelope.Type, processor.TypeOrigin.String())
	}
	if envelope.Schema.ID != "nebu.generic_origin_under_test.v1" {
		t.Errorf("envelope schema id = %q", envelope.Schema.ID)
	}
	// The flag set is the useful half of the envelope when there is no output
	// schema to publish, so it must not be empty.
	if len(envelope.Flags) == 0 {
		t.Error("envelope reports no flags")
	}
}
