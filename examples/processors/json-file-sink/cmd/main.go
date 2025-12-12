// Package main implements a simple JSON file sink that reads events from stdin.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	outFile := flag.String("out", "events.jsonl", "Output file path (JSONL format)")
	flag.Parse()

	if err := run(*outFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(outFile string) error {
	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		os.Exit(0)
	}()

	fmt.Fprintf(os.Stderr, "JSON File Sink - writing to %s\n", outFile)
	fmt.Fprintf(os.Stderr, "Reading events from stdin...\n")

	// Open output file
	f, err := os.Create(outFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	// Read events from stdin
	scanner := bufio.NewScanner(os.Stdin)
	eventCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Validate JSON
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid JSON: %v\n", err)
			continue
		}

		// Write to file
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("failed to write event: %w", err)
		}

		eventCount++
		if eventCount%100 == 0 {
			fmt.Fprintf(os.Stderr, "Processed %d events...\n", eventCount)
			writer.Flush() // Flush periodically
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stdin: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Processed %d events total\n", eventCount)
	return nil
}
