// Package main demonstrates a simple origin processor that counts ledgers.
//
// This example shows the core nebu flow:
//  1. Create an RPC ledger source
//  2. Implement a simple origin processor
//  3. Wire them together with the runtime
//  4. Process ledgers from Stellar mainnet
//
// Run with: go run examples/simple_origin/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
	"github.com/withObsrvr/nebu/pkg/source"
)

// CountingOrigin is a simple origin processor that counts and prints ledgers.
type CountingOrigin struct {
	count int
}

// NewCountingOrigin creates a new counting origin processor.
func NewCountingOrigin() *CountingOrigin {
	return &CountingOrigin{}
}

// Name implements processor.Processor.
func (o *CountingOrigin) Name() string {
	return "counting-origin"
}

// Type implements processor.Processor.
func (o *CountingOrigin) Type() processor.Type {
	return processor.TypeOrigin
}

// ProcessLedger implements processor.Origin.
// It prints information about each ledger and increments the counter.
func (o *CountingOrigin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
	o.count++

	seq := ledger.LedgerSequence()
	closeTime := time.Unix(int64(ledger.LedgerCloseTime()), 0)
	txCount := len(ledger.TransactionEnvelopes())

	fmt.Printf("[%d] Ledger %d (closed at %s) - %d transactions\n",
		o.count, seq, closeTime.Format(time.RFC3339), txCount)

	return nil
}

func main() {
	// Configuration
	rpcURL := getEnv("NEBU_RPC_URL", "https://archive-rpc.lightsail.network")
	startLedger := uint32(60200000)
	endLedger := uint32(60200009) // Process 10 ledgers

	fmt.Println("=== nebu Simple Origin Example ===")
	fmt.Printf("RPC URL: %s\n", rpcURL)
	fmt.Printf("Ledger range: %d to %d\n\n", startLedger, endLedger)

	// Create context with cancellation support
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n\nReceived interrupt signal, shutting down...")
		cancel()
	}()

	// Create the RPC ledger source
	src, err := source.NewRPCLedgerSource(rpcURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create RPC source: %v\n", err)
		os.Exit(1)
	}
	defer src.Close()

	// Create the origin processor
	origin := NewCountingOrigin()

	// Create the runtime
	rt := runtime.NewRuntime()

	// Run!
	fmt.Println("Starting to process ledgers...")
	start := time.Now()

	err = rt.RunOrigin(ctx, src, origin, startLedger, endLedger)

	duration := time.Since(start)

	if err != nil {
		if err == context.Canceled {
			fmt.Println("\nProcessing cancelled by user")
		} else {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			os.Exit(1)
		}
	}

	// Print summary
	fmt.Println("\n=== Summary ===")
	fmt.Printf("Processed %d ledgers in %v\n", origin.count, duration.Round(time.Millisecond))
	fmt.Printf("Average: %.2f ledgers/sec\n", float64(origin.count)/duration.Seconds())
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
