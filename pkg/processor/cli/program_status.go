package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/programstatus"
	statusstream "github.com/withObsrvr/nebu/pkg/status"
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
	statusPath string
	statusFile *os.File
	structured *statusstream.Writer
	statusMu   sync.Mutex
	statusErr  error
}

func attachProgramStatus(cmd *cobra.Command, app string, quiet *bool) *commandStatus {
	status := &commandStatus{
		app:        app,
		quiet:      quiet,
		writer:     os.Stderr,
		isTerminal: func() bool { return terminalFile(os.Stderr) },
	}
	cmd.Flags().StringVar(&status.mode, programStatusFlagName, programStatusDefault(), "program status reporting: auto, always, or never (or set NEBU_PROGRAM_STATUS)")
	cmd.Flags().StringVar(&status.statusPath, "status-file", os.Getenv("NEBU_STATUS_FILE"), "write nebu.status.v1 NDJSON updates to a dedicated file (or set NEBU_STATUS_FILE)")

	run := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validateProgramStatusMode(status.mode); err != nil {
			return err
		}
		if err := status.openStructured(); err != nil {
			return err
		}
		status.writeOSC(programstatus.Working, "Running "+app)
		if err := status.writeStructured(statusstream.Record{State: programstatus.Working, Message: "Running " + app}); err != nil {
			return errors.Join(err, status.closeStructured())
		}
		err := run(cmd, args)
		switch {
		case status.canceled.Load():
			// cancel reports idle immediately, before forced-exit timers can fire.
			err = errors.Join(err, context.Canceled)
		case err == nil:
			status.writeOSC(programstatus.Done, app+" finished")
			err = errors.Join(err, status.writeStructured(statusstream.Record{State: programstatus.Done, Message: app + " finished"}))
		case errors.Is(err, context.Canceled):
			status.writeOSC(programstatus.Idle, app+" canceled")
			err = errors.Join(err, status.writeStructured(statusstream.Record{State: programstatus.Idle, Message: app + " canceled"}))
		default:
			status.writeOSC(programstatus.Error, statusText(err.Error()))
			err = errors.Join(err, status.writeStructured(statusstream.Record{State: programstatus.Error, Message: statusText(err.Error())}))
		}
		err = errors.Join(err, status.takeStatusError(), status.closeStructured())
		return err
	}
	return status
}

func terminalFile(file *os.File) bool {
	return os.Getenv("TERM") != "dumb" && term.IsTerminal(int(file.Fd()))
}

func (s *commandStatus) cancel() {
	if s.canceled.CompareAndSwap(false, true) {
		s.writeOSC(programstatus.Idle, s.app+" canceled")
		s.recordStatusError(s.writeStructured(statusstream.Record{State: programstatus.Idle, Message: s.app + " canceled"}))
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

func (s *commandStatus) writeOSC(state programstatus.State, message string) {
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

func (s *commandStatus) openStructured() error {
	if s.statusPath == "" {
		return nil
	}
	if statusInfo, err := os.Stat(s.statusPath); err == nil {
		stdoutInfo, stdoutErr := os.Stdout.Stat()
		if stdoutErr != nil {
			return fmt.Errorf("inspect stdout: %w", stdoutErr)
		}
		if os.SameFile(statusInfo, stdoutInfo) {
			return fmt.Errorf("--status-file must not refer to redirected stdout")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect status file %q: %w", s.statusPath, err)
	}
	file, err := os.OpenFile(s.statusPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open status file %q: %w", s.statusPath, err)
	}
	s.statusFile = file
	s.structured = statusstream.NewWriter(file, s.app)
	return nil
}

func (s *commandStatus) closeStructured() error {
	if s.statusFile == nil {
		return nil
	}
	err := s.statusFile.Close()
	s.statusFile = nil
	s.structured = nil
	if err != nil {
		return fmt.Errorf("close status file: %w", err)
	}
	return nil
}

func (s *commandStatus) writeStructured(record statusstream.Record) error {
	if s == nil || s.structured == nil {
		return nil
	}
	if err := s.structured.Write(record); err != nil {
		return fmt.Errorf("write structured status: %w", err)
	}
	return nil
}

func (s *commandStatus) ledgerProgress(current, target uint32) error {
	if s == nil {
		return nil
	}
	record := statusstream.Record{State: programstatus.Working, CurrentLedger: &current, Message: "Processing ledgers"}
	if target > 0 {
		record.TargetLedger = &target
	}
	return s.writeStructured(record)
}

func (s *commandStatus) eventProgress(count uint64) error {
	if s == nil {
		return nil
	}
	return s.writeStructured(statusstream.Record{
		State: programstatus.Working, EventsProcessed: &count, Message: "Processing events",
	})
}

func (s *commandStatus) recordStatusError(err error) {
	if err == nil {
		return
	}
	s.statusMu.Lock()
	s.statusErr = errors.Join(s.statusErr, err)
	s.statusMu.Unlock()
}

func (s *commandStatus) takeStatusError() error {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	err := s.statusErr
	s.statusErr = nil
	return err
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
