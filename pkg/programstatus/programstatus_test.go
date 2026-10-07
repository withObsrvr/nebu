package programstatus

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite(t *testing.T) {
	progress := 40
	tests := []struct {
		name    string
		report  Report
		want    string
		wantErr string
	}{
		{
			name:   "working root record",
			report: Report{State: Working, App: "nebu", Message: "Fetching ledgers"},
			want:   "\x1b]7501;state=working:app=nebu:msg=RmV0Y2hpbmcgbGVkZ2Vycw==\x1b\\",
		},
		{
			name:   "blocked child record",
			report: Report{State: Blocked, ID: "fetch/archive", Kind: "auth", Progress: &progress},
			want:   "\x1b]7501;state=blocked:id=fetch/archive:kind=auth:progress=40\x1b\\",
		},
		{name: "invalid state", report: Report{State: "busy"}, wantErr: "invalid program status state"},
		{name: "invalid id", report: Report{State: Working, ID: "bad id"}, wantErr: "invalid program status id"},
		{name: "invalid progress", report: Report{State: Working, Progress: intPointer(101)}, wantErr: "outside 0..100"},
		{name: "control character", report: Report{State: Error, Message: "bad\nmessage"}, wantErr: "control character"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bytes.Buffer
			err := Write(&got, tt.report)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Empty(t, got.String())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}

func TestWriteReturnsWriterError(t *testing.T) {
	err := Write(failingWriter{}, Report{State: Done})
	require.ErrorContains(t, err, "write failed")
}

func intPointer(value int) *int { return &value }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
