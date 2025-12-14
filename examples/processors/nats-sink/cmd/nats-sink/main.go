package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

const version = "0.1.0"

var (
	// Connection settings
	natsURL      string
	credsFile    string
	connName     string
	connTimeout  int

	// Publishing settings
	subjectTmpl string
	useJetStream bool
	strict      bool

	// Connection state (lazy initialized)
	nc *nats.Conn
	js nats.JetStreamContext
)

func main() {
	// Setup graceful shutdown to ensure messages are flushed
	setupCleanup()

	config := cli.SinkConfig{
		Name:        "nats-sink",
		Description: "Publish JSON events to NATS message bus",
		Version:     version,
	}

	cli.RunSinkCLI(config, publishToNats, addFlags)

	// Ensure cleanup on normal exit
	cleanup()
}

// setupCleanup registers signal handlers for graceful shutdown
func setupCleanup() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cleanup()
		os.Exit(0)
	}()
}

// cleanup ensures NATS connection is properly closed
func cleanup() {
	if nc != nil {
		// Flush any pending messages before closing
		nc.Flush()
		nc.Close()
	}
}

// addFlags adds custom flags to the command
func addFlags(cmd *cobra.Command) {
	// Connection flags
	cmd.Flags().StringVar(&natsURL, "url", getEnvOrDefault("NATS_URL", "nats://localhost:4222"),
		"NATS server URL (or set NATS_URL)")
	cmd.Flags().StringVar(&credsFile, "creds", getEnvOrDefault("NATS_CREDS", ""),
		"Path to NATS credentials file (optional, or set NATS_CREDS)")
	cmd.Flags().StringVar(&connName, "name", "nats-sink",
		"Connection name for monitoring")
	cmd.Flags().IntVar(&connTimeout, "timeout", 5,
		"Connection timeout in seconds")

	// Publishing flags
	cmd.Flags().StringVar(&subjectTmpl, "subject", "events",
		"Subject template (e.g. 'stellar.{type}' or 'stellar.{transfer.assetCode}')")
	cmd.Flags().BoolVar(&useJetStream, "jetstream", false,
		"Use JetStream for reliable delivery")
	cmd.Flags().BoolVar(&strict, "strict", false,
		"Fail on missing template variables (default: use '_unknown')")
}

// publishToNats processes each event and publishes to NATS
func publishToNats(event map[string]interface{}) error {
	// Lazy connect on first event
	if nc == nil {
		if err := connect(); err != nil {
			return err
		}
	}

	// Resolve subject from template
	subject := resolveSubject(subjectTmpl, event)

	// Marshal event back to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Publish to NATS
	if useJetStream {
		_, err = js.Publish(subject, data)
	} else {
		err = nc.Publish(subject, data)
	}

	if err != nil {
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}

	return nil
}

// connect establishes connection to NATS server
func connect() error {
	opts := []nats.Option{
		nats.Name(connName),
		nats.Timeout(nats.DefaultTimeout),
		// Production resilience: reconnect forever to handle network blips
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * nats.DefaultTimeout),
	}

	// Add credentials if provided
	if credsFile != "" {
		opts = append(opts, nats.UserCredentials(credsFile))
	}

	// Connect to NATS
	var err error
	nc, err = nats.Connect(natsURL, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS at %s: %w", natsURL, err)
	}

	// Setup JetStream if requested
	if useJetStream {
		js, err = nc.JetStream()
		if err != nil {
			nc.Close()
			return fmt.Errorf("failed to create JetStream context: %w", err)
		}
	}

	return nil
}

// resolveSubject resolves template variables in the subject string
// Supports:
//   - {key} for top-level fields
//   - {nested.key} for nested fields with dot notation
func resolveSubject(template string, event map[string]interface{}) string {
	// If no template variables, return as-is
	if !strings.Contains(template, "{") {
		return template
	}

	result := template

	// Find all {var} patterns
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(template, -1)

	for _, match := range matches {
		placeholder := match[0] // e.g., "{type}"
		path := match[1]        // e.g., "type" or "transfer.assetCode"

		// Resolve value from event
		value := resolveValue(event, path)

		// Replace placeholder
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result
}

// resolveValue resolves a value from the event using dot-notation path
func resolveValue(event map[string]interface{}, path string) string {
	// Split path by dots
	parts := strings.Split(path, ".")

	// Navigate through nested maps
	var current interface{} = event
	for _, part := range parts {
		// Check if current is a map
		m, ok := current.(map[string]interface{})
		if !ok {
			return handleMissingValue(path)
		}

		// Get value
		val, exists := m[part]
		if !exists {
			return handleMissingValue(path)
		}

		current = val
	}

	// Convert final value to string and sanitize for NATS subjects
	strVal := fmt.Sprint(current)

	// CRITICAL: Sanitize dots and spaces to prevent breaking NATS subject hierarchy
	// Example: asset "Funny.Token" would create "stellar.Funny.Token" breaking wildcard subscriptions
	strVal = strings.ReplaceAll(strVal, ".", "_")
	strVal = strings.ReplaceAll(strVal, " ", "_")

	return strVal
}

// handleMissingValue handles missing template variables based on strict mode
func handleMissingValue(path string) string {
	if strict {
		fmt.Fprintf(os.Stderr, "Error: template variable '%s' not found in event\n", path)
		os.Exit(1)
	}
	return "_unknown"
}

// getEnvOrDefault gets environment variable or returns default
func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}
