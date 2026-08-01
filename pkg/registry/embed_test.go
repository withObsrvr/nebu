package registry

import (
	_ "embed"
	"testing"
)

//go:embed test_registry.yaml
var testEmbeddedRegistry []byte

func TestBuiltInRegistryContainsOnlyInRepoProcessors(t *testing.T) {
	reg, err := LoadFromBytes(embeddedRegistry)
	if err != nil {
		t.Fatalf("parse built-in registry: %v", err)
	}

	want := map[string]bool{
		"transaction-stats":   true,
		"ledger-change-stats": true,
	}
	if len(reg.Processors) != len(want) {
		t.Fatalf("built-in registry has %d processors, want %d", len(reg.Processors), len(want))
	}
	for _, proc := range reg.Processors {
		if !want[proc.Name] {
			t.Errorf("built-in registry unexpectedly contains %q", proc.Name)
		}
	}
}

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

func TestListProcessorsByType(t *testing.T) {
	reg, err := LoadFromBytes(testEmbeddedRegistry)
	if err != nil {
		t.Fatalf("Failed to parse registry: %v", err)
	}

	byType := reg.ListProcessorsByType()

	// test_registry.yaml has one origin processor
	if origins, ok := byType["origin"]; !ok || len(origins) == 0 {
		t.Error("Expected at least one origin processor")
	}

	// Verify all processors are accounted for
	total := 0
	for _, procs := range byType {
		total += len(procs)
	}
	if total != len(reg.Processors) {
		t.Errorf("ListProcessorsByType returned %d total processors, want %d", total, len(reg.Processors))
	}
}

func TestListProcessorsByType_Empty(t *testing.T) {
	reg := &Registry{}
	byType := reg.ListProcessorsByType()

	if len(byType) != 0 {
		t.Errorf("Expected empty map for empty registry, got %d entries", len(byType))
	}
}

func TestListProcessorNames(t *testing.T) {
	reg, err := LoadFromBytes(testEmbeddedRegistry)
	if err != nil {
		t.Fatalf("Failed to parse registry: %v", err)
	}

	names := reg.ListProcessorNames()

	if len(names) != len(reg.Processors) {
		t.Errorf("ListProcessorNames returned %d names, want %d", len(names), len(reg.Processors))
	}

	// Verify the test processor name is present
	found := false
	for _, name := range names {
		if name == "test-processor" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'test-processor' in processor names")
	}
}

func TestListProcessorNames_Empty(t *testing.T) {
	reg := &Registry{}
	names := reg.ListProcessorNames()

	if len(names) != 0 {
		t.Errorf("Expected empty slice for empty registry, got %d entries", len(names))
	}
}
