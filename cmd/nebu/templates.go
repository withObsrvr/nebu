package main

const originProcessorTemplate = `// Package {{.PackageName}} implements a custom origin processor.
package {{.PackageName}}

import (
	"context"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
)

// {{.ProcessorName}} is an origin processor that processes Stellar ledgers.
type {{.ProcessorName}} struct {
	// TODO: Add your fields here
	// emitter *processor.Emitter[*YourEventType]
}

// New{{.ProcessorName}} creates a new {{.PackageName}} processor.
func New{{.ProcessorName}}() *{{.ProcessorName}} {
	return &{{.ProcessorName}}{
		// TODO: Initialize your fields
		// emitter: processor.NewEmitter[*YourEventType](1024),
	}
}

// Name implements processor.Processor.
func (p *{{.ProcessorName}}) Name() string {
	return "{{.Name}}"
}

// Type implements processor.Processor.
func (p *{{.ProcessorName}}) Type() processor.Type {
	return processor.TypeOrigin
}

// ProcessLedger implements processor.Origin.
// This is called for each ledger in the range.
func (p *{{.ProcessorName}}) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
	// TODO: Implement your ledger processing logic here
	//
	// Example:
	//   1. Extract data from the ledger
	//   2. Convert to your event type
	//   3. Emit events via p.emitter.Emit(event)
	//
	// Tips:
	//   - Check ctx.Done() in loops for cancellation support
	//   - Return errors to stop processing
	//   - Use ledger.LedgerSequence() for the ledger number

	seq := ledger.LedgerSequence()
	_ = seq // TODO: Remove this line when you use seq

	return nil
}
`

const transformProcessorTemplate = `// Package {{.PackageName}} implements a transform processor.
package {{.PackageName}}

import (
	"context"

	"github.com/withObsrvr/nebu/pkg/processor"
)

// {{.ProcessorName}} is a transform processor.
type {{.ProcessorName}} struct {
	// TODO: Add your fields here
	// inputCh  chan *InputEventType
	// emitter  *processor.Emitter[*OutputEventType]
}

// New{{.ProcessorName}} creates a new {{.PackageName}} processor.
func New{{.ProcessorName}}() *{{.ProcessorName}} {
	return &{{.ProcessorName}}{
		// TODO: Initialize your fields
	}
}

// Name implements processor.Processor.
func (p *{{.ProcessorName}}) Name() string {
	return "{{.Name}}"
}

// Type implements processor.Processor.
func (p *{{.ProcessorName}}) Type() processor.Type {
	return processor.TypeTransform
}

// Process transforms input events to output events.
// TODO: Implement your transformation logic here.
func (p *{{.ProcessorName}}) Process(ctx context.Context /* , input *InputEventType */) error {
	// TODO: Implement your transformation logic
	//
	// Example:
	//   1. Receive input event
	//   2. Transform/filter/enrich
	//   3. Emit output event via p.emitter.Emit(output)

	return nil
}
`

const sinkProcessorTemplate = `// Package {{.PackageName}} implements a sink processor.
package {{.PackageName}}

import (
	"context"

	"github.com/withObsrvr/nebu/pkg/processor"
)

// {{.ProcessorName}} is a sink processor that consumes events.
type {{.ProcessorName}} struct {
	// TODO: Add your fields here (e.g., database connection, HTTP client, etc.)
}

// New{{.ProcessorName}} creates a new {{.PackageName}} processor.
func New{{.ProcessorName}}() *{{.ProcessorName}} {
	return &{{.ProcessorName}}{
		// TODO: Initialize your fields
	}
}

// Name implements processor.Processor.
func (p *{{.ProcessorName}}) Name() string {
	return "{{.Name}}"
}

// Type implements processor.Processor.
func (p *{{.ProcessorName}}) Type() processor.Type {
	return processor.TypeSink
}

// Consume processes an event and produces a side effect.
// TODO: Implement your sink logic here.
func (p *{{.ProcessorName}}) Consume(ctx context.Context /* , event *EventType */) error {
	// TODO: Implement your sink logic
	//
	// Examples:
	//   - Write to database
	//   - Send to message queue (Kafka, RabbitMQ, etc.)
	//   - POST to webhook
	//   - Write to file
	//   - Update cache
	//
	// Tips:
	//   - Handle errors gracefully
	//   - Consider batching for performance
	//   - Implement retry logic if needed

	return nil
}

// Close releases any resources held by the sink.
func (p *{{.ProcessorName}}) Close() error {
	// TODO: Close database connections, file handles, etc.
	return nil
}
`

const protoTemplate = `syntax = "proto3";

package {{.PackageName}};

option go_package = "github.com/withObsrvr/nebu/processors/examples/{{.PackageName}};{{.PackageName}}pb";

// TODO: Define your event messages here
//
// Example:
//
// message MyEvent {
//   uint32 ledger_sequence = 1;
//   string tx_hash = 2;
//   string data = 3;
// }

// TODO: Define your service (optional, for gRPC/HTTP endpoints)
//
// service {{.Name}}Service {
//   rpc GetEvents(GetEventsRequest) returns (stream MyEvent);
// }
//
// message GetEventsRequest {
//   uint32 start_ledger = 1;
//   uint32 end_ledger = 2;
// }
`

const manifestTemplate = `name: {{.Name}}
version: 0.1.0
type: {{.Type}}

display_name: {{.Name}} Processor
description: |
  TODO: Describe what this processor does

proto:
  input: TODO
  output: TODO

maintainer:
  name: Your Name
  url: https://example.com

license: MIT
`

const readmeTemplate = `# {{.Name}} Processor

TODO: Describe your processor

## Type

{{.Type}}

## What It Does

TODO: Explain what this processor does

## Usage

` + "```" + `go
// TODO: Add usage example
` + "```" + `

## Configuration

TODO: Document any configuration options

## Development

` + "```" + `bash
# Run tests
go test ./...

# Build
go build ./...
` + "```" + `
`
