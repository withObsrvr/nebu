package source

import (
	"testing"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/support/datastore"
)

func TestNewStorageLedgerSource_Validation(t *testing.T) {
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
			name: "GCS config with non-existent bucket",
			config: datastore.DataStoreConfig{
				Type: "GCS",
				Params: map[string]string{
					"destination_bucket_path": "test-bucket-does-not-exist/path",
				},
				Schema: datastore.DataStoreSchema{
					LedgersPerFile:    1,
					FilesPerPartition: 64000,
				},
			},
			// Will fail because bucket doesn't exist or no access
			// This is expected - we're validating the error handling works
			wantErr: true,
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

			_, err := NewStorageLedgerSource(tt.config, bufferConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewStorageLedgerSource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErrMessage != "" && err != nil {
				if err.Error() != tt.wantErrMessage {
					t.Errorf("NewStorageLedgerSource() error message = %v, want %v", err.Error(), tt.wantErrMessage)
				}
			}
		})
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
