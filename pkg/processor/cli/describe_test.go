package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/withObsrvr/nebu/pkg/processor"
)

// buildTestCommand creates a minimal cobra command that exercises the
// flag categories collectFlagInfo should handle: a required flag, an
// optional flag with a default, a hidden flag, and the reserved
// describe-json / help / version flags that must be filtered out.
func buildTestCommand(t *testing.T) *cobra.Command {
	t.Helper()

	var (
		required string
		optional int
		hidden   bool
		describe bool
	)

	cmd := &cobra.Command{
		Use:     "test-proc",
		Version: "0.1.2",
	}
	cmd.Flags().StringVar(&required, "required-flag", "", "A flag that must be set")
	cmd.Flags().IntVar(&optional, "optional-flag", 42, "An optional flag with a default")
	cmd.Flags().BoolVar(&hidden, "hidden-flag", false, "Should not appear in describe output")
	_ = cmd.Flags().MarkHidden("hidden-flag")
	cmd.Flags().BoolVar(&describe, describeFlagName, false, "infrastructure flag")

	require.NoError(t, cmd.MarkFlagRequired("required-flag"))
	return cmd
}

func TestCollectFlagInfo_FiltersReservedAndHiddenFlags(t *testing.T) {
	cmd := buildTestCommand(t)

	flags := collectFlagInfo(cmd)

	// Expect exactly two flags: required-flag and optional-flag.
	require.Len(t, flags, 2)

	names := []string{flags[0].Name, flags[1].Name}
	assert.Contains(t, names, "required-flag")
	assert.Contains(t, names, "optional-flag")
	assert.NotContains(t, names, "hidden-flag")
	assert.NotContains(t, names, describeFlagName)
	assert.NotContains(t, names, "help")
	assert.NotContains(t, names, "version")
}

func TestCollectFlagInfo_MarksRequired(t *testing.T) {
	cmd := buildTestCommand(t)

	flags := collectFlagInfo(cmd)
	byName := make(map[string]processor.DescribeFlag)
	for _, f := range flags {
		byName[f.Name] = f
	}

	assert.True(t, byName["required-flag"].Required, "required-flag must be marked required")
	assert.False(t, byName["optional-flag"].Required, "optional-flag must not be marked required")
}

func TestCollectFlagInfo_PreservesTypeAndDefault(t *testing.T) {
	cmd := buildTestCommand(t)

	flags := collectFlagInfo(cmd)
	byName := make(map[string]processor.DescribeFlag)
	for _, f := range flags {
		byName[f.Name] = f
	}

	assert.Equal(t, "string", byName["required-flag"].Type)
	assert.Equal(t, "int", byName["optional-flag"].Type)
	assert.Equal(t, "42", byName["optional-flag"].Default)
}

func TestWriteDescribeJSON_RoundTrip(t *testing.T) {
	// Build a realistic envelope, write it, parse it back, and assert
	// the fields survived the round-trip. This is the contract check
	// that ensures the envelope type and the JSON encoding are in
	// agreement.
	env := processor.DescribeEnvelope{
		Name:        "test-proc",
		Type:        "origin",
		Version:     "1.2.3",
		Description: "A test processor",
		Schema: processor.DescribeSchema{
			ID: "nebu.test.v1",
			Output: map[string]any{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type":    "object",
			},
		},
		Flags: []processor.DescribeFlag{
			{Name: "flag1", Type: "string", Required: true, Description: "First flag"},
			{Name: "flag2", Type: "int", Required: false, Description: "Second flag", Default: "0"},
		},
		Examples: []processor.DescribeExample{
			{Comment: "Basic", Command: "test-proc --flag1 value"},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeDescribeJSON(&buf, env))

	var decoded processor.DescribeEnvelope
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))

	assert.Equal(t, env.Name, decoded.Name)
	assert.Equal(t, env.Type, decoded.Type)
	assert.Equal(t, env.Version, decoded.Version)
	assert.Equal(t, env.Description, decoded.Description)
	assert.Equal(t, env.Schema.ID, decoded.Schema.ID)
	assert.Equal(t, "object", decoded.Schema.Output["type"])
	require.Len(t, decoded.Flags, 2)
	assert.Equal(t, "flag1", decoded.Flags[0].Name)
	assert.True(t, decoded.Flags[0].Required)
	require.Len(t, decoded.Examples, 1)
	assert.Equal(t, "test-proc --flag1 value", decoded.Examples[0].Command)
}

func TestDescriptorOf_ReturnsRealDescriptor(t *testing.T) {
	// descriptorOf must return a non-nil descriptor for a concrete
	// proto type, derived entirely from the generic type parameter
	// without requiring an instance.
	desc := descriptorOf[*wrapperspb.StringValue]()
	require.NotNil(t, desc)
	assert.Equal(t, "StringValue", string(desc.Name()))
	assert.Equal(t, "google.protobuf.StringValue", string(desc.FullName()))
}

func TestExamplesFromHelp_NilHelp(t *testing.T) {
	assert.Nil(t, examplesFromHelp(nil))
}

func TestExamplesFromHelp_EmptyExamples(t *testing.T) {
	assert.Nil(t, examplesFromHelp(&HelpConfig{}))
}

func TestExamplesFromHelp_Populated(t *testing.T) {
	help := &HelpConfig{
		Examples: []HelpExample{
			{Comment: "run it", Command: "foo --bar"},
			{Comment: "", Command: "foo"},
		},
	}
	got := examplesFromHelp(help)
	require.Len(t, got, 2)
	assert.Equal(t, "run it", got[0].Comment)
	assert.Equal(t, "foo --bar", got[0].Command)
	assert.Equal(t, "", got[1].Comment)
	assert.Equal(t, "foo", got[1].Command)
}
