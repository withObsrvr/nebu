package cli

import (
	"strings"
	"testing"
)

func TestDefaultOriginExamplesDoNotAdvertiseProcessorFollowFlag(t *testing.T) {
	for _, example := range DefaultOriginExamples("token-transfer") {
		if strings.Contains(example.Command, "token-transfer --follow") ||
			strings.Contains(example.Command, "token-transfer --start-ledger 60200000 --follow") {
			t.Fatalf("processor example advertises nonexistent --follow flag: %q", example.Command)
		}
	}
}
