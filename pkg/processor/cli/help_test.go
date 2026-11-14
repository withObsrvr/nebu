package cli

import (
	"regexp"
	"strings"
	"testing"
)

// These examples are generated from a processor's name, and every processor
// built against a nebu release prints them, so one wrong line here is wrong
// in thirty-odd help texts at once. Two have shipped that way: an example
// that invoked the processor with --follow, which no origin defines, and
// `jq 'select(.type == "transfer")'`, which no origin's events can match.
// The tests below encode what the generator is allowed to assume.

// selectOnField matches a jq select on a top-level field, which is the
// pattern that silently matched nothing.
var selectOnField = regexp.MustCompile(`select\(\s*\.([A-Za-z_][A-Za-z0-9_]*)`)

func TestDefaultOriginExamplesClaimOnlyWhatANameImplies(t *testing.T) {
	const name = "token-transfer"
	examples := DefaultOriginExamples(name)
	if len(examples) == 0 {
		t.Fatal("no origin examples generated")
	}

	for _, example := range examples {
		t.Run(example.Comment, func(t *testing.T) {
			// --follow belongs to nebu fetch. Origin processors reach a live
			// stream by piping from it, never by taking the flag themselves.
			for _, stage := range strings.Split(example.Command, "|") {
				words := strings.Fields(stage)
				if len(words) > 0 && words[0] == name && strings.Contains(stage, "--follow") {
					t.Errorf("example invokes the processor with --follow: %q", example.Command)
				}
			}

			// A generator that knows only the processor's name cannot know
			// its field names. _schema is the exception: every nebu event
			// carries it, whatever the processor.
			for _, match := range selectOnField.FindAllStringSubmatch(example.Command, -1) {
				if match[1] != "_schema" {
					t.Errorf("example filters on %q, a field this generator cannot know: %q",
						match[1], example.Command)
				}
			}
		})
	}
}

func TestDefaultTransformExamplesAreRunnable(t *testing.T) {
	for _, example := range DefaultTransformExamples("token-transfer", "usdc-filter") {
		if strings.Contains(example.Command, "...") {
			t.Errorf("example is not a command a reader can run: %q", example.Command)
		}
	}
}

func TestDefaultSinkExamplesGuardBeforeConverting(t *testing.T) {
	tests := []struct {
		name       string
		originName string
		wantFilter string
	}{
		{
			name:       "known origin gets a filter true of its events",
			originName: "token-transfer",
			wantFilter: ".transfer != null",
		},
		{
			// Anything else gets a passthrough. A filter that silently
			// matches nothing is worse documentation than no filter.
			name:       "unknown origin gets no guess",
			originName: "contract-events",
			wantFilter: "jq -c .",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			examples := DefaultSinkExamples(tt.originName, "json-file-sink")
			joined := ""
			for _, example := range examples {
				joined += example.Command + "\n"
				if strings.Contains(example.Command, "...") {
					t.Errorf("example is not runnable: %q", example.Command)
				}
				// A ledger holds fees, mints and burns beside transfers, so
				// an unguarded tonumber errors once per non-transfer event.
				if strings.Contains(example.Command, "tonumber") &&
					!strings.Contains(example.Command, "!= null") {
					t.Errorf("tonumber without a guard on the event kind: %q", example.Command)
				}
			}
			if !strings.Contains(joined, tt.wantFilter) {
				t.Errorf("examples for origin %q lack %q:\n%s", tt.originName, tt.wantFilter, joined)
			}
		})
	}
}
