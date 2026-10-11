package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

const checkpointVersion = 1

type fetchCheckpoint struct {
	Version             int       `json:"version"`
	RangeStart          uint32    `json:"range_start"`
	RangeEnd            uint32    `json:"range_end"`
	LastCompletedLedger uint32    `json:"last_completed_ledger"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type checkpointWriter struct {
	path       string
	state      fetchCheckpoint
	interval   uint32
	sinceWrite uint32
}

func (w *checkpointWriter) resuming() bool {
	return w != nil && w.state.LastCompletedLedger >= w.state.RangeStart
}

func openCheckpoint(path string, start, end, interval uint32) (*checkpointWriter, uint32, bool, error) {
	if path == "" {
		return nil, start, false, nil
	}
	if interval == 0 {
		return nil, 0, false, fmt.Errorf("--checkpoint-interval must be greater than zero")
	}

	writer := &checkpointWriter{
		path: path,
		state: fetchCheckpoint{
			Version:    checkpointVersion,
			RangeStart: start,
			RangeEnd:   end,
		},
		interval: interval,
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return writer, start, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("read checkpoint %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &writer.state); err != nil {
		return nil, 0, false, fmt.Errorf("parse checkpoint %q: %w", path, err)
	}
	if writer.state.Version != checkpointVersion {
		return nil, 0, false, fmt.Errorf("checkpoint %q has unsupported version %d", path, writer.state.Version)
	}
	if writer.state.RangeStart != start || writer.state.RangeEnd != end {
		return nil, 0, false, fmt.Errorf(
			"checkpoint %q belongs to range %d-%d, not requested range %d-%d",
			path, writer.state.RangeStart, writer.state.RangeEnd, start, end,
		)
	}
	if writer.state.LastCompletedLedger < start {
		return writer, start, false, nil
	}
	if end > 0 && writer.state.LastCompletedLedger >= end {
		return writer, end, true, nil
	}
	return writer, writer.state.LastCompletedLedger + 1, false, nil
}

func (w *checkpointWriter) advance(ledger uint32, force bool) error {
	if w == nil {
		return nil
	}
	w.state.LastCompletedLedger = ledger
	w.sinceWrite++
	if !force && w.sinceWrite < w.interval {
		return nil
	}
	w.state.UpdatedAt = time.Now().UTC()
	if err := writeCheckpointAtomically(w.path, w.state); err != nil {
		return err
	}
	w.sinceWrite = 0
	return nil
}

func (w *checkpointWriter) willPersist(force bool) bool {
	return w != nil && (force || w.sinceWrite+1 >= w.interval)
}

func validateCheckpointOutputPaths(checkpointPath, outputPath string) error {
	if checkpointPath == "" || outputPath == "" {
		return nil
	}

	checkpointAbs, err := filepath.Abs(filepath.Clean(checkpointPath))
	if err != nil {
		return fmt.Errorf("resolve checkpoint path %q: %w", checkpointPath, err)
	}
	outputAbs, err := filepath.Abs(filepath.Clean(outputPath))
	if err != nil {
		return fmt.Errorf("resolve output path %q: %w", outputPath, err)
	}
	if checkpointAbs == outputAbs {
		return fmt.Errorf("--checkpoint and --output must refer to different files")
	}

	checkpointInfo, checkpointErr := os.Stat(checkpointAbs)
	outputInfo, outputErr := os.Stat(outputAbs)
	if checkpointErr == nil && outputErr == nil && os.SameFile(checkpointInfo, outputInfo) {
		return fmt.Errorf("--checkpoint and --output must refer to different files")
	}
	if checkpointErr != nil && !os.IsNotExist(checkpointErr) {
		return fmt.Errorf("inspect checkpoint path %q: %w", checkpointPath, checkpointErr)
	}
	if outputErr != nil && !os.IsNotExist(outputErr) {
		return fmt.Errorf("inspect output path %q: %w", outputPath, outputErr)
	}
	return nil
}

func writeCheckpointAtomically(path string, state fetchCheckpoint) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create checkpoint directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".nebu-checkpoint-*")
	if err != nil {
		return fmt.Errorf("create temporary checkpoint: %w", err)
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpName) // best-effort cleanup after a reported write failure
		}
	}()

	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		_ = tmp.Close() // preserve the original encoding error
		return fmt.Errorf("encode checkpoint: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close() // preserve the original sync error
		return fmt.Errorf("sync checkpoint: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close checkpoint: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace checkpoint: %w", err)
	}
	removeTemp = false
	if err := syncCheckpointDirectory(dir); err != nil {
		return fmt.Errorf("sync checkpoint directory: %w", err)
	}
	return nil
}

func syncCheckpointDirectory(dir string) error {
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() {
		_ = directory.Close() // a sync or close error is reported by the explicit operations below
	}()

	if err := directory.Sync(); err != nil {
		// Windows and filesystems without directory fsync support still retain the
		// atomic rename guarantee, but cannot provide the stronger power-loss
		// durability guarantee.
		if runtime.GOOS == "windows" || errors.Is(err, syscall.EINVAL) {
			return nil
		}
		return err
	}
	if err := directory.Close(); err != nil {
		return err
	}
	return nil
}
