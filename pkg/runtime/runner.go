// Package runtime provides the core execution engine for nebu pipelines.
package runtime

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/source"
)

// Runtime is the execution engine that wires sources and processors together.
type Runtime struct {
	// Future: add logger, metrics, config options
}

// NewRuntime creates a new runtime instance.
func NewRuntime() *Runtime {
	return &Runtime{}
}

// RunOrigin streams ledgers from a source into an origin processor.
// This is the simplest execution mode - just source → origin.
//
// The function will:
//   - Stream ledgers from the source in the range [start, end]
//   - Call origin.ProcessLedger() for each ledger
//   - Stop on error or context cancellation
//   - Return the first error encountered
//
// Example usage:
//
//	rt := runtime.NewRuntime()
//	src, _ := source.NewRPCLedgerSource("https://mainnet.sorobanrpc.com")
//	origin := &MyOriginProcessor{}
//
//	err := rt.RunOrigin(ctx, src, origin, 100, 200)
func (r *Runtime) RunOrigin(
	ctx context.Context,
	src source.LedgerSource,
	origin processor.Origin,
	start, end uint32,
) error {
	// Create a channel for ledger streaming
	// Buffer size of 128 gives some breathing room for rate differences
	ledgerCh := make(chan xdr.LedgerCloseMeta, 128)

	// Stream errors channel
	errCh := make(chan error, 1)

	// Start the source streaming in a goroutine
	go func() {
		err := src.Stream(ctx, start, end, ledgerCh)
		if err != nil && err != context.Canceled {
			errCh <- fmt.Errorf("source stream error: %w", err)
		}
		close(errCh)
	}()

	// Process ledgers as they arrive
	for {
		select {
		case <-ctx.Done():
			// Context cancelled - stop processing
			return ctx.Err()

		case err, ok := <-errCh:
			if ok && err != nil {
				// Source encountered an error
				return err
			}
			// errCh closed with no error - continue processing ledgers

		case ledger, ok := <-ledgerCh:
			if !ok {
				// Channel closed - stream complete
				// Check if there was a final error
				if err := <-errCh; err != nil {
					return err
				}
				return nil
			}

			// Process the ledger through the origin
			if err := origin.ProcessLedger(ctx, ledger); err != nil {
				return fmt.Errorf("processor error at ledger %d: %w", ledger.LedgerSequence(), err)
			}
		}
	}
}
