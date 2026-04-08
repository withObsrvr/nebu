package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/withObsrvr/nebu/pkg/processor"
)

// describeEnvelopeTimeout caps how long nebu will wait for a processor
// binary to respond to --describe-json. Describe must be instant; if a
// binary takes longer, something is wrong.
const describeEnvelopeTimeout = 5 * time.Second

// findProcessorBinary resolves a processor name to an executable path.
//
// The resolution order is:
//  1. ./bin/<name> (repo-local dev build, from "make build-processors")
//  2. $PATH lookup (where "nebu install" puts binaries)
//
// ./bin/ wins when it exists because a developer running describe
// inside the nebu source tree almost always wants the freshly-built
// binary rather than whatever stale copy is in $GOPATH/bin. Users
// outside the source tree won't have a ./bin/ directory, so the PATH
// lookup takes over naturally.
//
// Returns an empty string and a non-nil error if the binary is not
// found — callers should treat this as "fall back to registry-only
// describe output," not a hard error.
func findProcessorBinary(name string) (string, error) {
	candidate := filepath.Join("bin", name)
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		abs, absErr := filepath.Abs(candidate)
		if absErr == nil {
			return abs, nil
		}
		return candidate, nil
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("processor binary %q not found in ./bin/ or $PATH", name)
}

// fetchEnvelope executes <name> --describe-json and parses the output
// into a DescribeEnvelope. Returns nil and a non-nil error on any
// failure (binary not found, timeout, non-zero exit, invalid JSON).
// The returned error is safe to present to the user.
func fetchEnvelope(name string) (*processor.DescribeEnvelope, error) {
	bin, err := findProcessorBinary(name)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), describeEnvelopeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "--describe-json")
	// Inherit stderr so the user sees any warnings the processor emits,
	// but capture stdout for parsing.
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running %q --describe-json: %w", name, err)
	}

	var env processor.DescribeEnvelope
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, fmt.Errorf("parsing describe envelope from %q: %w", name, err)
	}
	return &env, nil
}

// tryFetchEnvelope is the non-error variant of fetchEnvelope used by
// the main describe command. It returns nil silently if the binary
// is not installed (which is a normal state, not a failure), and
// returns nil with a stderr warning if the envelope parse fails
// (which is a real problem worth flagging but not worth halting on).
func tryFetchEnvelope(name string) *processor.DescribeEnvelope {
	env, err := fetchEnvelope(name)
	if err != nil {
		// Only print a warning for hard failures — the "not installed"
		// case is expected and handled silently.
		if _, lookErr := findProcessorBinary(name); lookErr == nil {
			fmt.Fprintf(os.Stderr, "warning: could not fetch describe envelope for %q: %v\n", name, err)
		}
		return nil
	}
	return env
}
