package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/programstatus"
	"golang.org/x/term"
)

const programStatusFlagName = "program-status"

type commandStatus struct {
	app        string
	mode       string
	quiet      *bool
	writer     io.Writer
	isTerminal func() bool
	canceled   atomic.Bool
}

func attachProgramStatus(cmd *cobra.Command, app string, quiet *bool) *commandStatus {
	status := &commandStatus{
		app:        app,
		quiet:      quiet,
		writer:     os.Stderr,
		isTerminal: func() bool { return terminalFile(os.Stderr) },
	}
	cmd.Flags().StringVar(&status.mode, programStatusFlagName, programStatusDefault(), "program status reporting: auto, always, or never (or set NEBU_PROGRAM_STATUS)")

	run := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validateProgramStatusMode(status.mode); err != nil {
			return err
		}
		status.write(programstatus.Working, "Running "+app)
		err := run(cmd, args)
		switch {
		case status.canceled.Load():
			// cancel reports idle immediately, before forced-exit timers can fire.
		case err == nil:
			status.write(programstatus.Done, app+" finished")
		case errors.Is(err, context.Canceled):
			status.write(programstatus.Idle, app+" canceled")
		default:
			status.write(programstatus.Error, statusText(err.Error()))
		}
		return err
	}
	return status
}

func terminalFile(file *os.File) bool {
	return os.Getenv("TERM") != "dumb" && term.IsTerminal(int(file.Fd()))
}

func (s *commandStatus) cancel() {
	if s.canceled.CompareAndSwap(false, true) {
		s.write(programstatus.Idle, s.app+" canceled")
	}
}

func programStatusDefault() string {
	if value := os.Getenv("NEBU_PROGRAM_STATUS"); value != "" {
		return value
	}
	return "auto"
}

func validateProgramStatusMode(mode string) error {
	switch mode {
	case "auto", "always", "never":
		return nil
	default:
		return fmt.Errorf("invalid --program-status value %q: expected auto, always, or never", mode)
	}
}

func (s *commandStatus) enabled() bool {
	if s.mode == "never" || (*s.quiet && s.mode != "always") {
		return false
	}
	return s.mode == "always" || (s.mode == "auto" && s.isTerminal())
}

func (s *commandStatus) write(state programstatus.State, message string) {
	if !s.enabled() {
		return
	}
	if err := programstatus.Write(s.writer, programstatus.Report{
		State: state, App: s.app, Message: message,
	}); err != nil {
		// Status reporting is an optional terminal enhancement and must
		// never make the processor fail.
		_ = err
	}
}

func statusText(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	if len(value) > 2048 {
		return strings.ToValidUTF8(value[:2048], "")
	}
	return value
}
