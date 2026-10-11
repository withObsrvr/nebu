package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/programstatus"
	"github.com/withObsrvr/nebu/pkg/status"
)

var (
	statusFilePath   string
	statusFileHandle *os.File
	statusWriter     *status.Writer
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
	if statusFileHandle == nil {
		return nil
	}
	err := statusFileHandle.Close()
	statusFileHandle = nil
	statusWriter = nil
	return err
}

func writeStructuredStatus(state programstatus.State, message string) error {
	if statusWriter == nil {
		return nil
	}
	return statusWriter.Write(status.Record{State: state, Message: oneLine(message)})
}

func writeStructuredProgress(current, target uint32) error {
	if statusWriter == nil {
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

func finishStatusStream(commandErr error) error {
	if statusWriter != nil {
		state := programstatus.Done
		message := "Command finished"
		if errors.Is(commandErr, context.Canceled) {
			state = programstatus.Idle
			message = "Command canceled"
		} else if commandErr != nil {
			state = programstatus.Error
			message = commandErr.Error()
		}
		if err := writeStructuredStatus(state, message); err != nil {
			commandErr = errors.Join(commandErr, fmt.Errorf("write final status: %w", err))
		}
	}
	if err := closeStatusStream(); err != nil {
		commandErr = errors.Join(commandErr, fmt.Errorf("close status file: %w", err))
	}
	return commandErr
}
