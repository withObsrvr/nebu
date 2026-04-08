package processor

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// Transform processors consume typed events and emit transformed events.
//
// Transform processors sit between origins and sinks in a pipeline. They
// filter, enrich, deduplicate, aggregate, or otherwise reshape an event
// stream without producing external side effects.
//
// Implementations typically hold an Emitter[T] of their output type and
// write transformed events to it from ProcessEvent. Because the input type
// varies by upstream processor, ProcessEvent accepts proto.Message and
// implementations are expected to type-assert to their concrete input type.
//
// # Error handling
//
// ProcessEvent does not return an error. Per-event errors (a malformed
// input, an unexpected type) should be reported via [ReportWarning] and
// the method should return normally; the pipeline will continue with the
// next event. Unrecoverable errors should be reported via [ReportFatal]
// and the method should return immediately.
//
// Example implementation:
//
//	type USDCFilter struct {
//	    name    string
//	    emitter *processor.Emitter[*ttpb.TokenTransferEvent]
//	}
//
//	func (f *USDCFilter) Name() string         { return f.name }
//	func (f *USDCFilter) Type() processor.Type { return processor.TypeTransform }
//
//	func (f *USDCFilter) ProcessEvent(ctx context.Context, event proto.Message) {
//	    ev, ok := event.(*ttpb.TokenTransferEvent)
//	    if !ok {
//	        processor.ReportWarning(ctx, f.name,
//	            fmt.Errorf("unexpected event type %T", event))
//	        return
//	    }
//	    if isUSDC(ev) {
//	        select {
//	        case <-ctx.Done():
//	            return
//	        default:
//	            f.emitter.Emit(ev)
//	        }
//	    }
//	}
//
//	func (f *USDCFilter) Out() <-chan *ttpb.TokenTransferEvent {
//	    return f.emitter.Out()
//	}
type Transform interface {
	Processor

	// ProcessEvent processes a single input event. The implementation
	// must respect context cancellation, type-assert the input event
	// to its expected concrete type, and report errors through the
	// Reporter attached to ctx (see [ReportWarning], [ReportFatal]).
	// Transformed output is written to the implementation's internal
	// Emitter.
	ProcessEvent(ctx context.Context, event proto.Message)
}
