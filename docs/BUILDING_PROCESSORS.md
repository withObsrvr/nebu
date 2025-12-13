# Building Custom Processors

This guide shows you how to build custom processors for nebu using the CLI wrapper packages.

## Table of Contents
- [Transform Processors](#transform-processors)
- [Sink Processors](#sink-processors)
- [Origin Processors](#origin-processors)
- [Testing Your Processor](#testing-your-processor)
- [Distribution](#distribution)

## Transform Processors

Transform processors read JSON events from stdin, transform/filter them, and write to stdout.

### When to Use

- Filter events based on criteria (asset type, amount, time)
- Modify event structure (add fields, rename, flatten)
- Enrich events with additional data
- Deduplicate or aggregate events

### Quick Start (5 Minutes)

**1. Create directory structure:**
```bash
mkdir -p examples/processors/my-filter/cmd/my-filter
cd examples/processors/my-filter
```

**2. Create `cmd/my-filter/main.go`:**
```go
package main

import (
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

func main() {
	config := cli.TransformConfig{
		Name:        "my-filter",
		Description: "Filter events based on custom criteria",
		Version:     version,
	}

	cli.RunTransformCLI(config, filterFunc, nil)
}

// filterFunc processes each event.
// Return the event to pass it through, or nil to filter it out.
func filterFunc(event map[string]interface{}) map[string]interface{} {
	// Your filtering logic here

	// Example: Only pass transfer events
	eventType, ok := event["type"].(string)
	if !ok || eventType != "transfer" {
		return nil  // Filter out
	}

	return event  // Pass through
}
```

**3. Build and test:**
```bash
go build -o ../../../bin/my-filter ./cmd/my-filter

# Test it
echo '{"type":"transfer","amount":"100"}
{"type":"fee","amount":"50"}
{"type":"transfer","amount":"200"}' | ./bin/my-filter

# Should output only the transfer events
```

### Adding Custom Flags

If your transform needs configuration flags:

```go
package main

import (
	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var (
	minAmount int64
	assetCode string
)

func main() {
	config := cli.TransformConfig{
		Name:        "my-filter",
		Description: "Filter events with custom criteria",
		Version:     "0.1.0",
	}

	cli.RunTransformCLI(config, filterFunc, addFlags)
}

func addFlags(cmd *cobra.Command) {
	cmd.Flags().Int64Var(&minAmount, "min", 0, "Minimum amount")
	cmd.Flags().StringVar(&assetCode, "asset", "", "Asset code to filter")
}

func filterFunc(event map[string]interface{}) map[string]interface{} {
	// Use minAmount and assetCode here
	// ...
	return event
}
```

### Transform Patterns

**Filtering Pattern:**
```go
func filterFunc(event map[string]interface{}) map[string]interface{} {
	// Check condition
	if !meetsCondition(event) {
		return nil  // Filter out
	}
	return event  // Keep
}
```

**Modification Pattern:**
```go
func transformFunc(event map[string]interface{}) map[string]interface{} {
	// Add new field
	event["processed_at"] = time.Now().Unix()

	// Modify existing field
	if amount, ok := event["amount"].(string); ok {
		event["amount_numeric"], _ = strconv.ParseInt(amount, 10, 64)
	}

	return event
}
```

**Enrichment Pattern:**
```go
var cache = make(map[string]interface{})

func enrichFunc(event map[string]interface{}) map[string]interface{} {
	// Look up additional data
	key := event["account"].(string)
	if metadata, exists := cache[key]; exists {
		event["metadata"] = metadata
	}

	return event
}
```

### Real-World Examples

See these processors for complete examples:
- [`usdc-filter`](../examples/processors/usdc-filter/cmd/usdc-filter/main.go) - Filter by asset code
- [`amount-filter`](../examples/processors/amount-filter/cmd/amount-filter/main.go) - Filter by amount range with custom flags
- [`time-window`](../examples/processors/time-window/cmd/time-window/main.go) - Filter by time range
- [`dedup`](../examples/processors/dedup/cmd/dedup/main.go) - Stateful deduplication

---

## Sink Processors

Sink processors read JSON events from stdin and produce side effects (write to databases, files, APIs, etc).

### When to Use

- Write events to databases (PostgreSQL, MongoDB, etc)
- Send events to message queues (Kafka, RabbitMQ)
- POST events to webhooks
- Write to files with special formatting
- Trigger alerts or notifications

### Quick Start (5 Minutes)

**1. Create directory structure:**
```bash
mkdir -p examples/processors/my-sink/cmd/my-sink
cd examples/processors/my-sink
```

**2. Create `cmd/my-sink/main.go`:**
```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"
var outputPath string

func main() {
	config := cli.SinkConfig{
		Name:        "my-sink",
		Description: "Write events to custom destination",
		Version:     version,
	}

	cli.RunSinkCLI(config, sinkFunc, addFlags)
}

func addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&outputPath, "out", "output.txt", "Output path")
}

// sinkFunc processes each event and produces a side effect.
// Return an error to stop processing.
func sinkFunc(event map[string]interface{}) error {
	// Your sink logic here

	// Example: Print event type
	eventType := event["type"]
	fmt.Printf("Received %v event\n", eventType)

	return nil
}
```

**3. Build and test:**
```bash
go build -o ../../../bin/my-sink ./cmd

# Test it
echo '{"type":"transfer","amount":"100"}
{"type":"fee","amount":"50"}' | ./bin/my-sink
```

### Sink Patterns

**Database Writing Pattern:**
```go
var db *sql.DB

func sinkFunc(event map[string]interface{}) error {
	// Open DB on first event
	if db == nil {
		var err error
		db, err = sql.Open("postgres", connectionString)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}

	// Insert event
	_, err := db.Exec(
		"INSERT INTO events (type, amount) VALUES ($1, $2)",
		event["type"], event["amount"],
	)
	return err
}
```

**Batching Pattern:**
```go
var (
	batch      []map[string]interface{}
	batchSize  = 100
	eventCount = 0
)

func sinkFunc(event map[string]interface{}) error {
	batch = append(batch, event)
	eventCount++

	// Flush batch when full
	if len(batch) >= batchSize {
		if err := flushBatch(); err != nil {
			return err
		}
		batch = nil
	}

	return nil
}

func flushBatch() error {
	// Write batch to destination
	// ...
	return nil
}
```

**Webhook Pattern:**
```go
import "net/http"

var httpClient = &http.Client{}

func sinkFunc(event map[string]interface{}) error {
	jsonData, err := json.Marshal(event)
	if err != nil {
		return err
	}

	resp, err := httpClient.Post(
		webhookURL,
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}

	return nil
}
```

### Real-World Examples

See this processor for a complete example:
- [`json-file-sink`](../examples/processors/json-file-sink/cmd/json-file-sink/main.go) - Write to JSONL files

---

## Origin Processors

Origin processors fetch raw data (XDR ledgers) and output JSON events to stdout.

**IMPORTANT:** Always use the **proto-first** approach when building origin processors. This ensures:
- Type safety via protobuf schemas
- Consistency across processors
- flowctl-ready from day one (just add gRPC server)
- Automatic JSON conversion via protojson

### When to Use

- Extract different event types from Stellar ledgers (NFT transfers, trades, liquidity pools)
- Parse custom smart contract events
- Process other blockchain data formats

### Quick Start (Proto-First) - 20 Minutes

**1. Create directory structure:**
```bash
mkdir -p examples/processors/my-origin/cmd/my-origin
cd examples/processors/my-origin
```

**2. Define protobuf schema (`my_origin.proto`):**
```protobuf
syntax = "proto3";

package my_origin;

option go_package = "github.com/withObsrvr/nebu/examples/processors/my-origin";

message MyEvent {
  int64 timestamp = 1;
  uint32 ledger_sequence = 2;
  string transaction_hash = 3;
  string event_type = 4;
  // Add your custom fields here
  string custom_field = 5;
}
```

**3. Generate Go code:**
```bash
# Install protoc-gen-go if not already installed
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Generate
export PATH="$HOME/go/bin:$PATH"
nix-shell -p protobuf --run "protoc --go_out=. --go_opt=paths=source_relative my_origin.proto"
```

**4. Create processor (`my_origin.go`):**
```go
package my_origin

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
)

type Origin struct {
	networkPass string
	out         chan *MyEvent
}

func NewOrigin(networkPass string) *Origin {
	return &Origin{
		networkPass: networkPass,
		out:         make(chan *MyEvent, 128),
	}
}

func (o *Origin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
	// Extract events from ledger
	events := o.extractEvents(ledger)

	// Emit each event
	for _, event := range events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case o.out <- event:
			// Event sent
		}
	}

	return nil
}

func (o *Origin) extractEvents(ledger xdr.LedgerCloseMeta) []*MyEvent {
	var events []*MyEvent

	// Your extraction logic here
	// Parse ledger and build MyEvent protobuf messages

	return events
}

// Required processor.Origin interface methods
func (o *Origin) Out() <-chan *MyEvent { return o.out }
func (o *Origin) Close()                { close(o.out) }
func (o *Origin) Name() string          { return "my-origin" }
func (o *Origin) Type() processor.Type  { return processor.TypeOrigin }
```

**5. Create CLI wrapper (`cmd/my-origin/main.go`):**
```go
package main

import (
	"github.com/withObsrvr/nebu/examples/processors/my-origin"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

func main() {
	config := cli.OriginConfig{
		Name:        "my-origin",
		Description: "Extract custom events from Stellar ledgers",
		Version:     version,
	}

	// Use RunProtoOriginCLI for proto-first processors
	cli.RunProtoOriginCLI(config, func(networkPass string) cli.ProtoOriginProcessor[*my_origin.MyEvent] {
		return my_origin.NewOrigin(networkPass)
	})
}
```

**6. Create go.mod:**
```bash
go mod init github.com/withObsrvr/nebu/examples/processors/my-origin
go mod tidy
```

**7. Build and test:**
```bash
go build -o ../../../bin/my-origin ./cmd/my-origin

# Test with real data
./bin/my-origin --start-ledger 60200000 --end-ledger 60200001 | head
```

### Real-World Example

See this processor for a complete example:
- [`token-transfer`](../examples/processors/token-transfer/) - Extract token transfer events from Stellar ledgers

---

## Testing Your Processor

### Manual Testing

**Test transform processor:**
```bash
# Create test input
echo '{"type":"transfer","amount":"100"}
{"type":"fee","amount":"50"}
{"type":"transfer","amount":"200"}' > test-input.jsonl

# Run processor
cat test-input.jsonl | ./bin/my-filter

# Verify output
cat test-input.jsonl | ./bin/my-filter | jq .
```

**Test in pipeline:**
```bash
# Origin → Transform → Sink
./bin/token-transfer --start-ledger 60200000 --end-ledger 60200001 | \
  ./bin/my-filter | \
  ./bin/json-file-sink --out test-output.jsonl

# Verify
cat test-output.jsonl | jq . | head
```

### Automated Testing

Create a test script (`test_my_processor.sh`):

```bash
#!/usr/bin/env bash
set -e

echo "Testing my-filter..."

# Test 1: Valid JSON output
OUTPUT=$(echo '{"type":"transfer"}' | ./bin/my-filter)
if echo "$OUTPUT" | jq -e . > /dev/null 2>&1; then
    echo "✓ Produces valid JSON"
else
    echo "✗ Invalid JSON output"
    exit 1
fi

# Test 2: Filtering works
INPUT='{"type":"transfer"}
{"type":"fee"}
{"type":"transfer"}'

COUNT=$(echo "$INPUT" | ./bin/my-filter | wc -l)
if [[ $COUNT -eq 2 ]]; then
    echo "✓ Filtering works"
else
    echo "✗ Expected 2 events, got $COUNT"
    exit 1
fi

echo "All tests passed!"
```

Make it executable and run:
```bash
chmod +x test_my_processor.sh
./test_my_processor.sh
```

---

## Distribution

### Building Binaries

**Single platform:**
```bash
go build -o ./bin/my-processor ./examples/processors/my-processor/cmd
```

**Cross-platform builds:**
```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o ./bin/my-processor-linux \
  ./examples/processors/my-processor/cmd

# macOS
GOOS=darwin GOARCH=amd64 go build -o ./bin/my-processor-macos \
  ./examples/processors/my-processor/cmd

# Windows
GOOS=windows GOARCH=amd64 go build -o ./bin/my-processor.exe \
  ./examples/processors/my-processor/cmd
```

### Installation

**Local installation:**
```bash
# Build and copy to PATH
go build -o $HOME/.local/bin/my-processor \
  ./examples/processors/my-processor/cmd
```

**Using go install:**
```bash
# If processor has its own module
cd examples/processors/my-processor
go install ./cmd
```

### Packaging

**Create release tarball:**
```bash
# Build
go build -o ./bin/my-processor ./examples/processors/my-processor/cmd

# Package
tar -czf my-processor-v1.0.0-linux-amd64.tar.gz \
  -C ./bin my-processor \
  -C .. README.md LICENSE
```

**Docker image:**
```dockerfile
FROM golang:1.25 AS builder
WORKDIR /app
COPY . .
RUN go build -o /bin/my-processor ./examples/processors/my-processor/cmd

FROM alpine:latest
COPY --from=builder /bin/my-processor /usr/local/bin/
ENTRYPOINT ["my-processor"]
```

---

## Tips and Best Practices

### Performance

1. **Avoid repeated allocations** - Reuse buffers and maps
2. **Batch operations** - For sinks, batch writes to databases/APIs
3. **Use goroutines carefully** - CLI wrappers handle concurrency
4. **Profile your processor** - Use `go tool pprof` for bottlenecks

### Error Handling

1. **Fail fast** - Return errors immediately for critical failures
2. **Log warnings** - Use stderr for non-fatal issues (in non-quiet mode)
3. **Provide context** - Wrap errors with `fmt.Errorf("context: %w", err)`
4. **Test error paths** - Ensure your processor handles bad input gracefully

### Documentation

1. **Add usage examples** - Show common use cases in comments
2. **Document flags** - Explain what each flag does
3. **Version your processor** - Update version string on changes
4. **Include examples** - Provide sample input/output in README

### Composability

1. **Read from stdin** - Accept piped input
2. **Write to stdout** - Output events as JSONL
3. **Support quiet mode** - Use `-q` to suppress progress messages
4. **Chain with others** - Test your processor in pipelines

---

## Getting Help

- Check existing processors in `examples/processors/` for patterns
- Read the [main README](../README.md) for architecture overview
- See [PIPELINE.md](../PIPELINE.md) for pipeline examples
- Run integration tests: `./tests/integration/test_pipelines.sh`

---

## Summary

**Transform Processor in 3 Steps:**
1. Create `cmd/processor-name/main.go` with `cli.RunTransformCLI()`
2. Implement `filterFunc(event) event|nil`
3. Build and test in pipelines

**Sink Processor in 3 Steps:**
1. Create `cmd/processor-name/main.go` with `cli.RunSinkCLI()`
2. Implement `sinkFunc(event) error`
3. Build and test with real data

**Origin Processor in 4 Steps:**
1. Implement `ProcessLedger(ledger) error`
2. Create `cmd/processor-name/main.go` with `cli.RunOriginCLI()`
3. Build and test with Stellar data
4. Chain with transforms and sinks

Happy building! 🚀
