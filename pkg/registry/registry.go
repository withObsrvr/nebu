// Package registry provides processor discovery and management.
package registry

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed registry.yaml
var embeddedRegistry []byte

// Registry represents the processor registry.
type Registry struct {
	Version    int                `yaml:"version"`
	Processors []ProcessorEntry `yaml:"processors"`
}

// ProcessorEntry describes a processor in the registry.
type ProcessorEntry struct {
	Name            string             `yaml:"name"`
	Type            string             `yaml:"type"` // origin, transform, sink
	Description     string             `yaml:"description"`
	LongDescription string             `yaml:"long_description,omitempty"`
	Location        ProcessorLocation  `yaml:"location"`
	Service         *ServiceConfig     `yaml:"service,omitempty"`
	Proto           *ProtoConfig       `yaml:"proto,omitempty"`
	Schema          *SchemaConfig      `yaml:"schema,omitempty"`
	Manifest        string             `yaml:"manifest,omitempty"`
	Maintainer      *MaintainerInfo    `yaml:"maintainer,omitempty"`
	Events          []EventInfo        `yaml:"events,omitempty"`
	OutputFields    []FieldInfo        `yaml:"output_fields,omitempty"`
	Examples        []ExampleInfo      `yaml:"examples,omitempty"`
	WorksWith       *WorksWithInfo     `yaml:"works_with,omitempty"`
	Source          string             `yaml:"-"` // "embedded" or "community", not serialized
}

// SchemaConfig describes the output schema.
type SchemaConfig struct {
	Version       string `yaml:"version"`
	Identifier    string `yaml:"identifier"`
	Documentation string `yaml:"documentation,omitempty"`
}

// EventInfo describes an event type produced by the processor.
type EventInfo struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// FieldInfo describes an output field.
type FieldInfo struct {
	Path        string `yaml:"path"`
	Description string `yaml:"description"`
}

// ExampleInfo describes a usage example.
type ExampleInfo struct {
	Comment string `yaml:"comment"`
	Command string `yaml:"command"`
}

// WorksWithInfo describes processor compatibility.
type WorksWithInfo struct {
	Input      []string `yaml:"input,omitempty"`
	Transforms []string `yaml:"transforms,omitempty"`
	Sinks      []string `yaml:"sinks,omitempty"`
}

// ProcessorLocation describes where the processor code lives.
type ProcessorLocation struct {
	Type          string `yaml:"type"` // local, git, module
	Path          string `yaml:"path,omitempty"`
	ModulePackage string `yaml:"module_package,omitempty"` // Go module path for go install
	URL           string `yaml:"url,omitempty"`
	Package       string `yaml:"package,omitempty"`
}

// ServiceConfig describes how to run the processor as a standalone service.
type ServiceConfig struct {
	Binary      string   `yaml:"binary"`
	DefaultPort int      `yaml:"default_port,omitempty"`
	Env         []string `yaml:"env,omitempty"`
}

// ProtoConfig describes the protobuf definitions.
type ProtoConfig struct {
	Source  string `yaml:"source"`
	Package string `yaml:"package"`
}

// MaintainerInfo describes who maintains the processor.
type MaintainerInfo struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url,omitempty"`
}

// Load loads the registry from a YAML file.
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry file: %w", err)
	}

	return LoadFromBytes(data)
}

// LoadFromBytes loads a registry from bytes.
func LoadFromBytes(data []byte) (*Registry, error) {
	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("failed to parse registry YAML: %w", err)
	}

	return &reg, nil
}

// LoadDefault loads the default registry from the nebu repo root.
// It first tries to load from the file system (for development with cloned repo),
// then falls back to the embedded registry (for go install users).
func LoadDefault() (*Registry, error) {
	// Try file system first (dev mode with cloned repo)
	paths := []string{
		"registry.yaml",
		"../registry.yaml",
		"../../registry.yaml",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return Load(path)
		}
	}

	// Fall back to embedded registry (go install mode)
	return LoadFromBytes(embeddedRegistry)
}

// FindProcessor finds a processor by name in the registry.
func (r *Registry) FindProcessor(name string) (*ProcessorEntry, error) {
	for _, proc := range r.Processors {
		if proc.Name == name {
			return &proc, nil
		}
	}
	return nil, fmt.Errorf("processor not found: %s", name)
}

// ListProcessors returns all processors of a given type.
// If procType is empty, returns all processors.
func (r *Registry) ListProcessors(procType string) []ProcessorEntry {
	if procType == "" {
		return r.Processors
	}

	var result []ProcessorEntry
	for _, proc := range r.Processors {
		if proc.Type == procType {
			result = append(result, proc)
		}
	}
	return result
}

// ListProcessorsByType returns processors grouped by type.
func (r *Registry) ListProcessorsByType() map[string][]ProcessorEntry {
	result := make(map[string][]ProcessorEntry)
	for _, proc := range r.Processors {
		result[proc.Type] = append(result[proc.Type], proc)
	}
	return result
}

// ListProcessorNames returns all processor names.
func (r *Registry) ListProcessorNames() []string {
	names := make([]string, len(r.Processors))
	for i, proc := range r.Processors {
		names[i] = proc.Name
	}
	return names
}

// ResolvePath resolves a relative path in the processor location to an absolute path.
func (p *ProcessorEntry) ResolvePath(basePath string) string {
	if p.Location.Type == "local" && !filepath.IsAbs(p.Location.Path) {
		return filepath.Join(basePath, p.Location.Path)
	}
	return p.Location.Path
}

const defaultExternalRegistry = "github.com/withObsrvr/nebu-processor-registry"

// LoadAll loads the embedded registry and merges in external registry processors.
// External processors that conflict with embedded names are skipped.
func LoadAll() (*Registry, error) {
	// Load embedded registry
	reg, err := LoadDefault()
	if err != nil {
		return nil, err
	}

	// Mark all embedded entries
	for i := range reg.Processors {
		reg.Processors[i].Source = "embedded"
	}

	// Determine external registry URL
	registryURL := os.Getenv("NEBU_REGISTRY")
	if registryURL == "" {
		registryURL = defaultExternalRegistry
	}

	// Fetch external processors
	extReg, err := NewExternalRegistry(registryURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not initialize external registry: %v\n", err)
		return reg, nil
	}

	extEntries, err := extReg.FetchAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch external registry: %v\n", err)
		return reg, nil
	}

	// Build set of embedded names for dedup
	embeddedNames := make(map[string]bool, len(reg.Processors))
	for _, p := range reg.Processors {
		embeddedNames[p.Name] = true
	}

	// Merge, skipping conflicts
	for _, ext := range extEntries {
		if embeddedNames[ext.Name] {
			continue
		}
		reg.Processors = append(reg.Processors, ext)
	}

	return reg, nil
}
