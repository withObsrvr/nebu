// Package jsonschema converts protobuf message descriptors into JSON
// Schema Draft 2020-12 representations. It is used by
// pkg/processor/cli to emit machine-readable output schemas from the
// --describe-json flag that each processor binary implements.
//
// The walker handles scalar types, nested messages, enums, oneofs,
// repeated fields, maps, and the common well-known types. 64-bit
// integer types are emitted as JSON strings to avoid JavaScript
// Number precision loss, matching protojson's canonical encoding.
//
// # Usage
//
//	schema := jsonschema.FromMessage(&ttpb.TokenTransferEvent{})
//	encoded, _ := json.MarshalIndent(schema, "", "  ")
//	fmt.Println(string(encoded))
//
// # Output shape
//
// For a non-trivial message, the returned schema uses $ref at the
// root pointing into $defs, and every nested message reachable from
// the root appears exactly once in $defs:
//
//	{
//	  "$schema": "https://json-schema.org/draft/2020-12/schema",
//	  "$ref":    "#/$defs/my.package.MyMessage",
//	  "$defs": {
//	    "my.package.MyMessage": { ... },
//	    "my.package.Nested":    { ... }
//	  }
//	}
//
// For a well-known scalar type (e.g., google.protobuf.Timestamp), the
// canonical scalar form is returned directly, without a $defs
// section.
//
// # Oneof semantics
//
// Proto oneofs are emitted as a JSON Schema "oneOf" constraint
// listing each case as a single-field required clause. This enforces
// "exactly one field present" semantics, which is stricter than
// proto3's "at most one" — a strictly conformant proto3 message with
// the oneof unset will fail schema validation. This is an acceptable
// compromise for v1; most processors always set their oneofs.
package jsonschema

import (
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// draftURI is the JSON Schema draft that the walker emits.
const draftURI = "https://json-schema.org/draft/2020-12/schema"

// FromMessage returns a JSON Schema for the given proto.Message. It
// is a convenience wrapper around FromDescriptor that extracts the
// descriptor from the message. A nil message returns an empty schema
// (which validates any value).
func FromMessage(m proto.Message) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return FromDescriptor(m.ProtoReflect().Descriptor())
}

// FromDescriptor returns a JSON Schema Draft 2020-12 representation
// of the given protobuf message descriptor. The returned map is
// suitable for encoding with encoding/json.
//
// If the top-level descriptor is a well-known type (e.g.,
// google.protobuf.Timestamp), its canonical form is returned
// directly. Otherwise the returned schema uses $ref + $defs as
// described in the package documentation.
//
// A nil descriptor returns an empty schema.
func FromDescriptor(md protoreflect.MessageDescriptor) map[string]any {
	if md == nil {
		return map[string]any{}
	}

	if wks := wellKnownSchema(md.FullName()); wks != nil {
		wks["$schema"] = draftURI
		return wks
	}

	w := newWalker()
	w.registerMessage(md)

	return map[string]any{
		"$schema": draftURI,
		"$ref":    "#/$defs/" + string(md.FullName()),
		"$defs":   w.defs,
	}
}

// walker collects message definitions as it walks a descriptor graph.
// The defs map holds fully-qualified message names keyed to their
// built JSON Schema representations.
type walker struct {
	defs map[string]map[string]any
}

func newWalker() *walker {
	return &walker{
		defs: make(map[string]map[string]any),
	}
}

// registerMessage ensures the given message type has an entry in defs
// and returns a $ref fragment pointing to it. If the message is a
// well-known type, its inline schema is returned and nothing is added
// to defs. Cycles are broken by inserting an empty sentinel into defs
// before walking, so that a recursive reference during the walk
// returns the $ref rather than recursing infinitely.
func (w *walker) registerMessage(md protoreflect.MessageDescriptor) map[string]any {
	if wks := wellKnownSchema(md.FullName()); wks != nil {
		return wks
	}

	key := string(md.FullName())
	if _, exists := w.defs[key]; !exists {
		// Insert sentinel BEFORE walking to break cycles. We mutate
		// this same map after building so that any $refs collected
		// mid-walk point at stable keys.
		w.defs[key] = map[string]any{}
		built := w.buildMessage(md)
		for k, v := range built {
			w.defs[key][k] = v
		}
	}

	return map[string]any{"$ref": "#/$defs/" + key}
}

// buildMessage constructs the JSON Schema fragment for a message
// type. It walks all fields, resolves nested messages through
// registerMessage (which uses $ref), and encodes oneofs via oneOf
// constraints.
func (w *walker) buildMessage(md protoreflect.MessageDescriptor) map[string]any {
	schema := map[string]any{
		"type":  "object",
		"title": string(md.Name()),
	}
	if desc := descriptorDescription(md); desc != "" {
		schema["description"] = desc
	}

	fields := md.Fields()
	if fields.Len() == 0 {
		return schema
	}

	properties := make(map[string]any, fields.Len())
	// oneofGroups is keyed by oneof name, ordered by first appearance
	// so that output is deterministic when multiple oneofs are present.
	oneofGroupOrder := make([]string, 0)
	oneofGroups := make(map[string][]string)

	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		name := fd.JSONName()
		properties[name] = w.buildField(fd)

		if oo := fd.ContainingOneof(); oo != nil && !oo.IsSynthetic() {
			ooName := string(oo.Name())
			if _, seen := oneofGroups[ooName]; !seen {
				oneofGroupOrder = append(oneofGroupOrder, ooName)
			}
			oneofGroups[ooName] = append(oneofGroups[ooName], name)
		}
	}

	schema["properties"] = properties

	// Encode oneof constraints. One group → direct "oneOf"; multiple
	// groups → "allOf" of oneOf constraints so each applies
	// independently.
	switch len(oneofGroups) {
	case 0:
		// no oneofs
	case 1:
		fields := oneofGroups[oneofGroupOrder[0]]
		schema["oneOf"] = buildOneofCases(fields)
	default:
		allOf := make([]map[string]any, 0, len(oneofGroupOrder))
		for _, ooName := range oneofGroupOrder {
			allOf = append(allOf, map[string]any{
				"oneOf": buildOneofCases(oneofGroups[ooName]),
			})
		}
		schema["allOf"] = allOf
	}

	return schema
}

// buildField returns the JSON Schema fragment for a single field,
// including repeated/map wrapping and field-level descriptions.
func (w *walker) buildField(fd protoreflect.FieldDescriptor) map[string]any {
	var schema map[string]any

	switch {
	case fd.IsMap():
		// Proto maps become JSON objects with additionalProperties
		// constraining the value type. Key types are ignored because
		// JSON object keys are always strings; protojson encodes
		// non-string keys as their string representation.
		schema = map[string]any{
			"type":                 "object",
			"additionalProperties": w.buildFieldType(fd.MapValue()),
		}
	case fd.IsList():
		schema = map[string]any{
			"type":  "array",
			"items": w.buildFieldType(fd),
		}
	default:
		schema = w.buildFieldType(fd)
	}

	if desc := descriptorDescription(fd); desc != "" {
		schema["description"] = desc
	}
	return schema
}

// buildFieldType returns the JSON Schema fragment for the scalar or
// message type of a field, ignoring repeated/map wrapping.
func (w *walker) buildFieldType(fd protoreflect.FieldDescriptor) map[string]any {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return map[string]any{"type": "boolean"}

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return map[string]any{"type": "integer", "format": "int32"}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return map[string]any{"type": "integer", "format": "uint32"}

	// 64-bit integers are emitted as strings to preserve precision
	// through JavaScript consumers. This matches protojson.
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return map[string]any{"type": "string", "format": "int64"}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return map[string]any{"type": "string", "format": "uint64"}

	case protoreflect.FloatKind:
		return map[string]any{"type": "number", "format": "float"}
	case protoreflect.DoubleKind:
		return map[string]any{"type": "number", "format": "double"}

	case protoreflect.StringKind:
		return map[string]any{"type": "string"}
	case protoreflect.BytesKind:
		// Proto bytes are base64-encoded in JSON.
		return map[string]any{"type": "string", "format": "byte"}

	case protoreflect.EnumKind:
		return buildEnum(fd.Enum())

	case protoreflect.MessageKind, protoreflect.GroupKind:
		return w.registerMessage(fd.Message())
	}

	return map[string]any{}
}

// buildEnum returns the JSON Schema fragment for an enum type. Enum
// values are emitted as their string names, sorted for deterministic
// output, matching protojson's encoding.
func buildEnum(ed protoreflect.EnumDescriptor) map[string]any {
	values := ed.Values()
	names := make([]string, values.Len())
	for i := 0; i < values.Len(); i++ {
		names[i] = string(values.Get(i).Name())
	}
	sort.Strings(names)
	schema := map[string]any{
		"type":  "string",
		"title": string(ed.Name()),
		"enum":  names,
	}
	if desc := descriptorDescription(ed); desc != "" {
		schema["description"] = desc
	}
	return schema
}

// buildOneofCases returns the "oneOf" case list for a proto oneof
// group. Each case requires exactly one of the oneof's field JSON
// names to be present.
func buildOneofCases(fieldJSONNames []string) []map[string]any {
	cases := make([]map[string]any, len(fieldJSONNames))
	for i, name := range fieldJSONNames {
		cases[i] = map[string]any{"required": []string{name}}
	}
	return cases
}

// descriptorDescription returns the leading-comment description for
// a descriptor, if source info was preserved when the proto was
// compiled. Returns empty string if no description is available.
//
// The Go protobuf runtime only preserves source locations when protos
// are compiled with --include_source_info. Many generated Go files do
// not include this information, so descriptions are best-effort.
func descriptorDescription(d any) string {
	var loc protoreflect.SourceLocation
	switch v := d.(type) {
	case protoreflect.MessageDescriptor:
		loc = v.ParentFile().SourceLocations().ByDescriptor(v)
	case protoreflect.FieldDescriptor:
		loc = v.ParentFile().SourceLocations().ByDescriptor(v)
	case protoreflect.EnumDescriptor:
		loc = v.ParentFile().SourceLocations().ByDescriptor(v)
	default:
		return ""
	}
	return strings.TrimSpace(loc.LeadingComments)
}
