package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"

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

// isDescribeJSONRequested reports whether --describe-json (or any
// truthy --describe-json=<value> form) appears in os.Args. This is
// deliberately implemented without cobra so it can be checked
// BEFORE cobra.Execute() runs — otherwise cobra's required-flag
// validation would reject describe invocations on processors that
// mark any flag required (e.g., postgres-sink requires --dsn). The
// describe protocol must work without any other flags set.
//
// Both bare (`--describe-json`) and value-bearing
// (`--describe-json=true`, `--describe-json=1`) forms are accepted,
// matching how cobra/pflag would parse the flag. `--describe-json=false`
// (and other false values) is honored as "not requested".
func isDescribeJSONRequested() bool {
	flag := "--" + describeFlagName
	flagEq := flag + "="
	for _, arg := range os.Args[1:] {
		if arg == flag {
			return true
		}
		if val, ok := strings.CutPrefix(arg, flagEq); ok {
			// Use strconv.ParseBool so we match pflag's bool parsing
			// (1/t/T/TRUE/true/True, and their negative counterparts).
			b, err := strconv.ParseBool(val)
			return err == nil && b
		}
	}
	return false
}

// emitDescribeIfRequested checks if --describe-json was passed and,
// if so, writes the envelope to stdout and exits 0. Called at the
// top of every Run*CLI helper before cobra.Execute() so the describe
// path bypasses cobra's flag validation entirely.
func emitDescribeIfRequested(env processor.DescribeEnvelope) {
	if !isDescribeJSONRequested() {
		return
	}
	if err := writeDescribeJSON(os.Stdout, env); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

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

// buildOriginEnvelope constructs the describe envelope for an origin
// processor, given the cobra command (for flags) and the proto
// descriptor of the emitted event type.
func buildOriginEnvelope(
	cmd *cobra.Command,
	config OriginConfig,
	outputDesc protoreflect.MessageDescriptor,
) processor.DescribeEnvelope {
	return processor.DescribeEnvelope{
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
}

// buildTransformEnvelope constructs the describe envelope for a
// transform processor. Input and Output schemas are generated from
// the optional InputType/OutputType fields on the config; a transform
// that operates on arbitrary JSON can leave both nil and consumers
// will see an envelope with only the schema.id field populated.
func buildTransformEnvelope(
	cmd *cobra.Command,
	config TransformConfig,
) processor.DescribeEnvelope {
	schema := processor.DescribeSchema{ID: config.SchemaID}
	if config.InputType != nil {
		schema.Input = jsonschema.FromMessage(config.InputType)
	}
	if config.OutputType != nil {
		schema.Output = jsonschema.FromMessage(config.OutputType)
	}
	return processor.DescribeEnvelope{
		Name:        config.Name,
		Type:        processor.TypeTransform.String(),
		Version:     config.Version,
		Description: config.Description,
		Schema:      schema,
		Flags:       collectFlagInfo(cmd),
		Examples:    examplesFromHelp(config.Help),
	}
}

// buildSinkEnvelope constructs the describe envelope for a sink
// processor. Only the Input schema is generated (sinks have no
// output).
func buildSinkEnvelope(
	cmd *cobra.Command,
	config SinkConfig,
) processor.DescribeEnvelope {
	schema := processor.DescribeSchema{ID: config.SchemaID}
	if config.InputType != nil {
		schema.Input = jsonschema.FromMessage(config.InputType)
	}
	return processor.DescribeEnvelope{
		Name:        config.Name,
		Type:        processor.TypeSink.String(),
		Version:     config.Version,
		Description: config.Description,
		Schema:      schema,
		Flags:       collectFlagInfo(cmd),
		Examples:    examplesFromHelp(config.Help),
	}
}
