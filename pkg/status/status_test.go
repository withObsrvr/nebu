package status

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/withObsrvr/nebu/pkg/programstatus"
)

func TestWriterEmitsVersionedProgressRecord(t *testing.T) {
	var output bytes.Buffer
	w := NewWriter(&output, "nebu")
	w.now = func() time.Time { return time.Date(2026, 10, 11, 14, 0, 0, 0, time.UTC) }
	current, target := uint32(120), uint32(150)

	require.NoError(t, w.Write(Record{
		State:         programstatus.Working,
		CurrentLedger: &current,
		TargetLedger:  &target,
		Message:       "Fetching ledgers",
	}))

	var got Record
	require.NoError(t, json.Unmarshal(output.Bytes(), &got))
	assert.Equal(t, Schema, got.Schema)
	assert.Equal(t, "nebu", got.App)
	assert.Equal(t, programstatus.Working, got.State)
	assert.Equal(t, uint64(30), *got.LedgerLag)
	assert.Equal(t, "2026-10-11T14:00:00Z", got.Timestamp.Format(time.RFC3339))
}

func TestWriterOmitsUnknownUnboundedProgress(t *testing.T) {
	var output bytes.Buffer
	w := NewWriter(&output, "nebu")
	current := uint32(120)
	require.NoError(t, w.Write(Record{State: programstatus.Working, CurrentLedger: &current}))

	var got map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &got))
	assert.NotContains(t, got, "target_ledger")
	assert.NotContains(t, got, "ledger_lag")
}

func TestWriterEmitsEventProgress(t *testing.T) {
	var output bytes.Buffer
	w := NewWriter(&output, "json-file-sink")
	count := uint64(42)
	require.NoError(t, w.Write(Record{State: programstatus.Working, EventsProcessed: &count}))

	var got Record
	require.NoError(t, json.Unmarshal(output.Bytes(), &got))
	require.NotNil(t, got.EventsProcessed)
	assert.Equal(t, uint64(42), *got.EventsProcessed)
}

func TestWriterRejectsInvalidRecords(t *testing.T) {
	var output bytes.Buffer
	w := NewWriter(&output, "nebu")
	lag := uint64(1)
	require.ErrorContains(t, w.Write(Record{State: programstatus.Working, LedgerLag: &lag}), "requires")
	require.ErrorContains(t, w.Write(Record{State: "unknown"}), "invalid")
}

func TestWriterDerivesCLI12Lifecycle(t *testing.T) {
	tests := []struct {
		name  string
		state programstatus.State
		want  Lifecycle
	}{
		{name: "working is running", state: programstatus.Working, want: Running},
		{name: "blocked is still running", state: programstatus.Blocked, want: Running},
		{name: "done is succeeded", state: programstatus.Done, want: Succeeded},
		{name: "error is failed", state: programstatus.Error, want: Failed},
		// CLI-17 reports idle "when interrupted", which is the only case in
		// which nebu emits it, so it maps to cancelled rather than queued.
		{name: "idle is cancelled", state: programstatus.Idle, want: Cancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			w := NewWriter(&output, "nebu")
			require.NoError(t, w.Write(Record{State: tt.state}))

			var got Record
			require.NoError(t, json.Unmarshal(output.Bytes(), &got))
			assert.Equal(t, tt.state, got.State, "7501 state must be preserved on the wire")
			assert.Equal(t, tt.want, got.Lifecycle)
		})
	}
}

func TestWriterOverwritesCallerSuppliedLifecycle(t *testing.T) {
	// The two fields must never disagree, so a caller cannot set one
	// independently of the other.
	var output bytes.Buffer
	w := NewWriter(&output, "nebu")
	require.NoError(t, w.Write(Record{State: programstatus.Done, Lifecycle: Failed}))

	var got Record
	require.NoError(t, json.Unmarshal(output.Bytes(), &got))
	assert.Equal(t, Succeeded, got.Lifecycle)
}

func TestWriterAlwaysEmitsLifecycle(t *testing.T) {
	// lifecycle is required, not optional: it carries no omitempty, so it is
	// present on every record a consumer sees.
	var output bytes.Buffer
	w := NewWriter(&output, "nebu")
	require.NoError(t, w.Write(Record{State: programstatus.Working}))

	var got map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &got))
	assert.Contains(t, got, "lifecycle")
	assert.Equal(t, "running", got["lifecycle"])
}

func TestWriterRejectsUnknownStateWithoutLifecycle(t *testing.T) {
	var output bytes.Buffer
	w := NewWriter(&output, "nebu")
	err := w.Write(Record{State: programstatus.State("bogus")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
	assert.Empty(t, output.String(), "an invalid record must not be written")
}
