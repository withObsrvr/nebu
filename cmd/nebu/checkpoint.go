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
	same, err := pathsReferToSameFile(checkpointPath, outputPath)
	if err != nil {
		return err
	}
	if same {
		return fmt.Errorf("--checkpoint and --output must refer to different files")
	}
	return nil
}

func pathsReferToSameFile(firstPath, secondPath string) (bool, error) {
	firstCanonical, err := canonicalizePath(firstPath)
	if err != nil {
		return false, fmt.Errorf("resolve path %q: %w", firstPath, err)
	}
	secondCanonical, err := canonicalizePath(secondPath)
	if err != nil {
		return false, fmt.Errorf("resolve path %q: %w", secondPath, err)
	}
	if firstCanonical == secondCanonical {
		return true, nil
	}

	firstInfo, firstErr := os.Stat(firstCanonical)
	secondInfo, secondErr := os.Stat(secondCanonical)
	if firstErr == nil && secondErr == nil && os.SameFile(firstInfo, secondInfo) {
		return true, nil
	}
	if firstErr != nil && !os.IsNotExist(firstErr) {
		return false, fmt.Errorf("inspect path %q: %w", firstPath, firstErr)
	}
	if secondErr != nil && !os.IsNotExist(secondErr) {
		return false, fmt.Errorf("inspect path %q: %w", secondPath, secondErr)
	}
	return false, nil
}

// canonicalizePath resolves symlinks in the deepest existing ancestor, then
// reattaches any path components that do not exist yet. This lets alias checks
// work before output files are created.
func canonicalizePath(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}

	current := abs
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
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
