package processor

// DescribeEnvelope is the top-level JSON object that every processor
// binary must emit when invoked with --describe-json. It is the
// machine-readable protocol by which nebu and external consumers
// introspect a processor without reading its source.
//
// The envelope is part of the stable contract surface (see
// docs/STABILITY.md). Field additions are permitted at any time;
// renames and removals require a major version bump.
//
// Consumers should be prepared for unknown fields: nebu may add
// non-required fields in minor releases, and well-behaved parsers
// must ignore them rather than fail.
type DescribeEnvelope struct {
	// Name is the processor name. Matches the binary name and the
	// registry entry name.
	Name string `json:"name"`

	// Type is one of "origin", "transform", or "sink" — matching
	// [Type.String].
	Type string `json:"type"`

	// Version is the processor's reported version string (as set
	// in OriginConfig/TransformConfig/SinkConfig). Optional.
	Version string `json:"version,omitempty"`

	// Description is a one-line human-readable summary.
	Description string `json:"description,omitempty"`

	// Schema describes the data the processor emits (and, for
	// transforms and sinks, the data it consumes).
	Schema DescribeSchema `json:"schema"`

	// Flags lists the command-line flags the processor accepts.
	// Derived automatically from the cobra flagset.
	Flags []DescribeFlag `json:"flags,omitempty"`

	// Examples lists representative command lines.
	Examples []DescribeExample `json:"examples,omitempty"`
}

// DescribeSchema carries the processor's input and output schemas
// plus a canonical identifier.
//
// The Output and Input fields are raw JSON Schema documents (Draft
// 2020-12). They are map[string]any rather than a typed struct so
// that the walker in pkg/processor/jsonschema can produce them
// without any marshaling round-trip.
type DescribeSchema struct {
	// ID is the canonical schema identifier declared by the
	// processor author (e.g., "nebu.token_transfer.v1"). When the
	// processor output changes in an incompatible way the author
	// bumps the version in the ID. Optional.
	ID string `json:"id,omitempty"`

	// Output is the JSON Schema of the events this processor emits.
	// Always present for origins and transforms; omitted for sinks
	// (which have no output).
	Output map[string]any `json:"output,omitempty"`

	// Input is the JSON Schema of the events this processor
	// consumes. Omitted for origins (which consume raw ledgers, not
	// events) and optional for transforms/sinks that accept any
	// JSON-encodable input.
	Input map[string]any `json:"input,omitempty"`
}

// DescribeFlag describes a single command-line flag.
type DescribeFlag struct {
	// Name is the long flag name without the leading "--"
	// (e.g., "start-ledger").
	Name string `json:"name"`

	// Type is the pflag type name ("string", "uint32", "bool",
	// "stringSlice", etc.). Consumers can map these to their own
	// type systems.
	Type string `json:"type"`

	// Required is true if the flag must be supplied for the
	// processor to run.
	Required bool `json:"required"`

	// Description is the short usage string shown in --help.
	Description string `json:"description,omitempty"`

	// Default is the default value, rendered as a string by pflag.
	// Omitted when empty.
	Default string `json:"default,omitempty"`
}

// DescribeExample is a representative command line with an optional
// explanatory comment.
type DescribeExample struct {
	Comment string `json:"comment,omitempty"`
	Command string `json:"command"`
}
