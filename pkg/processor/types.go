// Package processor defines interfaces and types for nebu processors.
//
// Processors are the core building blocks of nebu pipelines. They come in three types:
//   - Origin: Consumes ledgers from Stellar and emits typed events
//   - Transform: Consumes typed events and emits transformed events
//   - Sink: Consumes typed events and produces side effects (DB writes, etc.)
package processor

// Type represents the type of processor.
type Type int

const (
	// TypeOrigin processors consume LedgerCloseMeta and emit events.
	TypeOrigin Type = iota

	// TypeTransform processors consume events and emit transformed events.
	TypeTransform

	// TypeSink processors consume events and produce side effects.
	TypeSink
)

// String returns a human-readable name for the processor type.
func (t Type) String() string {
	switch t {
	case TypeOrigin:
		return "origin"
	case TypeTransform:
		return "transform"
	case TypeSink:
		return "sink"
	default:
		return "unknown"
	}
}

// Processor is the base interface that all nebu processors implement.
type Processor interface {
	// Name returns a unique identifier for this processor instance.
	Name() string

	// Type returns the type of this processor.
	Type() Type
}
