package processor

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// Sink processors consume typed events and produce external side effects.
//
// Sinks are the terminal nodes of a pipeline: they write to databases,
// files, message buses, or other external systems. Unlike Transform
// processors, Sinks do not emit events to downstream processors.
//
// Sinks that hold resources (database connections, file handles, network
// clients) should additionally implement io.Closer so the runtime can
// release them during graceful shutdown.
//
// # Error handling
//
// WriteEvent does not return an error. Per-event write failures (a bad
// row, a serialization error) should be reported via [ReportWarning]
// and the method should return normally; the pipeline will continue
// with the next event. Failures that leave the sink unable to write
// anything further (a dropped database connection that cannot be
// re-established) should be reported via [ReportFatal] and the method
// should return immediately.
//
// Example implementation:
//
//	type JSONFileSink struct {
//	    name string
//	    enc  *json.Encoder
//	}
//
//	func (s *JSONFileSink) Name() string         { return s.name }
//	func (s *JSONFileSink) Type() processor.Type { return processor.TypeSink }
//
//	func (s *JSONFileSink) WriteEvent(ctx context.Context, event proto.Message) {
//	    select {
//	    case <-ctx.Done():
//	        return
//	    default:
//	    }
//	    if err := s.enc.Encode(event); err != nil {
//	        processor.ReportWarning(ctx, s.name, err)
//	    }
//	}
type Sink interface {
	Processor

	// WriteEvent writes a single event to the sink's external destination.
	// The implementation must respect context cancellation, type-assert
	// the input event to its expected concrete type, and report errors
	// through the Reporter attached to ctx (see [ReportWarning],
	// [ReportFatal]).
	WriteEvent(ctx context.Context, event proto.Message)
}
