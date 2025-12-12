// Package duckdb_sink implements a sink processor that writes events to DuckDB.
package duckdb_sink

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/withObsrvr/nebu/pkg/processor"
	_ "github.com/marcboeker/go-duckdb"
)

// Sink writes token transfer events to a DuckDB database.
type Sink struct {
	db        *sql.DB
	tableName string
}

// NewSink creates a new DuckDB sink.
// dbPath is the path to the DuckDB database file (e.g., "events.db").
// If it doesn't exist, it will be created.
func NewSink(dbPath string) (*Sink, error) {
	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open DuckDB: %w", err)
	}

	sink := &Sink{
		db:        db,
		tableName: "token_transfer_events",
	}

	// Create table if it doesn't exist
	if err := sink.createTable(); err != nil {
		db.Close()
		return nil, err
	}

	return sink, nil
}

// Name implements processor.Processor.
func (s *Sink) Name() string {
	return "duckdb-sink"
}

// Type implements processor.Processor.
func (s *Sink) Type() processor.Type {
	return processor.TypeSink
}

// createTable creates the events table if it doesn't exist.
func (s *Sink) createTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
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
	`, s.tableName)

	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

// ConsumeEvent writes a token transfer event to DuckDB.
// This is a simplified example that expects a map[string]interface{} event.
func (s *Sink) ConsumeEvent(ctx context.Context, event map[string]interface{}) error {
	query := fmt.Sprintf(`
		INSERT INTO %s (
			event_type, ledger_sequence, tx_hash, contract_address,
			from_address, to_address, amount, asset_code, asset_issuer
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.tableName)

	// Extract fields with defaults
	eventType := getString(event, "type")
	ledgerSeq := getUint32(event, "ledger_sequence")
	txHash := getString(event, "tx_hash")
	contractAddr := getString(event, "contract_address")
	fromAddr := getString(event, "from")
	toAddr := getString(event, "to")
	amount := getString(event, "amount")

	// Handle asset
	var assetCode, assetIssuer string
	if asset, ok := event["asset"].(map[string]interface{}); ok {
		assetCode = getString(asset, "code")
		assetIssuer = getString(asset, "issuer")
	}

	_, err := s.db.ExecContext(ctx, query,
		eventType, ledgerSeq, txHash, contractAddr,
		fromAddr, toAddr, amount, assetCode, assetIssuer)

	if err != nil {
		return fmt.Errorf("failed to insert event: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (s *Sink) Close() error {
	return s.db.Close()
}

// Query executes a SQL query and returns the results as a string.
// Useful for debugging or viewing data.
func (s *Sink) Query(query string) (string, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	// Simple string representation
	result := ""
	cols, _ := rows.Columns()
	result += fmt.Sprintf("%v\n", cols)

	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		rows.Scan(valuePtrs...)
		result += fmt.Sprintf("%v\n", values)
	}

	return result, nil
}

// Helper functions

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getUint32(m map[string]interface{}, key string) uint32 {
	switch v := m[key].(type) {
	case uint32:
		return v
	case int:
		return uint32(v)
	case float64:
		return uint32(v)
	}
	return 0
}
