// Package storage provides a LedgerSource implementation backed by a
// Stellar datastore (GCS, S3, FS).
//
// This package implements the source.LedgerSource interface from the parent
// pkg/source package. Importers pay the full cost of the Stellar datastore
// and cloud SDK dependencies (AWS SDK, GCS client); code that only needs
// the interface should import pkg/source directly.
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/support/datastore"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/source/internal/ledgerstream"
)

// LedgerSource streams ledgers from GCS/S3 storage backends.
type LedgerSource struct {
	stream ledgerbackend.LedgerStream
}

// NewLedgerSource creates a source from a storage datastore.
func NewLedgerSource(
	datastoreConfig datastore.DataStoreConfig,
	bufferConfig ledgerbackend.BufferedStorageBackendConfig,
) (*LedgerSource, error) {
	// Validate configuration
	if datastoreConfig.Type == "" {
		return nil, fmt.Errorf("datastore type cannot be empty")
	}

	// NewBufferedStorageStream opens the datastore on the first pull rather
	// than here, and reads the layout from the manifest in the bucket at the
	// same time. That is the reason this source goes through the stream: the
	// backend it replaces took the schema from the caller and never checked
	// it, so a bucket exported with any other layout was read with the wrong
	// object keys instead of reporting a mismatch.
	return &LedgerSource{
		stream: ledgerbackend.NewBufferedStorageStream(bufferConfig, datastoreConfig, nil),
	}, nil
}

// Stream implements source.LedgerSource.Stream.
// Supports both bounded and unbounded ranges:
//   - Bounded: start > 0, end > 0 (streams ledgers from start to end)
//   - Unbounded: start > 0, end == 0 (streams ledgers from start continuously)
func (s *LedgerSource) Stream(
	ctx context.Context,
	start, end uint32,
	out chan<- xdr.LedgerCloseMeta,
) error {
	defer close(out)

	ledgerRange, err := ledgerstream.Range(start, end)
	if err != nil {
		return err
	}

	return ledgerstream.Pump(ctx, s.stream, ledgerRange, out)
}

// Close implements source.LedgerSource.Close.
//
// There is nothing to release: a LedgerStream opens the datastore on the
// first pull of a Stream call and closes it when that iteration ends, so a
// source that is not streaming holds nothing open. Close remains part of the
// contract and stays safe to call at any time.
func (s *LedgerSource) Close() error {
	return nil
}

// DefaultBufferedStorageBackendConfig returns sensible defaults for buffered storage.
func DefaultBufferedStorageBackendConfig() ledgerbackend.BufferedStorageBackendConfig {
	return ledgerbackend.BufferedStorageBackendConfig{
		BufferSize: 100,
		NumWorkers: 10,
		RetryLimit: 3,
		RetryWait:  5 * time.Second,
	}
}

// DefaultDataStoreSchema returns sensible defaults for datastore schema.
func DefaultDataStoreSchema() datastore.DataStoreSchema {
	return datastore.DataStoreSchema{
		LedgersPerFile:    1,
		FilesPerPartition: 64000,
	}
}
