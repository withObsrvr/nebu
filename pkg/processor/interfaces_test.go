package processor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// fakeTransform is a minimal Transform implementation used to assert the
// interface shape. It echoes string values to its internal emitter and
// reports a warning for any other input type.
type fakeTransform struct {
	name    string
	emitter *Emitter[*wrapperspb.StringValue]
}

func (f *fakeTransform) Name() string { return f.name }
func (f *fakeTransform) Type() Type   { return TypeTransform }

func (f *fakeTransform) ProcessEvent(ctx context.Context, event proto.Message) {
	ev, ok := event.(*wrapperspb.StringValue)
	if !ok {
		ReportWarning(ctx, f.name, errors.New("unexpected event type"))
		return
	}
	select {
	case <-ctx.Done():
		return
	default:
		f.emitter.Emit(ev)
	}
}

// fakeSink is a minimal Sink implementation used to assert the interface
// shape. It records everything it receives and reports a warning on
// unexpected input types.
type fakeSink struct {
	name     string
	received []*wrapperspb.StringValue
}

func (s *fakeSink) Name() string { return s.name }
func (s *fakeSink) Type() Type   { return TypeSink }

func (s *fakeSink) WriteEvent(ctx context.Context, event proto.Message) {
	ev, ok := event.(*wrapperspb.StringValue)
	if !ok {
		ReportWarning(ctx, s.name, errors.New("unexpected event type"))
		return
	}
	select {
	case <-ctx.Done():
		return
	default:
		s.received = append(s.received, ev)
	}
}

// Compile-time assertions that the fake implementations satisfy the
// contract interfaces. If the interface shape changes in a breaking way,
// this file will fail to compile.
var (
	_ Processor = (*fakeTransform)(nil)
	_ Transform = (*fakeTransform)(nil)
	_ Processor = (*fakeSink)(nil)
	_ Sink      = (*fakeSink)(nil)
)

func TestTransform_ProcessEvent(t *testing.T) {
	tr := &fakeTransform{
		name:    "test-transform",
		emitter: NewEmitter[*wrapperspb.StringValue](4),
	}

	assert.Equal(t, "test-transform", tr.Name())
	assert.Equal(t, TypeTransform, tr.Type())

	ctx := context.Background()
	tr.ProcessEvent(ctx, wrapperspb.String("hello"))

	tr.emitter.Close()
	var seen []string
	for ev := range tr.emitter.Out() {
		seen = append(seen, ev.Value)
	}
	assert.Equal(t, []string{"hello"}, seen)
}

func TestTransform_ReportsWarningOnBadType(t *testing.T) {
	tr := &fakeTransform{
		name:    "test-transform",
		emitter: NewEmitter[*wrapperspb.StringValue](1),
	}

	var reports []ErrorReport
	ctx := WithReporter(context.Background(), ReporterFunc(func(r ErrorReport) {
		reports = append(reports, r)
	}))

	// Passing a non-StringValue should trigger a warning but not panic.
	tr.ProcessEvent(ctx, wrapperspb.Int32(42))

	assert.Len(t, reports, 1)
	assert.Equal(t, SeverityWarning, reports[0].Severity)
	assert.Equal(t, "test-transform", reports[0].Processor)
}

func TestSink_WriteEvent(t *testing.T) {
	sk := &fakeSink{name: "test-sink"}

	assert.Equal(t, "test-sink", sk.Name())
	assert.Equal(t, TypeSink, sk.Type())

	ctx := context.Background()
	sk.WriteEvent(ctx, wrapperspb.String("a"))
	sk.WriteEvent(ctx, wrapperspb.String("b"))

	assert.Len(t, sk.received, 2)
	assert.Equal(t, "a", sk.received[0].Value)
	assert.Equal(t, "b", sk.received[1].Value)
}

func TestTransform_RespectsContextCancellation(t *testing.T) {
	tr := &fakeTransform{
		name:    "test-transform",
		emitter: NewEmitter[*wrapperspb.StringValue](1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Fill the buffer so an unchecked emit would block, then verify
	// that a cancelled context short-circuits before emission.
	tr.emitter.Emit(wrapperspb.String("preloaded"))

	// ProcessEvent is void — the test asserts behavior, not a return value.
	tr.ProcessEvent(ctx, wrapperspb.String("cancelled"))

	// The emitter still only contains the preloaded value; the second
	// emit was skipped because ctx was cancelled.
	tr.emitter.Close()
	var seen []string
	for ev := range tr.emitter.Out() {
		seen = append(seen, ev.Value)
	}
	assert.Equal(t, []string{"preloaded"}, seen)
}
