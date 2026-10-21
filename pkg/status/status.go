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

// Lifecycle is the operation-lifecycle vocabulary defined by CLI-12 of the
// OBSRVR Engineering Standard. It is carried alongside [Record.State] so one
// consumer can read nebu, flowctl and the obsrvr CLI with a single state
// machine, without nebu having to rename the Program Status Protocol tokens
// that CLI-17 requires on the wire.
type Lifecycle string

const (
	// Queued is declared for vocabulary completeness and is never emitted by
	// nebu: nebu runs work in the caller's own process and has no queue. A
	// consumer that handles it is ready for tools that do.
	Queued Lifecycle = "queued"
	// Running covers work in progress, including work that is blocked waiting
	// on input — a blocked operation has not terminated.
	Running Lifecycle = "running"
	// Succeeded is terminal: the operation completed its requested work.
	Succeeded Lifecycle = "succeeded"
	// Failed is terminal: the operation stopped on an error.
	Failed Lifecycle = "failed"
	// Cancelled is terminal: the operation stopped on interruption.
	Cancelled Lifecycle = "cancelled"
)

// lifecycleFor maps a Program Status Protocol state to its CLI-12 lifecycle
// equivalent. The mapping is total over the states [Writer.Write] accepts, so
// every record carries a lifecycle.
//
// Note that programstatus.Idle maps to Cancelled rather than to anything
// idle-sounding: per CLI-17 a tool reports idle "when interrupted", and that
// is the only circumstance in which nebu emits it.
func lifecycleFor(state programstatus.State) (Lifecycle, bool) {
	switch state {
	case programstatus.Working, programstatus.Blocked:
		return Running, true
	case programstatus.Done:
		return Succeeded, true
	case programstatus.Error:
		return Failed, true
	case programstatus.Idle:
		return Cancelled, true
	default:
		return "", false
	}
}

// Record is one lifecycle or progress update in the nebu status stream.
type Record struct {
	Schema string `json:"_schema"`
	// State is the Program Status Protocol (OSC 7501) state token.
	State programstatus.State `json:"state"`
	// Lifecycle is the CLI-12 equivalent of State. It is derived on write and
	// any value set by the caller is overwritten, so the two fields can never
	// disagree.
	Lifecycle       Lifecycle `json:"lifecycle"`
	App             string    `json:"app"`
	Timestamp       time.Time `json:"ts"`
	CurrentLedger   *uint32   `json:"current_ledger,omitempty"`
	TargetLedger    *uint32   `json:"target_ledger,omitempty"`
	LedgerLag       *uint64   `json:"ledger_lag,omitempty"`
	EventsProcessed *uint64   `json:"events_processed,omitempty"`
	Message         string    `json:"message,omitempty"`
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
	lifecycle, ok := lifecycleFor(record.State)
	if !ok {
		return fmt.Errorf("invalid structured status state %q", record.State)
	}
	record.Lifecycle = lifecycle
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
