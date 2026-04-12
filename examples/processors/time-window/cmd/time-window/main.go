// Package main provides a standalone CLI for the time-window transform processor.
//
// This processor filters events based on time ranges using actual event timestamps.
// It prefers meta.closedAtUnix emitted by origin processors.
//
// Usage:
//
//	# Events from last hour
//	cat events.jsonl | time-window --last 1h
//
//	# Events from last 24 hours
//	token-transfer --start-ledger 60200000 --end-ledger 60300000 | \
//	  time-window --last 24h
//
//	# Events from last 7 days
//	token-transfer --start-ledger 60200000 --end-ledger 60300000 | \
//	  time-window --last 168h | \
//	  json-file-sink --out weekly.jsonl
package main

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

var (
	lastDuration string
	startTime    int64
	endTime      int64
)

func main() {
	config := cli.TransformConfig{
		Name:        "time-window",
		Description: "Filter events by time range using ledger timestamps",
		Version:     version,
	}

	cli.RunTransformCLI(config, filterByTimeWindow, addFlags)
}

func addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&lastDuration, "last", "", "Filter for events from last duration (e.g., 1h, 24h, 7d)")
	cmd.Flags().Int64Var(&startTime, "start", 0, "Start timestamp (Unix seconds, 0 = no limit)")
	cmd.Flags().Int64Var(&endTime, "end", 0, "End timestamp (Unix seconds, 0 = no limit)")
}

// filterByTimeWindow filters events based on time ranges.
// Requires meta.closedAtUnix so filtering uses real ledger close time.
func filterByTimeWindow(event map[string]interface{}) map[string]interface{} {
	meta, ok := event["meta"].(map[string]interface{})
	if !ok {
		return nil
	}

	closedAtUnix, ok := meta["closedAtUnix"].(float64)
	if !ok {
		return nil
	}

	eventTime := int64(closedAtUnix)

	// Check --last duration
	if lastDuration != "" {
		duration, err := time.ParseDuration(lastDuration)
		if err != nil {
			return nil // Invalid duration, filter out
		}

		cutoffTime := time.Now().Unix() - int64(duration.Seconds())
		if eventTime < cutoffTime {
			return nil // Event too old
		}
	}

	// Check --start timestamp
	if startTime > 0 && eventTime < startTime {
		return nil
	}

	// Check --end timestamp
	if endTime > 0 && eventTime > endTime {
		return nil
	}

	// Passed time window filters
	return event
}
