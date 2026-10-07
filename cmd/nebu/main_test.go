package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateProgramStatusMode(t *testing.T) {
	for _, mode := range []string{"auto", "always", "never"} {
		t.Run(mode, func(t *testing.T) {
			require.NoError(t, validateProgramStatusMode(mode))
		})
	}
	require.ErrorContains(t, validateProgramStatusMode("sometimes"), "expected auto, always, or never")
}

func TestOneLineProducesValidProtocolText(t *testing.T) {
	assert.Equal(t, "first second third", oneLine("first\nsecond\tthird"))

	got := oneLine(strings.Repeat("a", 2047) + "€")
	assert.LessOrEqual(t, len(got), 2048)
	assert.True(t, utf8.ValidString(got))
}
