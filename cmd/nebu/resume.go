package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/spf13/cobra"
)

// identifierRegexp validates SQL identifiers to prevent injection.
var identifierRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func newResumeCmd() *cobra.Command {
	var (
		resumeDSN     string
		resumeTable   string
		ledgerColumn  string
		fallbackStart int64
		queryTimeout  time.Duration
	)

	cmd := &cobra.Command{
		Use:   "resume -- <pipeline>",
		Short: "Auto-resume a pipeline from the last processed ledger",
		Long: `Resume queries PostgreSQL for the maximum ledger sequence already processed,
then launches the given pipeline with --start-ledger set to max+1.

This preserves Unix-pipe separation: the origin processor doesn't need DB access.

Examples:
  # Resume using the ledger_sequence column (default, works with postgres-sink v0.2.0+)
  nebu resume --dsn postgres://localhost/mydb --table events --fallback-start 60000000 \
    -- "token-transfer --end-ledger 0 | postgres-sink --dsn postgres://localhost/mydb"

  # Resume using JSONB data column (for tables without ledger_sequence column)
  nebu resume --dsn postgres://localhost/mydb --table events --ledger-column jsonb \
    --fallback-start 60000000 \
    -- "token-transfer --end-ledger 0 | postgres-sink --dsn postgres://localhost/mydb"

  # Resume using a custom column name
  nebu resume --dsn postgres://localhost/mydb --table events --ledger-column sequence_num \
    --fallback-start 60000000 \
    -- "token-transfer --end-ledger 0 | postgres-sink --dsn postgres://localhost/mydb"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pipeline := strings.Join(args, " ")

			startLedger, err := queryMaxLedger(resumeDSN, resumeTable, ledgerColumn, queryTimeout)
			if err != nil {
				return fmt.Errorf("failed to query max ledger: %w", err)
			}

			if startLedger == 0 {
				startLedger = fallbackStart
				logInfo("No existing data found, using fallback start: %d", startLedger)
			} else {
				startLedger++ // Resume from next ledger
				logInfo("Resuming from ledger %d", startLedger)
			}

			// Inject --start-ledger into the pipeline command
			finalPipeline := injectStartLedger(pipeline, startLedger)

			logInfo("Executing: %s", finalPipeline)

			// Execute via sh -c to preserve pipe operators
			shellCmd := exec.Command("sh", "-c", finalPipeline)
			shellCmd.Stdin = os.Stdin
			shellCmd.Stdout = os.Stdout
			shellCmd.Stderr = os.Stderr

			return shellCmd.Run()
		},
	}

	cmd.Flags().StringVar(&resumeDSN, "dsn", getEnvOrDefault("POSTGRES_DSN", ""),
		"PostgreSQL connection string (or set POSTGRES_DSN env)")
	cmd.Flags().StringVar(&resumeTable, "table", "events",
		"Table to query for max ledger sequence")
	cmd.Flags().StringVar(&ledgerColumn, "ledger-column", "",
		"Column for ledger sequence: omit for 'ledger_sequence', 'jsonb' for JSONB extraction, or a custom column name")
	cmd.Flags().Int64Var(&fallbackStart, "fallback-start", 0,
		"Start ledger to use if table is empty")
	cmd.Flags().DurationVar(&queryTimeout, "query-timeout", 30*time.Second,
		"Timeout for the max ledger query")

	// MarkFlagRequired returns an error if the flag name doesn't exist.
	// Since we just registered these flags above, this should never fail,
	// but we handle it to fail fast on wiring mistakes.
	if err := cmd.MarkFlagRequired("dsn"); err != nil {
		panic(fmt.Sprintf("failed to mark flag required: %v", err))
	}
	if err := cmd.MarkFlagRequired("fallback-start"); err != nil {
		panic(fmt.Sprintf("failed to mark flag required: %v", err))
	}

	return cmd
}

// queryMaxLedger queries the database for the maximum ledger sequence in the given table.
func queryMaxLedger(dsn, table, ledgerColumn string, timeout time.Duration) (int64, error) {
	// Validate table name
	if !identifierRegexp.MatchString(table) {
		return 0, fmt.Errorf("invalid table name %q: must match [a-zA-Z_][a-zA-Z0-9_]*", table)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return 0, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Verify connectivity before querying
	if err := db.PingContext(ctx); err != nil {
		return 0, fmt.Errorf("failed to connect to database: %w", err)
	}

	quotedTable := pq.QuoteIdentifier(table)

	var query string
	switch {
	case ledgerColumn == "jsonb":
		// Extract from JSONB data column, trying nested meta.ledgerSequence first
		query = fmt.Sprintf(`SELECT COALESCE(MAX(
			GREATEST(
				(data->'meta'->>'ledgerSequence')::bigint,
				(data->>'ledgerSequence')::bigint
			)
		), 0) FROM %s`, quotedTable)
	case ledgerColumn != "":
		// Use a custom explicit column
		if !identifierRegexp.MatchString(ledgerColumn) {
			return 0, fmt.Errorf("invalid column name %q: must match [a-zA-Z_][a-zA-Z0-9_]*", ledgerColumn)
		}
		quotedCol := pq.QuoteIdentifier(ledgerColumn)
		query = fmt.Sprintf(`SELECT COALESCE(MAX(%s), 0) FROM %s`, quotedCol, quotedTable)
	default:
		// Default: use the explicit ledger_sequence column (added by postgres-sink v0.2.0+)
		query = fmt.Sprintf(`SELECT COALESCE(MAX(ledger_sequence), 0) FROM %s`, quotedTable)
	}

	var maxLedger int64
	if err := db.QueryRowContext(ctx, query).Scan(&maxLedger); err != nil {
		return 0, fmt.Errorf("failed to query max ledger: %w", err)
	}

	return maxLedger, nil
}

// injectStartLedger replaces or appends --start-ledger in the pipeline command.
func injectStartLedger(pipeline string, ledger int64) string {
	flag := fmt.Sprintf("--start-ledger %d", ledger)

	// If --start-ledger already exists, replace it
	if idx := strings.Index(pipeline, "--start-ledger"); idx != -1 {
		// Find end of the existing value
		rest := pipeline[idx+len("--start-ledger"):]
		// Skip whitespace
		i := 0
		for i < len(rest) && rest[i] == ' ' {
			i++
		}
		// Skip the value
		for i < len(rest) && rest[i] != ' ' && rest[i] != '|' {
			i++
		}
		return pipeline[:idx] + flag + rest[i:]
	}

	// If --start-ledger doesn't exist but --start does, replace it
	if idx := strings.Index(pipeline, "--start "); idx != -1 {
		rest := pipeline[idx+len("--start "):]
		i := 0
		for i < len(rest) && rest[i] != ' ' && rest[i] != '|' {
			i++
		}
		return pipeline[:idx] + flag + rest[i:]
	}

	// Inject at the beginning of the first command (before first pipe)
	if pipeIdx := strings.Index(pipeline, "|"); pipeIdx != -1 {
		firstCmd := strings.TrimSpace(pipeline[:pipeIdx])
		rest := pipeline[pipeIdx:]
		return firstCmd + " " + flag + " " + rest
	}

	// No pipe, just append
	return pipeline + " " + flag
}

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}
