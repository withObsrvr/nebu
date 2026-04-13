package hooks

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/withObsrvr/nebu/pkg/runtime"
)

// RateLimitHook returns a runtime.Hooks bundle that throttles ledger
// processing to at most `ledgersPerSecond` ledgers per second, using
// a simple token-bucket algorithm in BeforeLedger.
//
// Unlike --follow-style RPC throttling (which happens at the source),
// this hook gates the processor itself — useful when your downstream
// sink has a throughput limit (e.g. a Postgres upsert budget, a
// webhook target with a published rate limit) and you need the whole
// pipeline to back-pressure accordingly.
//
// # Behavior
//
//   - ledgersPerSecond <= 0: no throttling (returns a zero-value
//     Hooks{} that no-ops).
//   - ledgersPerSecond > 0: tokens refill at the configured rate,
//     capped at a burst of 1. BeforeLedger blocks until a token is
//     available or ctx is canceled.
//
// If ctx is canceled while waiting, BeforeLedger returns
// ctx.Err() and the runtime aborts the pipeline with that error.
//
// # Example
//
//	// In your cmd/<processor>/main.go:
//	cfg := cli.OriginConfig{
//	    Processor: myProcessor,
//	    Hooks: []runtime.Hooks{
//	        hooks.RateLimitHook(50), // max 50 ledgers/sec
//	    },
//	}
//
// # Why BeforeLedger
//
// Putting the throttle in BeforeLedger (rather than in the processor
// itself) means it applies uniformly regardless of which processor
// is running. Drop this file into any processor's directory and get
// the same behavior.
//
// # Copy this file
//
// This is intentionally self-contained with no dependencies outside
// the Go standard library. Copy rate_limit.go into your own
// processor's cmd/<name>/ directory (rename `package hooks` to your
// package, usually `main`) and call RateLimitHook() from your
// OriginConfig wiring. That's the whole pattern.
func RateLimitHook(ledgersPerSecond float64) runtime.Hooks {
	if ledgersPerSecond <= 0 {
		return runtime.Hooks{}
	}

	interval := time.Duration(float64(time.Second) / ledgersPerSecond)
	next := time.Now()

	return runtime.Hooks{
		BeforeLedger: func(ctx context.Context, _ xdr.LedgerCloseMeta) error {
			now := time.Now()
			if now.Before(next) {
				wait := next.Sub(now)
				select {
				case <-ctx.Done():
					return fmt.Errorf("rate limiter canceled: %w", ctx.Err())
				case <-time.After(wait):
				}
			}
			// Advance the schedule by one interval. Using `next` as the
			// anchor (rather than time.Now()) smooths out bursts — if
			// ProcessLedger takes longer than `interval`, we don't add
			// extra delay on the next call.
			next = next.Add(interval)
			// But don't let `next` fall too far behind realtime; cap at
			// the current wall clock to prevent a runaway burst if the
			// processor stalls for minutes.
			if cur := time.Now(); next.Before(cur) {
				next = cur.Add(interval)
			}
			return nil
		},
	}
}
