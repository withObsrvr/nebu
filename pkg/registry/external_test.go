package registry

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestExternalDescriptionMetadata(t *testing.T) {
	document := `
processor:
  name: old-origin
  type: origin
  description: Old origin
  version: 1.2.3
  deprecated: true
  deprecation_notice: Use new-origin instead
repo:
  github: example/processors
  ref: v1.2.3
proto:
  source: example/proto
  package: old_origin
schema:
  version: v2
  identifier: nebu.old_origin.v2
  documentation: https://example.com/schema
metadata:
  category: stellar
  tags: [stellar, soroban]
docs:
  quick_start: old-origin --help
  examples: old-origin | jq
`

	var desc ExternalProcessorDesc
	if err := yaml.Unmarshal([]byte(document), &desc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	entry := desc.ToProcessorEntry()
	if !entry.Deprecated || entry.DeprecationNotice != "Use new-origin instead" {
		t.Fatalf("deprecation metadata = %t, %q", entry.Deprecated, entry.DeprecationNotice)
	}
	if entry.Location.Ref != "v1.2.3" {
		t.Fatalf("location ref = %q, want v1.2.3", entry.Location.Ref)
	}
	if entry.Proto == nil || entry.Proto.Package != "old_origin" {
		t.Fatalf("proto = %+v", entry.Proto)
	}
	if entry.Schema == nil || entry.Schema.Identifier != "nebu.old_origin.v2" {
		t.Fatalf("schema = %+v", entry.Schema)
	}
	if len(entry.Examples) != 2 {
		t.Fatalf("examples = %+v, want quick start and examples", entry.Examples)
	}
	if entry.Metadata["category"] != "stellar" {
		t.Fatalf("metadata = %+v", entry.Metadata)
	}
}

func TestValidateExternalDescriptionRejectsUnsupportedKeys(t *testing.T) {
	document := []byte(`
processor:
  name: example
  type: origin
  description: Example
  version: 1.0.0
  unsupported_key: true
repo:
  github: example/processors
  ref: v1.0.0
`)
	err := ValidateExternalDescription(document)
	if err == nil || !strings.Contains(err.Error(), "unsupported_key") {
		t.Fatalf("ValidateExternalDescription() error = %v, want unsupported_key", err)
	}
}

func TestValidateExternalDescriptionRejectsAdditionalDocuments(t *testing.T) {
	document := []byte(`
processor:
  name: example
  type: origin
  description: Example
  version: 1.0.0
repo:
  github: example/processors
---
processor:
  unsupported_key: true
`)
	err := ValidateExternalDescription(document)
	if err == nil || !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("ValidateExternalDescription() error = %v, want multiple-document rejection", err)
	}
}

func TestExternalDescInstallBlock(t *testing.T) {
	tests := []struct {
		name        string
		yamlDoc     string
		wantInstall *InstallConfig // nil means entry.Install must be nil
	}{
		{
			name: "install block with explicit version",
			yamlDoc: `
processor:
  name: webhook-sink
  type: sink
  description: POST events to an HTTP endpoint
  version: 0.2.0
  language: TypeScript
repo:
  github: withObsrvr/nebu-processor-registry
install:
  kind: binary
  url: https://example.com/releases/webhook-sink-v{version}/webhook-sink-{os}-{arch}
  checksums: https://example.com/releases/webhook-sink-v{version}/checksums.txt
  version: 0.1.0
`,
			wantInstall: &InstallConfig{
				Kind:      "binary",
				URL:       "https://example.com/releases/webhook-sink-v{version}/webhook-sink-{os}-{arch}",
				Checksums: "https://example.com/releases/webhook-sink-v{version}/checksums.txt",
				Version:   "0.1.0",
			},
		},
		{
			name: "install version falls back to processor version",
			yamlDoc: `
processor:
  name: webhook-sink
  type: sink
  description: POST events to an HTTP endpoint
  version: 0.3.0
repo:
  github: withObsrvr/nebu-processor-registry
install:
  kind: binary
  url: https://example.com/{version}/webhook-sink-{os}-{arch}
  checksums: https://example.com/{version}/checksums.txt
`,
			wantInstall: &InstallConfig{
				Kind:      "binary",
				URL:       "https://example.com/{version}/webhook-sink-{os}-{arch}",
				Checksums: "https://example.com/{version}/checksums.txt",
				Version:   "0.3.0",
			},
		},
		{
			name: "no install block",
			yamlDoc: `
processor:
  name: kafka-sink
  type: sink
  description: Publish to Kafka
  version: 1.0.0
repo:
  github: withObsrvr/nebu-processor-registry
`,
			wantInstall: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var desc ExternalProcessorDesc
			if err := yaml.Unmarshal([]byte(tt.yamlDoc), &desc); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			entry := desc.ToProcessorEntry()

			if tt.wantInstall == nil {
				if entry.Install != nil {
					t.Fatalf("entry.Install = %+v, want nil", entry.Install)
				}
				return
			}
			if entry.Install == nil {
				t.Fatal("entry.Install is nil, want a value")
			}
			if *entry.Install != *tt.wantInstall {
				t.Errorf("entry.Install = %+v, want %+v", *entry.Install, *tt.wantInstall)
			}
		})
	}
}

// TestCacheRoundTripPreservesInstall guards against the lossy cache
// round-trip: entries are persisted by reconstructing an
// ExternalProcessorDesc, so any field not carried through writeCache is
// silently dropped on the next cached load.
func TestCacheRoundTripPreservesInstall(t *testing.T) {
	reg := &ExternalRegistry{
		URL:      "github.com/example/test-registry",
		CacheDir: t.TempDir(),
		TTL:      time.Hour,
	}

	entries := []ProcessorEntry{
		{
			Name:              "webhook-sink",
			Type:              "sink",
			Description:       "POST events to an HTTP endpoint",
			Deprecated:        true,
			DeprecationNotice: "Use http-sink",
			Location:          ProcessorLocation{Ref: "v0.1.0"},
			Proto:             &ProtoConfig{Source: "example/proto", Package: "webhook"},
			Schema:            &SchemaConfig{Version: "v1", Identifier: "nebu.webhook.v1"},
			Metadata:          map[string]any{"category": "delivery"},
			Examples:          []ExampleInfo{{Comment: "Quick start", Command: "webhook-sink --help"}},
			Install: &InstallConfig{
				Kind:      "binary",
				URL:       "https://example.com/{version}/webhook-sink-{os}-{arch}",
				Checksums: "https://example.com/{version}/checksums.txt",
				Version:   "0.1.0",
			},
		},
		{
			Name:        "kafka-sink",
			Type:        "sink",
			Description: "Publish to Kafka",
		},
	}

	if err := reg.writeCache(entries); err != nil {
		t.Fatalf("writeCache: %v", err)
	}
	got, err := reg.loadFromCache()
	if err != nil {
		t.Fatalf("loadFromCache: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d entries, want 2", len(got))
	}

	byName := map[string]ProcessorEntry{}
	for _, e := range got {
		byName[e.Name] = e
	}

	ws, ok := byName["webhook-sink"]
	if !ok {
		t.Fatal("webhook-sink missing from cached entries")
	}
	if ws.Install == nil {
		t.Fatal("webhook-sink Install was dropped by the cache round-trip")
	}
	if *ws.Install != *entries[0].Install {
		t.Errorf("cached Install = %+v, want %+v", *ws.Install, *entries[0].Install)
	}
	if !ws.Deprecated || ws.DeprecationNotice != entries[0].DeprecationNotice || ws.Location.Ref != "v0.1.0" {
		t.Errorf("cached metadata was dropped: %+v", ws)
	}
	if ws.Proto == nil || *ws.Proto != *entries[0].Proto || ws.Schema == nil || *ws.Schema != *entries[0].Schema {
		t.Errorf("cached proto/schema was dropped: proto=%+v schema=%+v", ws.Proto, ws.Schema)
	}
	if ws.Metadata["category"] != "delivery" {
		t.Errorf("cached metadata map was dropped: %+v", ws.Metadata)
	}

	if ks := byName["kafka-sink"]; ks.Install != nil {
		t.Errorf("kafka-sink gained an Install block through the cache: %+v", ks.Install)
	}
}
