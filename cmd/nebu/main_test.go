package main

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
)

func TestCommandExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "usage missing argument", args: []string{"--program-status=never", "fetch"}, want: 2},
		{name: "usage bad flag", args: []string{"--program-status=never", "fetch", "--not-a-flag"}, want: 2},
		{name: "processor not found", args: []string{"--program-status=never", "install", "does-not-exist"}, want: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			programStatusStarted = false
			root := newRootCmd()
			root.SetArgs(test.args)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			if got := nebuErrors.ExitCode(root.Execute()); got != test.want {
				t.Fatalf("command exit code = %d, want %d", got, test.want)
			}
		})
	}
}

func TestUnreachableSourceExitCode(t *testing.T) {
	err := nebuErrors.StreamError(errors.New("connection refused"))
	assert.Equal(t, 4, nebuErrors.ExitCode(err))
}

func TestValidateProgramStatusMode(t *testing.T) {
	for _, mode := range []string{"auto", "always", "never"} {
		t.Run(mode, func(t *testing.T) {
			require.NoError(t, validateProgramStatusMode(mode))
		})
	}
	require.ErrorContains(t, validateProgramStatusMode("sometimes"), "expected auto, always, or never")
}

func TestTerminalFileRejectsNonTerminalCharacterDevice(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, devNull.Close()) })

	assert.False(t, terminalFile(devNull))
}

func TestOneLineProducesValidProtocolText(t *testing.T) {
	assert.Equal(t, "first second third", oneLine("first\nsecond\tthird"))

	got := oneLine(strings.Repeat("a", 2047) + "€")
	assert.LessOrEqual(t, len(got), 2048)
	assert.True(t, utf8.ValidString(got))
}
