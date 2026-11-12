package storage

import (
	"context"
	"testing"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/support/datastore"
	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestNewLedgerSource_Validation(t *testing.T) {
	tests := []struct {
		name           string
		config         datastore.DataStoreConfig
		wantErr        bool
		wantErrMessage string
	}{
		{
			name: "empty type",
			config: datastore.DataStoreConfig{
				Type: "",
			},
			wantErr:        true,
			wantErrMessage: "datastore type cannot be empty",
		},
		{
			// Construction only validates what it can answer locally.
			// Reaching the bucket — and reading the layout from its
			// manifest — happens on the first pull of a Stream call, so an
			// unreachable bucket is reported there rather than here. The
			// SDK's stream owns that lifecycle; see Stream.
			name: "unreachable bucket defers its error to streaming",
			config: datastore.DataStoreConfig{
				Type: "GCS",
				Params: map[string]string{
					"destination_bucket_path": "test-bucket-does-not-exist/path",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bufferConfig := ledgerbackend.BufferedStorageBackendConfig{
				BufferSize: 10,
				NumWorkers: 2,
				RetryLimit: 0,
				RetryWait:  0,
			}

			source, err := NewLedgerSource(tt.config, bufferConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLedgerSource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErrMessage != "" && err != nil {
				if err.Error() != tt.wantErrMessage {
					t.Errorf("NewLedgerSource() error message = %v, want %v", err.Error(), tt.wantErrMessage)
				}
			}

			if err == nil {
				if cerr := source.Close(); cerr != nil {
					t.Errorf("Close() on a source that never streamed = %v, want nil", cerr)
				}
			}
		})
	}
}

func TestStreamRejectsInvalidRangeBeforeTouchingTheDatastore(t *testing.T) {
	// A range error must not depend on the bucket being reachable: it is a
	// caller mistake, and reporting it needs no network.
	source, err := NewLedgerSource(datastore.DataStoreConfig{
		Type:   "GCS",
		Params: map[string]string{"destination_bucket_path": "test-bucket-does-not-exist/path"},
	}, DefaultBufferedStorageBackendConfig())
	if err != nil {
		t.Fatalf("NewLedgerSource() = %v", err)
	}

	out := make(chan xdr.LedgerCloseMeta, 1)
	if err := source.Stream(context.Background(), 0, 10, out); err == nil {
		t.Fatal("Stream() with a zero start ledger = nil, want an error")
	}

	if _, open := <-out; open {
		t.Error("Stream() must close the output channel on a range error")
	}
}

func TestDefaultBufferedStorageBackendConfig(t *testing.T) {
	config := DefaultBufferedStorageBackendConfig()

	if config.BufferSize != 100 {
		t.Errorf("DefaultBufferedStorageBackendConfig().BufferSize = %d, want 100", config.BufferSize)
	}

	if config.NumWorkers != 10 {
		t.Errorf("DefaultBufferedStorageBackendConfig().NumWorkers = %d, want 10", config.NumWorkers)
	}

	if config.RetryLimit != 3 {
		t.Errorf("DefaultBufferedStorageBackendConfig().RetryLimit = %d, want 3", config.RetryLimit)
	}
}

func TestDefaultDataStoreSchema(t *testing.T) {
	schema := DefaultDataStoreSchema()

	if schema.LedgersPerFile != 1 {
		t.Errorf("DefaultDataStoreSchema().LedgersPerFile = %d, want 1", schema.LedgersPerFile)
	}

	if schema.FilesPerPartition != 64000 {
		t.Errorf("DefaultDataStoreSchema().FilesPerPartition = %d, want 64000", schema.FilesPerPartition)
	}
}
