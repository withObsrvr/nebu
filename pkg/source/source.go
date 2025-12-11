// Package source provides abstractions for streaming Stellar ledger data.
package source

import (
	"context"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// LedgerSource streams LedgerCloseMeta objects from a Stellar data source.
// Implementations must be safe for concurrent use.
type LedgerSource interface {
	// Stream sends ledgers in the range [start, end] (inclusive) to the out channel.
	// The implementation must close the out channel when done or on error.
	//
	// Stream respects context cancellation and will stop streaming if ctx is cancelled.
	// Returns an error if the range cannot be prepared or if ledger retrieval fails.
	Stream(ctx context.Context, start, end uint32, out chan<- xdr.LedgerCloseMeta) error

	// Close releases any resources held by the source.
	Close() error
}
