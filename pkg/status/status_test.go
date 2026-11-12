package status

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestResolveIdentityAdoptsBeforeMinting(t *testing.T) {
	const flowctlRun = "f1000000-0000-4000-8000-000000000001"
	const nebuRun = "n1000000-0000-4000-8000-000000000002"

	tests := []struct {
		name            string
		env             map[string]string
		wantRunID       string
		wantMinted      bool
		wantComponentID string
	}{
		{
			// The orchestrator owns the correlation scope: taking its id is
			// what makes nebu's stream line up with flowctl's record of the
			// same run instead of inventing a second identity for it.
			name:            "flowctl wins",
			env:             map[string]string{EnvFlowctlRunID: flowctlRun, EnvRunID: nebuRun},
			wantRunID:       flowctlRun,
			wantComponentID: "",
		},
		{
			name:      "caller-supplied id is honoured when flowctl is absent",
			env:       map[string]string{EnvRunID: nebuRun},
			wantRunID: nebuRun,
		},
		{
			name:       "standalone mints",
			env:        nil,
			wantMinted: true,
		},
		{
			name:            "flowctl component wins",
			env:             map[string]string{EnvFlowctlComponentID: "flowctl-stage", EnvComponentID: "local-stage"},
			wantMinted:      true,
			wantComponentID: "flowctl-stage",
		},
		{
			name:            "caller-supplied component",
			env:             map[string]string{EnvComponentID: "local-stage"},
			wantMinted:      true,
			wantComponentID: "local-stage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear every input so a value leaking in from the developer's
			// own shell cannot make this pass or fail by accident.
			for _, key := range []string{EnvFlowctlRunID, EnvFlowctlComponentID, EnvRunID, EnvComponentID} {
				t.Setenv(key, "")
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			got := ResolveIdentity()

			if tt.wantMinted {
				_, err := uuid.Parse(got.RunID)
				require.NoError(t, err, "a minted run id must be the same UUID shape flowctl produces")
			} else {
				assert.Equal(t, tt.wantRunID, got.RunID)
			}
			assert.Equal(t, tt.wantComponentID, got.ComponentID)
		})
	}
}

func TestResolveIdentityMintsADistinctIDPerCall(t *testing.T) {
	// Two concurrent runs of the same processor have to be tellable apart;
	// that is the whole point of carrying an id.
	for _, key := range []string{EnvFlowctlRunID, EnvRunID} {
		t.Setenv(key, "")
	}

	seen := map[string]bool{}
	for range 50 {
		id := ResolveIdentity().RunID
		require.NotEmpty(t, id)
		require.False(t, seen[id], "minted run id %q repeated", id)
		seen[id] = true
	}
}

func TestWriterStampsIdentityOnEveryRecord(t *testing.T) {
	var output bytes.Buffer
	identity := Identity{RunID: "run-1", ComponentID: "stage-1"}
	w := NewWriterWithIdentity(&output, "nebu", identity)

	require.NoError(t, w.Write(Record{State: programstatus.Working}))
	require.NoError(t, w.Write(Record{State: programstatus.Done}))

	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	count := 0
	for decoder.More() {
		var record Record
		require.NoError(t, decoder.Decode(&record))
		assert.Equal(t, "run-1", record.RunID)
		assert.Equal(t, "stage-1", record.ComponentID)
		count++
	}
	assert.Equal(t, 2, count, "both records must carry the identity, not just the first")
}

func TestWriterOverwritesCallerSuppliedIdentity(t *testing.T) {
	// Identity belongs to the writer, like app and ts. A caller setting it
	// per record could otherwise split one run across several ids.
	var output bytes.Buffer
	w := NewWriterWithIdentity(&output, "nebu", Identity{RunID: "run-1", ComponentID: "stage-1"})

	require.NoError(t, w.Write(Record{
		State:       programstatus.Working,
		RunID:       "someone-elses-run",
		ComponentID: "someone-elses-stage",
	}))

	var record Record
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	assert.Equal(t, "run-1", record.RunID)
	assert.Equal(t, "stage-1", record.ComponentID)
}

func TestWriterAlwaysCarriesARunID(t *testing.T) {
	// However the writer was built. A consumer that has to handle a missing
	// run id gains nothing from the field existing.
	var output bytes.Buffer
	w := NewWriterWithIdentity(&output, "nebu", Identity{})

	require.NoError(t, w.Write(Record{State: programstatus.Working}))

	var record Record
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	_, err := uuid.Parse(record.RunID)
	assert.NoError(t, err)
	assert.Empty(t, record.ComponentID, "a standalone run has no component to name")
}

func TestWriterIdentityIsReadable(t *testing.T) {
	// A caller reporting the run id elsewhere must read the stamped value
	// rather than resolving again and getting a different one.
	var output bytes.Buffer
	w := NewWriter(&output, "nebu")

	require.NoError(t, w.Write(Record{State: programstatus.Working}))

	var record Record
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	assert.Equal(t, w.Identity().RunID, record.RunID)

	var nilWriter *Writer
	assert.Empty(t, nilWriter.Identity().RunID, "a nil writer must not panic")
}
