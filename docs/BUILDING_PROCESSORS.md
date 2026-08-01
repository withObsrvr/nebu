# Building Custom Processors

This guide shows how to build nebu processors against the current contract surface.

If you're building something intended to live outside this repo, start with:

- [`docs/STABILITY.md`](./STABILITY.md) — what is permanently stable
- [`docs/REGISTRY_SPEC.md`](./REGISTRY_SPEC.md) — how processors are described and discovered
- the external proto-first walkthrough in the community registry: [`BUILDING_PROTO_PROCESSORS.md`](https://github.com/withObsrvr/nebu-processor-registry/blob/main/BUILDING_PROTO_PROCESSORS.md)

## The contract, in one minute

Processors come in three types:

- **Origin** — consumes ledgers, emits events
- **Transform** — consumes events, emits transformed events
- **Sink** — consumes events, produces side effects

The stable interfaces live in [`pkg/processor`](../pkg/processor) and [`pkg/source`](../pkg/source).

### Error handling: streams never throw

The current processor contract does **not** return `error` from processing methods:

- `Origin.ProcessLedger(ctx, ledger)`
- `Transform.ProcessEvent(ctx, event)`
- `Sink.WriteEvent(ctx, event)`

Instead:

- use `processor.ReportWarning(ctx, name, err)` for per-event/per-ledger failures that should **not** halt the pipeline
- use `processor.ReportFatal(ctx, name, err)` for unrecoverable failures that **should** halt the pipeline

That is the stable, current API. If you see old examples returning `error`, treat them as obsolete.

## When to build each processor type

### Build a Transform when you want to
- filter events by asset, amount, or time
- enrich or reshape JSON
- deduplicate or normalize a stream

### Build a Sink when you want to
- write to a database
- publish to a queue or webhook
- write files in a custom format
- trigger side effects

### Build an Origin when you want to
- extract new event types from Stellar ledgers
- wrap Stellar SDK processors
- parse contract-specific or protocol-specific ledger data

---

## Transform processors

Transform processors read newline-delimited JSON from stdin, transform or filter each event, and write newline-delimited JSON to stdout.

### Quick start

**1. Create the directory structure**

```bash
mkdir -p my-filter/cmd/my-filter
cd my-filter
```

**2. Create `cmd/my-filter/main.go`**

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

// Return the event to pass it through, or nil to drop it.
func filterFunc(event map[string]interface{}) map[string]interface{} {
    transfer, ok := event["transfer"].(map[string]interface{})
    if !ok {
        return nil
    }
    if transfer["assetCode"] != "USDC" {
        return nil
    }
    return event
}
```

**3. Build and test**

```bash
mkdir -p bin
go build -o ./bin/my-filter ./cmd/my-filter

echo '{"transfer":{"assetCode":"USDC","amount":"100"}}'
```

```bash
echo '{"transfer":{"assetCode":"USDC","amount":"100"}}' | ./bin/my-filter
```

### Adding custom flags

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
    transfer, ok := event["transfer"].(map[string]interface{})
    if !ok {
        return nil
    }
    if assetCode != "" && transfer["assetCode"] != assetCode {
        return nil
    }
    return event
}
```

### Real examples

- [`usdc-filter`](https://github.com/withObsrvr/nebu-processor-registry/blob/main/processors/usdc-filter/cmd/usdc-filter/main.go)
- [`amount-filter`](https://github.com/withObsrvr/nebu-processor-registry/blob/main/processors/amount-filter/cmd/amount-filter/main.go)
- [`time-window`](https://github.com/withObsrvr/nebu-processor-registry/blob/main/processors/time-window/cmd/time-window/main.go)
- [`dedup`](https://github.com/withObsrvr/nebu-processor-registry/blob/main/processors/dedup/cmd/dedup/main.go)

---

## Sink processors

Sink processors read newline-delimited JSON from stdin and produce side effects.

### Quick start

**1. Create the directory structure**

```bash
mkdir -p my-sink/cmd/my-sink
cd my-sink
```

**2. Create `cmd/my-sink/main.go`**

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
        Description: "Write events to a custom destination",
        Version:     version,
    }

    cli.RunSinkCLI(config, sinkFunc, addFlags)
}

func addFlags(cmd *cobra.Command) {
    cmd.Flags().StringVar(&outputPath, "out", "output.txt", "Output path")
}

// sinkFunc should return nil on success and a non-nil error when the
// CLI wrapper should stop the process. Prefer internal retries where
// appropriate; for typed in-runtime sinks, use ReportWarning / ReportFatal.
func sinkFunc(event map[string]interface{}) error {
    fmt.Printf("received event with schema %v\n", event["_schema"])
    return nil
}
```

### Sink patterns

**Database writing**

```go
var db *sql.DB

func sinkFunc(event map[string]interface{}) error {
    if db == nil {
        var err error
        db, err = sql.Open("postgres", connectionString)
        if err != nil {
            return fmt.Errorf("connect postgres: %w", err)
        }
    }

    _, err := db.Exec(
        "INSERT INTO events (schema_id, payload) VALUES ($1, $2)",
        event["_schema"], event,
    )
    return err
}
```

**Batching**

```go
var batch []map[string]interface{}
const batchSize = 100

func sinkFunc(event map[string]interface{}) error {
    batch = append(batch, event)
    if len(batch) < batchSize {
        return nil
    }
    if err := flushBatch(batch); err != nil {
        return err
    }
    batch = nil
    return nil
}
```

### Real examples

- [`json-file-sink`](https://github.com/withObsrvr/nebu-processor-registry/blob/main/processors/json-file-sink/cmd/json-file-sink/main.go)
- [`nats-sink`](https://github.com/withObsrvr/nebu-processor-registry/blob/main/processors/nats-sink/cmd/nats-sink/main.go)
- [`postgres-sink`](https://github.com/withObsrvr/nebu-processor-registry/blob/main/processors/postgres-sink/cmd/postgres-sink/main.go)

---

## Origin processors

Origins are best built **proto-first**.

That means:

- define a protobuf message for your event
- emit that protobuf type internally
- let the CLI helper expose JSON on stdout
- implement `--describe-json` through the helper so tools can introspect your processor

### Quick start

**1. Create the directory structure**

```bash
mkdir -p my-origin/cmd/my-origin
cd my-origin
```

**2. Define protobuf schema in `proto/my_origin.proto`**

```protobuf
syntax = "proto3";

package my_origin;

option go_package = "github.com/you/my-origin/proto";

message MyEvent {
  uint32 ledger_sequence = 1;
  string transaction_hash = 2;
  string event_type = 3;
  string custom_field = 4;
}
```

**3. Generate Go code**

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
export PATH="$HOME/go/bin:$PATH"
protoc --go_out=. --go_opt=paths=source_relative proto/my_origin.proto
```

**4. Create the processor**

```go
package my_origin

import (
    "context"
    "fmt"

    "github.com/stellar/go-stellar-sdk/xdr"
    "github.com/withObsrvr/nebu/pkg/processor"

    mypb "github.com/you/my-origin/proto"
)

type Origin struct {
    emitter *processor.Emitter[*mypb.MyEvent]
}

func NewOrigin() *Origin {
    return &Origin{
        emitter: processor.NewEmitter[*mypb.MyEvent](128),
    }
}

func (o *Origin) Name() string         { return "my-origin" }
func (o *Origin) Type() processor.Type { return processor.TypeOrigin }
func (o *Origin) Out() <-chan *mypb.MyEvent {
    return o.emitter.Out()
}
func (o *Origin) Close() { o.emitter.Close() }

func (o *Origin) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) {
    events, err := extractEvents(ledger)
    if err != nil {
        processor.ReportWarning(ctx, o.Name(), fmt.Errorf("ledger %d: %w", ledger.LedgerSequence(), err))
        return
    }

    for _, ev := range events {
        select {
        case <-ctx.Done():
            return
        default:
            o.emitter.Emit(ev)
        }
    }
}

func extractEvents(ledger xdr.LedgerCloseMeta) ([]*mypb.MyEvent, error) {
    // Parse the ledger and build protobuf events.
    return nil, nil
}
```

**5. Create the CLI wrapper**

```go
package main

import (
    myorigin "github.com/you/my-origin"
    mypb "github.com/you/my-origin/proto"
    "github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

func main() {
    config := cli.OriginConfig{
        Name:        "my-origin",
        Description: "Extract custom events from Stellar ledgers",
        Version:     version,
        SchemaID:    "nebu.my_origin.v1",
    }

    cli.RunProtoOriginCLI(config, func(networkPass string) cli.ProtoOriginProcessor[*mypb.MyEvent] {
        _ = networkPass
        return myorigin.NewOrigin()
    })
}
```

**6. Initialize the module**

```bash
go mod init github.com/you/my-origin
go mod tidy
```

**7. Build and test**

```bash
mkdir -p bin
go build -o ./bin/my-origin ./cmd/my-origin
./bin/my-origin --describe-json | jq .
```

### Real examples

- [`token-transfer`](https://github.com/withObsrvr/nebu-processor-registry/tree/main/processors/token-transfer)
- [`contract-events`](https://github.com/withObsrvr/nebu-processor-registry/tree/main/processors/contract-events)
- [`contract-invocation`](https://github.com/withObsrvr/nebu-processor-registry/tree/main/processors/contract-invocation)

---

## Testing your processor

### Manual testing

**Transform**

```bash
echo '{"transfer":{"assetCode":"USDC","amount":"100"}}' | ./bin/my-filter | jq .
```

**Full pipeline**

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200001 | \
  ./bin/my-filter | \
  ./bin/json-file-sink --out test-output.jsonl
```

### Runtime contract tests

For typed processors embedded in Go, test the actual interfaces from `pkg/processor`:

- origins should implement `ProcessLedger(ctx, ledger)` without returning `error`
- transforms should implement `ProcessEvent(ctx, event)`
- sinks should implement `WriteEvent(ctx, event)`
- warnings/fatals should be asserted via a test reporter attached with `processor.WithReporter`

---

## Distribution

### Build locally

```bash
go build -o ./bin/my-processor ./cmd/my-processor
```

### Install via `go install`

```bash
cd my-processor
go install ./cmd/my-processor
```

### Publish for `nebu install`

For a processor that lives outside this repo:

1. put it in its own Go module
2. make sure `go install <module>/cmd/<name>@latest` works
3. implement `--describe-json` (the Go CLI helpers do this automatically)
4. add a `description.yml` entry in an external registry per [`docs/REGISTRY_SPEC.md`](./REGISTRY_SPEC.md)

---

## Best practices

### Contract usage

- depend only on `pkg/processor` and `pkg/source` if you want stability
- treat `pkg/runtime`, `pkg/registry`, and concrete source implementations as unstable unless you intentionally pin versions

### Errors

- use `ReportWarning` for malformed input, partial decode failures, and per-event issues
- use `ReportFatal` only when the processor cannot continue safely
- do not panic for normal bad data

### CLI behavior

- read from stdin when you are a transform or sink
- write JSONL to stdout when you are an origin or transform CLI
- keep logs and progress on stderr
- support `--describe-json`

### Documentation

- document your `_schema` value
- include copy-pasteable examples
- show one bounded-range example first
- prefer installed-binary examples over repo-local build examples in user docs

## See also

- [`README.md`](../README.md)
- [`docs/STABILITY.md`](./STABILITY.md)
- [`docs/PIPELINE.md`](./PIPELINE.md)
- [`docs/REGISTRY_SPEC.md`](./REGISTRY_SPEC.md)
- [`token-transfer/SCHEMA.md`](https://github.com/withObsrvr/nebu-processor-registry/blob/main/processors/token-transfer/SCHEMA.md)
