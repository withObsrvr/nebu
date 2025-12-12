// Package main implements a standalone DuckDB sink that reads JSON events from stdin.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/marcboeker/go-duckdb"
)

func main() {
	dbPath := flag.String("db", "events.db", "Path to DuckDB database file")
	flag.Parse()

	if err := run(*dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(dbPath string) error {
	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		cancel()
	}()

	fmt.Fprintf(os.Stderr, "DuckDB Sink - writing to %s\n", dbPath)
	fmt.Fprintf(os.Stderr, "Reading JSON events from stdin...\n")

	// Open DuckDB
	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open DuckDB: %w", err)
	}
	defer db.Close()

	// Create table
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS token_transfer_events (
			id INTEGER PRIMARY KEY,
			event_type VARCHAR NOT NULL,
			ledger_sequence INTEGER NOT NULL,
			tx_hash VARCHAR NOT NULL,
			contract_address VARCHAR,
			from_address VARCHAR,
			to_address VARCHAR,
			amount VARCHAR NOT NULL,
			asset_code VARCHAR,
			asset_issuer VARCHAR,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Read events from stdin
	scanner := bufio.NewScanner(os.Stdin)
	eventCount := 0

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "Processed %d events\n", eventCount)
			return nil
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse JSON
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse JSON: %v\n", err)
			continue
		}

		// Insert event
		if err := insertEvent(db, event); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to insert event: %v\n", err)
			continue
		}

		eventCount++
		if eventCount%100 == 0 {
			fmt.Fprintf(os.Stderr, "Processed %d events...\n", eventCount)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stdin: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Processed %d events total\n", eventCount)
	return nil
}

func insertEvent(db *sql.DB, event map[string]interface{}) error {
	query := `
		INSERT INTO token_transfer_events (
			event_type, ledger_sequence, tx_hash, contract_address,
			from_address, to_address, amount, asset_code, asset_issuer
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	eventType := getStringField(event, "type")
	ledgerSeq := getIntField(event, "ledger_sequence")
	txHash := getStringField(event, "tx_hash")
	contractAddr := getStringField(event, "contract_address")
	fromAddr := getStringField(event, "from")
	toAddr := getStringField(event, "to")
	amount := getStringField(event, "amount")

	var assetCode, assetIssuer string
	if asset, ok := event["asset"].(map[string]interface{}); ok {
		assetCode = getStringField(asset, "code")
		assetIssuer = getStringField(asset, "issuer")
	}

	_, err := db.Exec(query,
		eventType, ledgerSeq, txHash, contractAddr,
		fromAddr, toAddr, amount, assetCode, assetIssuer)

	return err
}

func getStringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getIntField(m map[string]interface{}, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}
