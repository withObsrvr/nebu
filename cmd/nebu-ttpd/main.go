// Package main implements nebu-ttpd, the Token Transfer Processor daemon.
//
// nebu-ttpd is a standalone HTTP service that streams Stellar token transfer events.
// It wraps Stellar's token_transfer.EventsProcessor and exposes events over HTTP.
//
// Usage:
//
//	nebu-ttpd
//
// Environment variables:
//   - NEBU_RPC_URL: Stellar RPC endpoint (default: https://archive-rpc.lightsail.network)
//   - NEBU_LISTEN_ADDR: HTTP listen address (default: :8080)
//   - NEBU_NETWORK: Network passphrase (default: Public Global Stellar Network ; September 2015)
//
// API Endpoints:
//   - GET /events?start=X&end=Y: Stream token transfer events as newline-delimited JSON
//   - GET /health: Health check
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stellar/go-stellar-sdk/network"
	token_transfer "github.com/withObsrvr/nebu/examples/processors/token-transfer"
	"github.com/withObsrvr/nebu/pkg/source"
)

func main() {
	// Configuration from environment
	rpcURL := getEnv("NEBU_RPC_URL", "https://archive-rpc.lightsail.network")
	listenAddr := getEnv("NEBU_LISTEN_ADDR", ":8080")
	networkPassphrase := getEnv("NEBU_NETWORK", network.PublicNetworkPassphrase)

	log.Printf("nebu-ttpd starting...")
	log.Printf("  RPC URL: %s", rpcURL)
	log.Printf("  Listen: %s", listenAddr)
	log.Printf("  Network: %s", networkPassphrase)

	// Create RPC ledger source
	src, err := source.NewRPCLedgerSource(rpcURL)
	if err != nil {
		log.Fatalf("Failed to create RPC source: %v", err)
	}
	defer src.Close()

	// Create origin processor (used as template for per-request processors)
	origin := token_transfer.NewOrigin(networkPassphrase)

	// Create HTTP server
	ttpServer := token_transfer.NewServer(src, origin)

	mux := http.NewServeMux()
	ttpServer.RegisterRoutes(mux)
	mux.HandleFunc("/health", token_transfer.HealthCheck())

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // No timeout for streaming responses
		IdleTimeout:  120 * time.Second,
	}

	// Start server in background
	go func() {
		log.Printf("Server listening on %s", listenAddr)
		log.Printf("  GET /events?start=X&end=Y - Stream token transfer events")
		log.Printf("  GET /health - Health check")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down gracefully...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
