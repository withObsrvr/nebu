package jsonschema

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

// wellKnownSchemas maps well-known protobuf type names to their
// canonical JSON Schema representations. These are the types that
// protojson encodes in a special form (not as regular objects) and
// that downstream consumers expect in their idiomatic shape — a
// Timestamp is an ISO-8601 string, not an object with seconds/nanos
// fields.
//
// This is a var rather than a const so that tests can temporarily
// disable entries to exercise the walker on types that would
// otherwise be short-circuited here. Do not mutate at runtime outside
// of tests.
var wellKnownSchemas = map[protoreflect.FullName]func() map[string]any{
	// Core well-known types.
	"google.protobuf.Timestamp": func() map[string]any {
		return map[string]any{"type": "string", "format": "date-time"}
	},
	"google.protobuf.Duration": func() map[string]any {
		return map[string]any{"type": "string", "format": "duration"}
	},
	"google.protobuf.Empty": func() map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false}
	},
	"google.protobuf.Any": func() map[string]any {
		// Any holds an arbitrary serialized message; in JSON it is an
		// object with @type plus the wrapped message fields. We do
		// not constrain its shape beyond "object".
		return map[string]any{"type": "object"}
	},
	"google.protobuf.FieldMask": func() map[string]any {
		return map[string]any{"type": "string"}
	},

	// Struct family. These hold arbitrary JSON-shaped values.
	"google.protobuf.Struct": func() map[string]any {
		return map[string]any{"type": "object"}
	},
	"google.protobuf.ListValue": func() map[string]any {
		return map[string]any{"type": "array"}
	},
	"google.protobuf.Value": func() map[string]any {
		// Value is a dynamic JSON value — any JSON type is valid.
		return map[string]any{}
	},
	"google.protobuf.NullValue": func() map[string]any {
		return map[string]any{"type": "null"}
	},

	// Scalar wrappers. 64-bit integer wrappers follow the same
	// string-encoding rule as bare 64-bit scalars (see schema.go).
	"google.protobuf.BoolValue": func() map[string]any {
		return map[string]any{"type": "boolean"}
	},
	"google.protobuf.StringValue": func() map[string]any {
		return map[string]any{"type": "string"}
	},
	"google.protobuf.BytesValue": func() map[string]any {
		return map[string]any{"type": "string", "format": "byte"}
	},
	"google.protobuf.Int32Value": func() map[string]any {
		return map[string]any{"type": "integer", "format": "int32"}
	},
	"google.protobuf.Int64Value": func() map[string]any {
		return map[string]any{"type": "string", "format": "int64"}
	},
	"google.protobuf.UInt32Value": func() map[string]any {
		return map[string]any{"type": "integer", "format": "uint32"}
	},
	"google.protobuf.UInt64Value": func() map[string]any {
		return map[string]any{"type": "string", "format": "uint64"}
	},
	"google.protobuf.FloatValue": func() map[string]any {
		return map[string]any{"type": "number", "format": "float"}
	},
	"google.protobuf.DoubleValue": func() map[string]any {
		return map[string]any{"type": "number", "format": "double"}
	},
}

// wellKnownSchema returns the JSON Schema fragment for a well-known
// protobuf type, or nil if the type is not well-known. Each call
// returns a fresh map so callers can safely mutate the result.
func wellKnownSchema(name protoreflect.FullName) map[string]any {
	if fn, ok := wellKnownSchemas[name]; ok {
		return fn()
	}
	return nil
}
