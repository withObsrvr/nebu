package registry

import (
	_ "embed"
	"testing"
)

//go:embed test_registry.yaml
var testEmbeddedRegistry []byte

func TestEmbedYAML(t *testing.T) {
	if len(testEmbeddedRegistry) == 0 {
		t.Fatal("Embedded YAML is empty")
	}

	// Try to parse it
	reg, err := LoadFromBytes(testEmbeddedRegistry)
	if err != nil {
		t.Fatalf("Failed to parse embedded YAML: %v", err)
	}

	if reg.Version == 0 {
		t.Error("Registry version is 0")
	}

	if len(reg.Processors) == 0 {
		t.Error("No processors found in embedded registry")
	}

	t.Logf("Successfully embedded and parsed registry with %d processors", len(reg.Processors))
}
