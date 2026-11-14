package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStderr runs fn with os.Stderr replaced by a pipe and returns what it
// wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	saved := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = saved }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing the write end: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading the captured stderr: %v", err)
	}
	return string(out)
}

// logResolved exists because the quiet flag carried two different meanings.
// `nebu fetch` infers quiet mode when stdout is a pipe so a merged stderr
// cannot corrupt the stream, and that silenced the resolved --last range in
// the one form the quickstart documents — a piped fetch. An operator who
// typed --quiet still gets silence.
func TestLogResolvedSurvivesAutoQuietOnly(t *testing.T) {
	tests := []struct {
		name  string
		quiet bool
		auto  bool
		want  bool
	}{
		{name: "plain terminal run", quiet: false, auto: false, want: true},
		{name: "piped stdout, quiet inferred", quiet: true, auto: true, want: true},
		{name: "operator asked for quiet", quiet: true, auto: false, want: false},
		// An explicit --quiet plus a pipe is the row above, not a fifth case:
		// fetch only infers quiet mode when it is not already on, so
		// autoQuiet never gets set over an operator's own --quiet.
	}

	savedQuiet, savedAuto := quietMode, autoQuiet
	defer func() { quietMode, autoQuiet = savedQuiet, savedAuto }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietMode, autoQuiet = tt.quiet, tt.auto

			out := captureStderr(t, func() {
				logResolved("Resolved --last %d to ledgers %d-%d", 3, 100, 102)
			})

			if got := strings.Contains(out, "Resolved --last 3"); got != tt.want {
				t.Errorf("message printed = %v, want %v (stderr: %q)", got, tt.want, out)
			}
		})
	}
}

// logInfo must keep deferring to either kind of quiet: the banner and the
// progress counters are exactly what a piped stdout should not risk.
func TestLogInfoStaysSilentUnderAnyQuiet(t *testing.T) {
	savedQuiet, savedAuto := quietMode, autoQuiet
	defer func() { quietMode, autoQuiet = savedQuiet, savedAuto }()

	quietMode, autoQuiet = true, true
	if out := captureStderr(t, func() { logInfo("fetching") }); out != "" {
		t.Errorf("logInfo wrote %q under auto-quiet, want nothing", out)
	}
}
