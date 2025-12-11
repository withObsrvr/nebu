package processor

import "google.golang.org/protobuf/proto"

// Emitter is a helper that manages a typed output channel for processors.
// It provides a simple abstraction over channels for emitting events.
//
// Example usage:
//
//	type MyProcessor struct {
//	    emitter *Emitter[*MyEvent]
//	}
//
//	func NewMyProcessor() *MyProcessor {
//	    return &MyProcessor{
//	        emitter: NewEmitter[*MyEvent](1024),
//	    }
//	}
//
//	func (p *MyProcessor) ProcessLedger(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
//	    event := &MyEvent{...}
//	    p.emitter.Emit(event)
//	    return nil
//	}
//
//	func (p *MyProcessor) Out() <-chan *MyEvent {
//	    return p.emitter.Out()
//	}
type Emitter[T proto.Message] struct {
	ch chan T
}

// NewEmitter creates a new Emitter with the specified buffer size.
// A larger buffer can help smooth over processing rate differences,
// but increases memory usage.
func NewEmitter[T proto.Message](bufSize int) *Emitter[T] {
	return &Emitter[T]{
		ch: make(chan T, bufSize),
	}
}

// Out returns the read-only output channel for consuming emitted events.
func (e *Emitter[T]) Out() <-chan T {
	return e.ch
}

// Emit sends an event to the output channel.
// This will block if the channel buffer is full.
//
// Callers should check context cancellation when emitting in a loop:
//
//	select {
//	case <-ctx.Done():
//	    return ctx.Err()
//	default:
//	    emitter.Emit(event)
//	}
func (e *Emitter[T]) Emit(ev T) {
	e.ch <- ev
}

// Close closes the output channel, signaling that no more events will be emitted.
// Consumers will see the channel close when they drain all buffered events.
func (e *Emitter[T]) Close() {
	close(e.ch)
}
