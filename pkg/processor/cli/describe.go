package cli

import (
	"encoding/json"
	"io"
	"reflect"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/processor/jsonschema"
)

// describeFlagName is the canonical flag every CLI helper registers
// to request the describe envelope. Exported as a constant so that
// nebu can use the same name when shelling out.
const describeFlagName = "describe-json"

// reservedFlagNames is the set of flags that are filtered out of the
// describe envelope. These are either nebu infrastructure
// (describe-json itself) or POSIX conventions (help, version) that
// every CLI tool has — not interesting as processor configuration.
var reservedFlagNames = map[string]bool{
	describeFlagName: true,
	"help":           true,
	"version":        true,
}

// writeDescribeJSON encodes the envelope to w as pretty-printed JSON
// with a trailing newline. The cli helpers call this after building
// the envelope and then exit normally.
func writeDescribeJSON(w io.Writer, env processor.DescribeEnvelope) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}

// collectFlagInfo walks the cobra command's flagset and returns a
// DescribeFlag for every visible flag except the describe-json flag
// itself (which is nebu infrastructure, not processor configuration).
//
// The "required" field is derived from cobra's required-flag
// annotation, which is how cobra.Command.MarkFlagRequired records
// the state.
func collectFlagInfo(cmd *cobra.Command) []processor.DescribeFlag {
	var flags []processor.DescribeFlag
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || reservedFlagNames[f.Name] {
			return
		}
		flags = append(flags, processor.DescribeFlag{
			Name:        f.Name,
			Type:        f.Value.Type(),
			Required:    flagIsRequired(f),
			Description: f.Usage,
			Default:     f.DefValue,
		})
	})
	return flags
}

// flagIsRequired returns true if cobra has marked the flag as
// required via Command.MarkFlagRequired. Cobra records this state in
// the flag's annotations map under a well-known key.
func flagIsRequired(f *pflag.Flag) bool {
	if f.Annotations == nil {
		return false
	}
	vals, ok := f.Annotations[cobra.BashCompOneRequiredFlag]
	if !ok || len(vals) == 0 {
		return false
	}
	return vals[0] == "true"
}

// examplesFromHelp converts the processor's HelpConfig examples into
// the stable DescribeExample form for the envelope. Returns nil if
// help is not configured.
func examplesFromHelp(help *HelpConfig) []processor.DescribeExample {
	if help == nil || len(help.Examples) == 0 {
		return nil
	}
	out := make([]processor.DescribeExample, len(help.Examples))
	for i, ex := range help.Examples {
		out[i] = processor.DescribeExample{
			Comment: ex.Comment,
			Command: ex.Command,
		}
	}
	return out
}

// descriptorOf returns the protobuf descriptor for the generic type
// parameter T without requiring the caller to supply an instance.
//
// The reflection dance is necessary because a bare zero-valued T
// (e.g., (*TokenTransferEvent)(nil)) would cause ProtoReflect() to
// be called on a nil pointer. We use reflect.New to allocate a fresh
// non-nil instance of the pointed-to type instead.
func descriptorOf[T proto.Message]() protoreflect.MessageDescriptor {
	var zero T
	rt := reflect.TypeOf(zero)
	if rt == nil {
		// T is an interface type with no dynamic type; nothing to reflect.
		return nil
	}
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	fresh, ok := reflect.New(rt).Interface().(proto.Message)
	if !ok {
		return nil
	}
	return fresh.ProtoReflect().Descriptor()
}

// buildOriginEnvelopeFromDescriptor constructs the describe envelope
// for an origin processor, given the cobra command (for flags) and
// the output message descriptor.
func buildOriginEnvelopeFromDescriptor(
	cmd *cobra.Command,
	config OriginConfig,
	outputDesc protoreflect.MessageDescriptor,
) processor.DescribeEnvelope {
	env := processor.DescribeEnvelope{
		Name:        config.Name,
		Type:        processor.TypeOrigin.String(),
		Version:     config.Version,
		Description: config.Description,
		Schema: processor.DescribeSchema{
			ID:     config.SchemaID,
			Output: jsonschema.FromDescriptor(outputDesc),
		},
		Flags:    collectFlagInfo(cmd),
		Examples: examplesFromHelp(config.Help),
	}
	return env
}
