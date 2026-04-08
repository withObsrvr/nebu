package jsonschema

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ---- Well-known types ----

func TestFromMessage_WellKnown_Timestamp(t *testing.T) {
	schema := FromMessage(timestamppb.New(time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, draftURI, schema["$schema"])
	assert.Equal(t, "string", schema["type"])
	assert.Equal(t, "date-time", schema["format"])
	// No $defs / $ref at the top level for well-known scalars.
	_, hasDefs := schema["$defs"]
	assert.False(t, hasDefs)
	_, hasRef := schema["$ref"]
	assert.False(t, hasRef)
}

func TestFromMessage_WellKnown_Duration(t *testing.T) {
	schema := FromMessage(durationpb.New(0))
	assert.Equal(t, "string", schema["type"])
	assert.Equal(t, "duration", schema["format"])
}

func TestFromMessage_WellKnown_Empty(t *testing.T) {
	schema := FromMessage(&emptypb.Empty{})
	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, false, schema["additionalProperties"])
}

func TestFromMessage_WellKnown_Any(t *testing.T) {
	schema := FromMessage(&anypb.Any{})
	assert.Equal(t, "object", schema["type"])
}

func TestFromMessage_WellKnown_Wrapper_String(t *testing.T) {
	schema := FromMessage(wrapperspb.String(""))
	assert.Equal(t, "string", schema["type"])
}

func TestFromMessage_WellKnown_Wrapper_Int32(t *testing.T) {
	schema := FromMessage(wrapperspb.Int32(0))
	assert.Equal(t, "integer", schema["type"])
	assert.Equal(t, "int32", schema["format"])
}

func TestFromMessage_WellKnown_Wrapper_Int64_AsString(t *testing.T) {
	// 64-bit integers must be emitted as strings to preserve precision
	// through JavaScript consumers. Critical for Stellar amounts.
	schema := FromMessage(wrapperspb.Int64(0))
	assert.Equal(t, "string", schema["type"], "int64 must be emitted as string")
	assert.Equal(t, "int64", schema["format"])
}

func TestFromMessage_WellKnown_Wrapper_UInt64_AsString(t *testing.T) {
	schema := FromMessage(wrapperspb.UInt64(0))
	assert.Equal(t, "string", schema["type"])
	assert.Equal(t, "uint64", schema["format"])
}

func TestFromMessage_WellKnown_Wrapper_Bytes(t *testing.T) {
	schema := FromMessage(wrapperspb.Bytes(nil))
	assert.Equal(t, "string", schema["type"])
	assert.Equal(t, "byte", schema["format"])
}

func TestFromMessage_WellKnown_Wrapper_Bool(t *testing.T) {
	schema := FromMessage(wrapperspb.Bool(false))
	assert.Equal(t, "boolean", schema["type"])
}

// ---- Nil inputs ----

func TestFromMessage_Nil(t *testing.T) {
	schema := FromMessage(nil)
	assert.Equal(t, map[string]any{}, schema)
}

func TestFromDescriptor_NilDescriptor(t *testing.T) {
	schema := FromDescriptor(nil)
	assert.Equal(t, map[string]any{}, schema)
}

// ---- Non-well-known walker ----

// walkerTest bypasses well-known special-casing by temporarily
// removing an entry from the wellKnownSchemas map. Restores on test
// cleanup.
func withoutWellKnown(t *testing.T, name protoreflect.FullName) {
	t.Helper()
	saved, ok := wellKnownSchemas[name]
	if !ok {
		t.Fatalf("expected %q to be a well-known type", name)
	}
	delete(wellKnownSchemas, name)
	t.Cleanup(func() { wellKnownSchemas[name] = saved })
}

func TestFromMessage_TopLevelStructure(t *testing.T) {
	// Use a non-well-known proto: descriptorpb.FileDescriptorProto.
	schema := FromMessage(&descriptorpb.FileDescriptorProto{})

	assert.Equal(t, draftURI, schema["$schema"])
	assert.Equal(t, "#/$defs/google.protobuf.FileDescriptorProto", schema["$ref"])

	defs, ok := schema["$defs"].(map[string]map[string]any)
	require.True(t, ok, "$defs must be a map[string]map[string]any, got %T", schema["$defs"])
	require.Contains(t, defs, "google.protobuf.FileDescriptorProto")

	top := defs["google.protobuf.FileDescriptorProto"]
	assert.Equal(t, "object", top["type"])
	assert.Equal(t, "FileDescriptorProto", top["title"])
}

func TestFromMessage_ScalarFields(t *testing.T) {
	// FileDescriptorProto has a variety of scalar fields:
	//   string name, string package, repeated string dependency,
	//   repeated int32 public_dependency, etc.
	schema := FromMessage(&descriptorpb.FileDescriptorProto{})

	defs := schema["$defs"].(map[string]map[string]any)
	top := defs["google.protobuf.FileDescriptorProto"]
	props := top["properties"].(map[string]any)

	// string field
	name := props["name"].(map[string]any)
	assert.Equal(t, "string", name["type"])

	// repeated string field
	dep := props["dependency"].(map[string]any)
	assert.Equal(t, "array", dep["type"])
	items := dep["items"].(map[string]any)
	assert.Equal(t, "string", items["type"])

	// repeated int32 field
	pubDep := props["publicDependency"].(map[string]any)
	assert.Equal(t, "array", pubDep["type"])
	pubDepItems := pubDep["items"].(map[string]any)
	assert.Equal(t, "integer", pubDepItems["type"])
	assert.Equal(t, "int32", pubDepItems["format"])
}

func TestFromMessage_NestedMessageViaRef(t *testing.T) {
	schema := FromMessage(&descriptorpb.FileDescriptorProto{})
	defs := schema["$defs"].(map[string]map[string]any)

	top := defs["google.protobuf.FileDescriptorProto"]
	props := top["properties"].(map[string]any)

	// messageType is repeated DescriptorProto — array of refs.
	msgType := props["messageType"].(map[string]any)
	assert.Equal(t, "array", msgType["type"])
	items := msgType["items"].(map[string]any)
	ref, ok := items["$ref"].(string)
	require.True(t, ok, "items should contain a $ref")
	assert.Equal(t, "#/$defs/google.protobuf.DescriptorProto", ref)

	// DescriptorProto must be in defs.
	assert.Contains(t, defs, "google.protobuf.DescriptorProto")
}

func TestFromMessage_Enum(t *testing.T) {
	// FieldDescriptorProto has an enum field: Type type.
	schema := FromMessage(&descriptorpb.FieldDescriptorProto{})
	defs := schema["$defs"].(map[string]map[string]any)

	top := defs["google.protobuf.FieldDescriptorProto"]
	props := top["properties"].(map[string]any)

	typeField := props["type"].(map[string]any)
	assert.Equal(t, "string", typeField["type"])
	enum, ok := typeField["enum"].([]string)
	require.True(t, ok, "enum must be a []string, got %T", typeField["enum"])
	assert.NotEmpty(t, enum)
	// Known values we expect to see.
	assert.Contains(t, enum, "TYPE_STRING")
	assert.Contains(t, enum, "TYPE_INT64")
}

func TestFromMessage_SelfReferential(t *testing.T) {
	// DescriptorProto contains repeated DescriptorProto nested_type —
	// the ur-self-referential type. The walker must not recurse.
	schema := FromMessage(&descriptorpb.DescriptorProto{})
	defs := schema["$defs"].(map[string]map[string]any)

	assert.Contains(t, defs, "google.protobuf.DescriptorProto")
	top := defs["google.protobuf.DescriptorProto"]
	props := top["properties"].(map[string]any)

	nested := props["nestedType"].(map[string]any)
	assert.Equal(t, "array", nested["type"])
	items := nested["items"].(map[string]any)
	assert.Equal(t, "#/$defs/google.protobuf.DescriptorProto", items["$ref"])
}

func TestFromMessage_Oneof(t *testing.T) {
	// structpb.Value has a "kind" oneof: null_value, number_value,
	// string_value, bool_value, struct_value, list_value. We need to
	// temporarily disable its well-known handling to exercise the
	// walker's oneof code path.
	withoutWellKnown(t, "google.protobuf.Value")
	// struct_value and list_value reference Struct/ListValue which
	// are also well-known; leave those alone so we don't recurse.

	schema := FromMessage(&structpb.Value{})
	defs := schema["$defs"].(map[string]map[string]any)
	top := defs["google.protobuf.Value"]

	// Expect a oneOf constraint because the walker detected the kind oneof.
	oneOf, ok := top["oneOf"].([]map[string]any)
	require.True(t, ok, "expected oneOf, got: %#v", top)

	// Six cases — one per kind.
	assert.Len(t, oneOf, 6)

	// Each case should require exactly one field.
	seenFields := make(map[string]bool)
	for _, caseSchema := range oneOf {
		req, ok := caseSchema["required"].([]string)
		require.True(t, ok)
		assert.Len(t, req, 1)
		seenFields[req[0]] = true
	}
	assert.Contains(t, seenFields, "nullValue")
	assert.Contains(t, seenFields, "numberValue")
	assert.Contains(t, seenFields, "stringValue")
	assert.Contains(t, seenFields, "boolValue")
	assert.Contains(t, seenFields, "structValue")
	assert.Contains(t, seenFields, "listValue")
}

func TestFromMessage_Map(t *testing.T) {
	// structpb.Struct has fields: map<string, Value> fields. We
	// disable well-known handling on Struct to exercise the map code
	// path, and also on Value so map values are walked rather than
	// short-circuited.
	withoutWellKnown(t, "google.protobuf.Struct")
	withoutWellKnown(t, "google.protobuf.Value")

	schema := FromMessage(&structpb.Struct{})
	defs := schema["$defs"].(map[string]map[string]any)
	top := defs["google.protobuf.Struct"]
	props := top["properties"].(map[string]any)

	fields := props["fields"].(map[string]any)
	assert.Equal(t, "object", fields["type"])
	addl, ok := fields["additionalProperties"].(map[string]any)
	require.True(t, ok, "map field must have additionalProperties, got %#v", fields)
	// The value type is a $ref to Value because Value is now walkable.
	assert.Equal(t, "#/$defs/google.protobuf.Value", addl["$ref"])
}

// ---- Structural / round-trip validity ----

func TestFromDescriptor_OutputIsValidJSON(t *testing.T) {
	// The walker output must round-trip cleanly through encoding/json.
	// This is the safety check that nothing exotic (non-serializable
	// types, cycles, etc.) snuck into the schema.
	schema := FromMessage(&descriptorpb.FileDescriptorProto{})

	encoded, err := json.Marshal(schema)
	require.NoError(t, err, "schema must be JSON-marshalable")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))

	// Basic structural sanity: root has $schema, $ref, $defs.
	assert.Equal(t, draftURI, decoded["$schema"])
	assert.NotEmpty(t, decoded["$ref"])
	assert.NotNil(t, decoded["$defs"])
}

func TestFromDescriptor_OutputMatchesDraftURI(t *testing.T) {
	schema := FromMessage(&descriptorpb.FileDescriptorProto{})
	assert.Equal(t, "https://json-schema.org/draft/2020-12/schema", schema["$schema"])
}

// ---- Empty messages ----

func TestFromMessage_EmptyMessage_NoFields(t *testing.T) {
	// Disable well-known handling for Empty so we walk it as an
	// ordinary message.
	withoutWellKnown(t, "google.protobuf.Empty")

	schema := FromMessage(&emptypb.Empty{})
	defs := schema["$defs"].(map[string]map[string]any)
	top := defs["google.protobuf.Empty"]

	assert.Equal(t, "object", top["type"])
	assert.Equal(t, "Empty", top["title"])
	// No properties key should be present when a message has no fields.
	_, hasProps := top["properties"]
	assert.False(t, hasProps, "empty message should not emit properties key")
}

