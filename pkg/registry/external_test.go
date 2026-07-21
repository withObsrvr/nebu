package registry

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

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
			Name:        "webhook-sink",
			Type:        "sink",
			Description: "POST events to an HTTP endpoint",
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

	if ks := byName["kafka-sink"]; ks.Install != nil {
		t.Errorf("kafka-sink gained an Install block through the cache: %+v", ks.Install)
	}
}
