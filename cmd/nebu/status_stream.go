package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/programstatus"
	"github.com/withObsrvr/nebu/pkg/status"
)

var (
	statusFilePath string

	// statusMu guards the writer handles below. A signal handler reports
	// cancellation from its own goroutine while the command is still
	// emitting progress from the main one, so these are touched
	// concurrently. status.Writer serialises its own writes; what needs
	// guarding is the handles being swapped for nil on close.
	statusMu         sync.Mutex
	statusFileHandle *os.File
	statusWriter     *status.Writer

	// statusTerminal records that a terminal record has been written.
	// Nothing may follow one: a consumer that reads idle/cancelled and
	// then working has watched a finished run come back to life.
	statusTerminal bool
)

func statusFileDefault() string {
	return os.Getenv("NEBU_STATUS_FILE")
}

func openStatusStream(cmd *cobra.Command) error {
	if statusFilePath == "" {
		return nil
	}
	if statusInfo, err := os.Stat(statusFilePath); err == nil {
		stdoutInfo, stdoutErr := os.Stdout.Stat()
		if stdoutErr != nil {
			return fmt.Errorf("inspect stdout: %w", stdoutErr)
		}
		if os.SameFile(statusInfo, stdoutInfo) {
			return fmt.Errorf("--status-file must not refer to redirected stdout")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect status file %q: %w", statusFilePath, err)
	}
	for _, flagName := range []string{"output", "checkpoint"} {
		flag := cmd.Flags().Lookup(flagName)
		if flag == nil || flag.Value.String() == "" {
			continue
		}
		same, err := pathsReferToSameFile(statusFilePath, flag.Value.String())
		if err != nil {
			return err
		}
		if same {
			return fmt.Errorf("--status-file and --%s must refer to different files", flagName)
		}
	}
	file, err := os.OpenFile(statusFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open status file %q: %w", statusFilePath, err)
	}
	statusFileHandle = file
	statusWriter = status.NewWriter(file, "nebu")
	return nil
}

func closeStatusStream() error {
	statusMu.Lock()
	defer statusMu.Unlock()
	return closeStatusStreamLocked()
}

func closeStatusStreamLocked() error {
	if statusFileHandle == nil {
		return nil
	}
	err := statusFileHandle.Close()
	statusFileHandle = nil
	statusWriter = nil
	return err
}

func writeStructuredStatus(state programstatus.State, message string) error {
	statusMu.Lock()
	defer statusMu.Unlock()
	return writeStructuredStatusLocked(state, message)
}

func writeStructuredStatusLocked(state programstatus.State, message string) error {
	if statusWriter == nil || statusTerminal {
		return nil
	}
	return statusWriter.Write(status.Record{State: state, Message: oneLine(message)})
}

func writeStructuredProgress(current, target uint32) error {
	statusMu.Lock()
	defer statusMu.Unlock()

	// Dropped once the run has reported a terminal state. The fetch loop
	// checks the context and then writes progress, so a signal arriving in
	// that gap — or while the preceding output write is blocked — would
	// otherwise append a working record after the terminal one.
	if statusWriter == nil || statusTerminal {
		return nil
	}
	record := status.Record{
		State:         programstatus.Working,
		CurrentLedger: &current,
		Message:       "Fetching ledgers",
	}
	if target > 0 {
		record.TargetLedger = &target
	}
	return statusWriter.Write(record)
}

// cancelStatusStream writes the terminal cancellation record as soon as the
// signal arrives, rather than waiting for the command to unwind.
//
// A command that cannot stop promptly is force-exited with os.Exit, and
// os.Exit runs no deferred code — so the terminal record written on the normal
// finish path never happens. The stream's last word stays "working" while the
// process exits 130, and anything following the stream shows that run as still
// live, forever. Reporting here means the record is already on disk before any
// force-exit timer can fire.
//
// Safe to call more than once; only the first call writes. The record is
// terminal, so every later write is dropped — including progress the main
// goroutine was already about to emit. The stream stays open because the
// normal finish path still owns closing it.
func cancelStatusStream() {
	statusMu.Lock()
	if statusTerminal {
		statusMu.Unlock()
		return
	}
	// Write before marking, both inside one critical section: the mark is
	// what suppresses later writes, so setting it first would suppress
	// this one.
	err := writeStructuredStatusLocked(programstatus.Idle, "Command canceled")
	statusTerminal = true
	statusMu.Unlock()

	if err != nil {
		fmt.Fprintln(os.Stderr, "write cancellation status:", err)
	}
	writeProgramStatus(programstatus.Idle, "Command canceled")
}

func finishStatusStream(commandErr error) error {
	statusMu.Lock()
	defer statusMu.Unlock()

	// cancelStatusStream may already have written the terminal record.
	// Writing another would put a second terminal state on a stream whose
	// contract says there is one.
	if statusWriter != nil && !statusTerminal {
		state := programstatus.Done
		message := "Command finished"
		if errors.Is(commandErr, context.Canceled) {
			state = programstatus.Idle
			message = "Command canceled"
		} else if commandErr != nil {
			state = programstatus.Error
			message = commandErr.Error()
		}
		if err := writeStructuredStatusLocked(state, message); err != nil {
			commandErr = errors.Join(commandErr, fmt.Errorf("write final status: %w", err))
		}
		statusTerminal = true
	}
	if err := closeStatusStreamLocked(); err != nil {
		commandErr = errors.Join(commandErr, fmt.Errorf("close status file: %w", err))
	}
	return commandErr
}
