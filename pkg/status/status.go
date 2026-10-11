// Package status writes the versioned, machine-readable nebu status stream.
package status

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/withObsrvr/nebu/pkg/programstatus"
)

// Schema identifies the current NDJSON status record contract.
const Schema = "nebu.status.v1"

// Record is one lifecycle or progress update in the nebu status stream.
type Record struct {
	Schema          string              `json:"_schema"`
	State           programstatus.State `json:"state"`
	App             string              `json:"app"`
	Timestamp       time.Time           `json:"ts"`
	CurrentLedger   *uint32             `json:"current_ledger,omitempty"`
	TargetLedger    *uint32             `json:"target_ledger,omitempty"`
	LedgerLag       *uint64             `json:"ledger_lag,omitempty"`
	EventsProcessed *uint64             `json:"events_processed,omitempty"`
	Message         string              `json:"message,omitempty"`
}

// Writer serializes status records as newline-delimited JSON.
type Writer struct {
	mu      sync.Mutex
	encoder *json.Encoder
	app     string
	now     func() time.Time
}

// NewWriter returns a status writer for app.
func NewWriter(w io.Writer, app string) *Writer {
	return &Writer{encoder: json.NewEncoder(w), app: app, now: time.Now}
}

// Write validates and writes one status record.
func (w *Writer) Write(record Record) error {
	if w == nil {
		return nil
	}
	switch record.State {
	case programstatus.Idle, programstatus.Working, programstatus.Done,
		programstatus.Blocked, programstatus.Error:
	default:
		return fmt.Errorf("invalid structured status state %q", record.State)
	}
	if record.TargetLedger != nil && record.CurrentLedger != nil {
		lag := uint64(0)
		if *record.TargetLedger > *record.CurrentLedger {
			lag = uint64(*record.TargetLedger - *record.CurrentLedger)
		}
		record.LedgerLag = &lag
	} else if record.LedgerLag != nil {
		return fmt.Errorf("structured status ledger_lag requires current_ledger and target_ledger")
	}
	record.Schema = Schema
	record.App = w.app
	record.Timestamp = w.now().UTC()

	w.mu.Lock()
	defer w.mu.Unlock()
	return w.encoder.Encode(record)
}
