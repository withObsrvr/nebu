package source

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/support/datastore"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// StorageLedgerSource streams ledgers from GCS/S3 storage backends.
type StorageLedgerSource struct {
	backend   ledgerbackend.LedgerBackend
	dataStore datastore.DataStore
}

// NewStorageLedgerSource creates a source from a storage datastore.
func NewStorageLedgerSource(
	datastoreConfig datastore.DataStoreConfig,
	bufferConfig ledgerbackend.BufferedStorageBackendConfig,
) (*StorageLedgerSource, error) {
	// Validate configuration
	if datastoreConfig.Type == "" {
		return nil, fmt.Errorf("datastore type cannot be empty")
	}

	// Create datastore
	ctx := context.Background()
	store, err := datastore.NewDataStore(ctx, datastoreConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create datastore: %w", err)
	}

	// Create buffered storage backend
	backend, err := ledgerbackend.NewBufferedStorageBackend(bufferConfig, store, datastoreConfig.Schema)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to create buffered backend: %w", err)
	}

	return &StorageLedgerSource{
		backend:   backend,
		dataStore: store,
	}, nil
}

// Stream implements LedgerSource.Stream.
// Supports both bounded and unbounded ranges:
//   - Bounded: start > 0, end > 0 (streams ledgers from start to end)
//   - Unbounded: start > 0, end == 0 (streams ledgers from start continuously)
func (s *StorageLedgerSource) Stream(
	ctx context.Context,
	start, end uint32,
	out chan<- xdr.LedgerCloseMeta,
) error {
	defer close(out)

	// Validate range
	if start == 0 {
		return fmt.Errorf("start ledger must be > 0 (got %d)", start)
	}

	// Validate bounded range
	if end > 0 && start > end {
		return fmt.Errorf("invalid range: start (%d) must be <= end (%d)", start, end)
	}

	// Prepare range (bounded or unbounded)
	var rang ledgerbackend.Range
	if end == 0 {
		// Unbounded range - stream continuously from start
		rang = ledgerbackend.UnboundedRange(start)
		log.Infof("Preparing unbounded range starting at ledger %d", start)
	} else {
		// Bounded range - stream from start to end
		rang = ledgerbackend.BoundedRange(start, end)
		log.Infof("Preparing bounded range [%d, %d]", start, end)
	}

	if err := s.backend.PrepareRange(ctx, rang); err != nil {
		log.Errorf("failed to prepare range: %v", err)
		return fmt.Errorf("failed to prepare range: %w", err)
	}

	// Stream ledgers
	seq := start
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ledger, err := s.backend.GetLedger(ctx, seq)
		if err != nil {
			return fmt.Errorf("failed to get ledger %d: %w", seq, err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- ledger:
			// Ledger sent successfully
		}

		seq++

		// Stop if bounded range and reached end
		if end > 0 && seq > end {
			return nil
		}
	}
}

// Close implements LedgerSource.Close.
func (s *StorageLedgerSource) Close() error {
	var backendErr, datastoreErr error

	if s.backend != nil {
		backendErr = s.backend.Close()
	}

	if s.dataStore != nil {
		datastoreErr = s.dataStore.Close()
	}

	// Return first error encountered
	if backendErr != nil {
		return backendErr
	}
	return datastoreErr
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
