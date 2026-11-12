// Package status writes the versioned, machine-readable nebu status stream.
package status

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

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
	Lifecycle Lifecycle `json:"lifecycle"`
	App       string    `json:"app"`
	// RunID correlates every record a run emits, and correlates them with
	// the records emitted by the other stages of the same pipeline. Set on
	// write from the writer's [Identity]; any value a caller sets is
	// overwritten. Always present and never empty.
	RunID string `json:"run_id"`
	// ComponentID names this process's position in a pipeline, when
	// something upstream knows it. Absent for a standalone run, where
	// `app` is already the only name there is. Set on write, like RunID.
	ComponentID     string    `json:"component_id,omitempty"`
	Timestamp       time.Time `json:"ts"`
	CurrentLedger   *uint32   `json:"current_ledger,omitempty"`
	TargetLedger    *uint32   `json:"target_ledger,omitempty"`
	LedgerLag       *uint64   `json:"ledger_lag,omitempty"`
	EventsProcessed *uint64   `json:"events_processed,omitempty"`
	Message         string    `json:"message,omitempty"`
}

// Environment variables that carry a run's identity into nebu.
const (
	// EnvFlowctlRunID is the pipeline run flowctl exports to every child
	// process it starts.
	EnvFlowctlRunID = "FLOWCTL_RUN_ID"
	// EnvFlowctlComponentID is the pipeline component flowctl exports
	// alongside it.
	EnvFlowctlComponentID = "FLOWCTL_COMPONENT_ID"
	// EnvRunID lets a caller that is not flowctl supply the run id — a UI
	// composing `nebu fetch | processor | sink` itself, for instance, which
	// needs one id across three processes that cannot otherwise know they
	// belong together.
	EnvRunID = "NEBU_RUN_ID"
	// EnvComponentID is the standalone equivalent of EnvFlowctlComponentID.
	// It is per-process: exporting one value for a whole shell pipeline
	// labels every stage the same and defeats the purpose.
	EnvComponentID = "NEBU_COMPONENT_ID"
)

// Identity correlates the records of one run, and the runs of one pipeline.
//
// Without it a consumer following several streams cannot tell two concurrent
// invocations of the same processor apart, and cannot tell that the three
// stages of one pipeline belong together.
type Identity struct {
	// RunID identifies the run. Never empty on a resolved Identity.
	RunID string
	// ComponentID identifies this process within a pipeline, when known.
	ComponentID string
}

// ResolveIdentity adopts the identity of whatever is running nebu, and mints
// one only when nothing is.
//
// Adopting comes first because the orchestrator owns the correlation scope:
// flowctl already mints a run id per pipeline run and exports it to every
// child it starts, so taking that value is what makes nebu's status stream
// line up with flowctl's record of the same run instead of inventing a second
// identity for it. A caller-supplied NEBU_RUN_ID is honoured next, for
// pipelines nebu is not being orchestrated into. Only a genuinely standalone
// run mints, and it mints the same UUID shape flowctl does, so a consumer
// never has to care which case it got.
func ResolveIdentity() Identity {
	return Identity{
		RunID: firstNonEmpty(
			os.Getenv(EnvFlowctlRunID),
			os.Getenv(EnvRunID),
			uuid.NewString(),
		),
		ComponentID: firstNonEmpty(
			os.Getenv(EnvFlowctlComponentID),
			os.Getenv(EnvComponentID),
		),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// Writer serializes status records as newline-delimited JSON.
type Writer struct {
	mu       sync.Mutex
	encoder  *json.Encoder
	app      string
	identity Identity
	now      func() time.Time
}

// NewWriter returns a status writer for app, resolving the run identity from
// the environment. See [ResolveIdentity].
func NewWriter(w io.Writer, app string) *Writer {
	return NewWriterWithIdentity(w, app, ResolveIdentity())
}

// NewWriterWithIdentity returns a status writer for app carrying an explicit
// identity. A run id is minted if identity does not carry one, so that every
// record a writer produces has one however the writer was built.
func NewWriterWithIdentity(w io.Writer, app string, identity Identity) *Writer {
	if identity.RunID == "" {
		identity.RunID = uuid.NewString()
	}
	return &Writer{encoder: json.NewEncoder(w), app: app, identity: identity, now: time.Now}
}

// Identity returns the identity this writer stamps on every record. A caller
// that needs to report the run id elsewhere — a log line, an API response —
// reads it here rather than resolving it a second time and getting a
// different answer.
func (w *Writer) Identity() Identity {
	if w == nil {
		return Identity{}
	}
	return w.identity
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
	record.RunID = w.identity.RunID
	record.ComponentID = w.identity.ComponentID
	record.Timestamp = w.now().UTC()

	w.mu.Lock()
	defer w.mu.Unlock()
	return w.encoder.Encode(record)
}
