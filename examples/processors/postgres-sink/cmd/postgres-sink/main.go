// Package main provides a standalone CLI for the postgres-sink processor.
//
// postgres-sink stores nebu events in PostgreSQL using JSONB for flexible schema.
// It provides automatic TOID generation for idempotency and batched COPY for performance.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
	"github.com/withObsrvr/nebu/pkg/toid"
)

const version = "0.1.0"

var (
	// Connection settings
	dsn string

	// Table settings
	tableName string
	batchSize int

	// Conflict resolution
	conflictMode string // "ignore" or "update"

	// State
	db          *sql.DB
	batch       []batchEvent
	ctx         context.Context
	cancel      context.CancelFunc
	flushTicker *time.Ticker
)

type batchEvent struct {
	id        int64
	eventType *string
	data      []byte
}

func main() {
	// Setup context and graceful shutdown
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	setupCleanup()

	config := cli.SinkConfig{
		Name:        "postgres-sink",
		Description: "Store events in PostgreSQL with JSONB schema",
		Version:     version,
	}

	cli.RunSinkCLI(config, processEvent, addFlags)

	// Cleanup on normal exit
	cleanup()
}

// setupCleanup registers signal handlers for graceful shutdown
func setupCleanup() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Fprintln(os.Stderr, "\nReceived shutdown signal, flushing...")
		cleanup()
		os.Exit(0)
	}()
}

// cleanup ensures database connection is properly closed and batch is flushed
func cleanup() {
	// Stop the ticker first
	if flushTicker != nil {
		flushTicker.Stop()
	}

	// Flush any pending events BEFORE canceling context
	if db != nil && len(batch) > 0 {
		if err := flushBatch(); err != nil {
			fmt.Fprintf(os.Stderr, "Error flushing final batch: %v\n", err)
		}
	}

	// Now cancel context to stop ticker goroutine
	cancel()

	// Close database connection
	if db != nil {
		db.Close()
	}
}

// addFlags adds custom flags to the command
func addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&dsn, "dsn", getEnvOrDefault("POSTGRES_DSN", ""),
		"PostgreSQL connection string (or set POSTGRES_DSN env)")
	cmd.Flags().StringVar(&tableName, "table", "events",
		"Table name for storing events")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000,
		"Number of events to batch before COPY")
	cmd.Flags().StringVar(&conflictMode, "conflict", "ignore",
		"Conflict resolution: 'ignore' (DO NOTHING) or 'update' (DO UPDATE)")

	cmd.MarkFlagRequired("dsn")
}

// processEvent handles each incoming event
func processEvent(event map[string]interface{}) error {
	// Lazy connect on first event
	if db == nil {
		if err := connect(); err != nil {
			return err
		}
		if err := ensureTable(); err != nil {
			return err
		}
		startFlushTicker()
	}

	// Generate or extract TOID
	var id int64
	var err error

	// Check if event already has a pre-calculated TOID
	if toidVal, ok := event["toid"]; ok {
		switch v := toidVal.(type) {
		case float64:
			id = int64(v)
		case int64:
			id = v
		case int:
			id = int64(v)
		default:
			return fmt.Errorf("invalid toid type: %T", toidVal)
		}
	} else if idVal, ok := event["id"]; ok {
		// Also support "id" field
		switch v := idVal.(type) {
		case float64:
			id = int64(v)
		case int64:
			id = v
		case int:
			id = int64(v)
		default:
			return fmt.Errorf("invalid id type: %T", idVal)
		}
	} else {
		// Auto-generate TOID from meta fields
		id, err = toid.FromEvent(event)
		if err != nil {
			return fmt.Errorf("failed to generate TOID: %w", err)
		}
	}

	// Extract event type if present
	eventType := extractEventType(event)

	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Add to batch
	batch = append(batch, batchEvent{
		id:        id,
		eventType: eventType,
		data:      data,
	})

	// Flush if batch is full
	if len(batch) >= batchSize {
		return flushBatch()
	}

	return nil
}

// extractEventType extracts the event type from an event.
// Supports multiple formats:
//   - Custom jq: "event_type" or "function_name" field
//   - contract-events: "eventType" field (e.g., "transfer", "fee")
//   - contract-invocation: "functionName" field (e.g., "work", "transfer")
//   - protobuf oneof: field name indicates type (e.g., has "transfer" field)
//   - simple: "type" field (e.g., {"type": "transfer"})
func extractEventType(event map[string]interface{}) *string {
	// Try custom jq "event_type" field first (snake_case convention)
	if t, ok := event["event_type"].(string); ok && t != "" && t != "unknown" {
		result := t
		return &result
	}

	// Try contract-events "eventType" field (camelCase)
	if t, ok := event["eventType"].(string); ok && t != "" && t != "unknown" {
		result := t
		return &result
	}

	// Try contract-invocation "functionName" field
	if t, ok := event["functionName"].(string); ok && t != "" {
		result := t
		return &result
	}

	// Try custom jq "function_name" field (snake_case)
	if t, ok := event["function_name"].(string); ok && t != "" {
		result := t
		return &result
	}

	// Try protobuf oneof fields (token-transfer, etc.)
	// Check for common event type fields
	oneofFields := []string{"transfer", "mint", "burn", "clawback", "fee", "payment", "invoke"}
	for _, field := range oneofFields {
		if _, exists := event[field]; exists {
			result := field
			return &result
		}
	}

	// Fall back to simple "type" field (but skip enum values like "CONTRACT")
	if t, ok := event["type"].(string); ok && t != "CONTRACT" && t != "SYSTEM" && t != "DIAGNOSTIC" {
		result := t
		return &result
	}

	return nil
}

// connect establishes connection to PostgreSQL
func connect() error {
	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings for production
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	return nil
}

// ensureTable creates the events table if it doesn't exist
func ensureTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id BIGINT PRIMARY KEY,
			event_type TEXT,
			data JSONB NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`, tableName)

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Create indexes
	indexes := []string{
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_data ON %s USING GIN (data)", tableName, tableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_event_type ON %s (event_type) WHERE event_type IS NOT NULL", tableName, tableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_created_at ON %s (created_at)", tableName, tableName),
	}

	for _, idx := range indexes {
		if _, err := db.ExecContext(ctx, idx); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// flushBatch writes the current batch to PostgreSQL using COPY
func flushBatch() error {
	if len(batch) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare COPY statement
	stmt, err := tx.PrepareContext(ctx, getUpsertQuery())
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Insert each event in the batch
	for _, evt := range batch {
		_, err := stmt.ExecContext(ctx, evt.id, evt.eventType, evt.data)
		if err != nil {
			return fmt.Errorf("failed to insert event: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Clear batch
	batch = batch[:0]

	return nil
}

// getUpsertQuery returns the appropriate INSERT query based on conflict mode
func getUpsertQuery() string {
	base := fmt.Sprintf(`
		INSERT INTO %s (id, event_type, data)
		VALUES ($1, $2, $3)
	`, tableName)

	switch conflictMode {
	case "update":
		return base + `
			ON CONFLICT (id) DO UPDATE SET
				event_type = EXCLUDED.event_type,
				data = EXCLUDED.data,
				created_at = NOW()
		`
	case "ignore":
		fallthrough
	default:
		return base + ` ON CONFLICT (id) DO NOTHING`
	}
}

// startFlushTicker starts a ticker to flush batches periodically
func startFlushTicker() {
	flushTicker = time.NewTicker(1 * time.Second)
	go func() {
		for {
			select {
			case <-flushTicker.C:
				if len(batch) > 0 {
					if err := flushBatch(); err != nil {
						fmt.Fprintf(os.Stderr, "Error flushing batch: %v\n", err)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// getEnvOrDefault gets environment variable or returns default
func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}
