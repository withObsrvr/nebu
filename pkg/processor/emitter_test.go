package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestEmitter(t *testing.T) {
	// Use a simple protobuf message for testing
	emitter := NewEmitter[*wrapperspb.StringValue](5)

	// Emit some events
	go func() {
		emitter.Emit(wrapperspb.String("event1"))
		emitter.Emit(wrapperspb.String("event2"))
		emitter.Emit(wrapperspb.String("event3"))
		emitter.Close()
	}()

	// Collect events
	var events []string
	for ev := range emitter.Out() {
		events = append(events, ev.Value)
	}

	assert.Equal(t, []string{"event1", "event2", "event3"}, events)
}

func TestEmitter_Out(t *testing.T) {
	emitter := NewEmitter[*wrapperspb.Int32Value](1)

	// Channel should be readable
	ch := emitter.Out()
	assert.NotNil(t, ch)

	// Emit and verify
	go emitter.Emit(wrapperspb.Int32(42))

	val := <-ch
	assert.Equal(t, int32(42), val.Value)
}

func TestEmitter_Close(t *testing.T) {
	emitter := NewEmitter[*wrapperspb.BoolValue](1)

	emitter.Close()

	// Channel should be closed
	_, ok := <-emitter.Out()
	assert.False(t, ok, "channel should be closed")
}

// Verify emitter works with any proto.Message type
func TestEmitter_GenericProtoMessage(t *testing.T) {
	// Test with different protobuf types
	stringEmitter := NewEmitter[*wrapperspb.StringValue](1)
	assert.NotNil(t, stringEmitter)

	int32Emitter := NewEmitter[*wrapperspb.Int32Value](1)
	assert.NotNil(t, int32Emitter)

	// Both should work since they implement proto.Message
	var _ proto.Message = (*wrapperspb.StringValue)(nil)
	var _ proto.Message = (*wrapperspb.Int32Value)(nil)
}
