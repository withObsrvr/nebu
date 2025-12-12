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
	Name        string             `yaml:"name"`
	Type        string             `yaml:"type"` // origin, transform, sink
	Description string             `yaml:"description"`
	Location    ProcessorLocation  `yaml:"location"`
	Service     *ServiceConfig     `yaml:"service,omitempty"`
	Proto       *ProtoConfig       `yaml:"proto,omitempty"`
	Manifest    string             `yaml:"manifest,omitempty"`
	Maintainer  *MaintainerInfo    `yaml:"maintainer,omitempty"`
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

// ResolvePath resolves a relative path in the processor location to an absolute path.
func (p *ProcessorEntry) ResolvePath(basePath string) string {
	if p.Location.Type == "local" && !filepath.IsAbs(p.Location.Path) {
		return filepath.Join(basePath, p.Location.Path)
	}
	return p.Location.Path
}
