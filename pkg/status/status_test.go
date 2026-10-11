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
