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
	"github.com/withObsrvr/nebu/pkg/source"
	"github.com/withObsrvr/nebu/pkg/source/internal/ledgerstream"
)

// LedgerSource streams ledgers from GCS/S3 storage backends.
type LedgerSource struct {
	stream ledgerbackend.LedgerStream
	// config is retained so LedgerWindow can open the datastore to ask what
	// it holds. The stream opens its own on the first pull and does not
	// expose it, and it has no window query of its own.
	config datastore.DataStoreConfig
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
		config: datastoreConfig,
	}, nil
}

// LedgerWindow implements source.WindowedSource.
//
// The window is what has been *exported*, not what exists on the network. An
// exporter runs behind the tip, so "the last 100 ledgers" from an archive is
// a different hundred than from an RPC endpoint — which is why a caller is
// expected to report the range it resolved rather than just the count.
//
// This opens its own datastore: the stream opens one on the first pull and
// does not expose it. Finding the oldest ledger is a binary search over the
// object listing, so this costs a bounded handful of round trips — paid once
// at startup, against a backfill that will run far longer.
func (s *LedgerSource) LedgerWindow(ctx context.Context) (source.LedgerWindow, error) {
	store, err := datastore.NewDataStore(ctx, s.config)
	if err != nil {
		return source.LedgerWindow{}, fmt.Errorf("open datastore: %w", err)
	}
	defer func() {
		// Nothing actionable: the window has already been read, and the
		// stream opens its own store for the actual work.
		_ = store.Close()
	}()

	// The schema has to come from the manifest for the object keys to be
	// right, the same way the stream resolves it.
	schema, err := datastore.LoadSchema(ctx, store, s.config)
	if err != nil {
		return source.LedgerWindow{}, fmt.Errorf("load datastore schema: %w", err)
	}

	latest, err := datastore.FindLatestLedgerSequence(ctx, store)
	if err != nil {
		return source.LedgerWindow{}, fmt.Errorf("find latest exported ledger: %w", err)
	}
	oldest, err := datastore.FindOldestLedgerSequence(ctx, store, schema)
	if err != nil {
		return source.LedgerWindow{}, fmt.Errorf("find oldest exported ledger: %w", err)
	}

	return source.LedgerWindow{Oldest: oldest, Latest: latest}, nil
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

// assert at compile time that the source can report its window.
var _ source.WindowedSource = (*LedgerSource)(nil)
