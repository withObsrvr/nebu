package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"

	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
)

func newResumeCmd() *cobra.Command {
	var (
		resumeDSN     string
		resumeTable   string
		ledgerColumn  string
		fallbackStart int64
	)

	cmd := &cobra.Command{
		Use:   "resume -- <pipeline>",
		Short: "Auto-resume a pipeline from the last processed ledger",
		Long: `Resume queries PostgreSQL for the maximum ledger sequence already processed,
then launches the given pipeline with --start-ledger set to max+1.

This preserves Unix-pipe separation: the origin processor doesn't need DB access.

Examples:
  # Resume from where postgres-sink left off (using JSONB data column)
  nebu resume --dsn postgres://localhost/mydb --table events --fallback-start 60000000 \
    -- "token-transfer --end-ledger 0 | postgres-sink --dsn postgres://localhost/mydb"

  # Resume using an explicit ledger_sequence column
  nebu resume --dsn postgres://localhost/mydb --table events --ledger-column ledger_sequence \
    --fallback-start 60000000 \
    -- "token-transfer --end-ledger 0 | postgres-sink --dsn postgres://localhost/mydb"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pipeline := strings.Join(args, " ")

			startLedger, err := queryMaxLedger(resumeDSN, resumeTable, ledgerColumn)
			if err != nil {
				return fmt.Errorf("failed to query max ledger: %w", err)
			}

			if startLedger == 0 {
				startLedger = fallbackStart
				if !quietMode {
					fmt.Fprintf(os.Stderr, "No existing data found, using fallback start: %d\n", startLedger)
				}
			} else {
				startLedger++ // Resume from next ledger
				if !quietMode {
					fmt.Fprintf(os.Stderr, "Resuming from ledger %d\n", startLedger)
				}
			}

			// Inject --start-ledger into the pipeline command
			finalPipeline := injectStartLedger(pipeline, startLedger)

			if !quietMode {
				fmt.Fprintf(os.Stderr, "Executing: %s\n", finalPipeline)
			}

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
		"Explicit column name for ledger sequence (default: extract from JSONB data column)")
	cmd.Flags().Int64Var(&fallbackStart, "fallback-start", 0,
		"Start ledger to use if table is empty")

	cmd.MarkFlagRequired("dsn")
	cmd.MarkFlagRequired("fallback-start")

	return cmd
}

// queryMaxLedger queries the database for the maximum ledger sequence in the given table.
func queryMaxLedger(dsn, table, ledgerColumn string) (int64, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return 0, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	var query string
	if ledgerColumn != "" {
		// Use explicit column
		query = fmt.Sprintf(`SELECT COALESCE(MAX(%s), 0) FROM %s`, ledgerColumn, table)
	} else {
		// Extract from JSONB data column
		query = fmt.Sprintf(`SELECT COALESCE(MAX((data->>'ledgerSequence')::bigint), 0) FROM %s`, table)
	}

	var maxLedger int64
	if err := db.QueryRow(query).Scan(&maxLedger); err != nil {
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
