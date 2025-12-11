package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Scaffold new processors and components",
		Long:  `Create boilerplate code for new processors, transforms, or sinks.`,
	}

	cmd.AddCommand(newProcessorCmd())

	return cmd
}

func newProcessorCmd() *cobra.Command {
	var processorType string

	cmd := &cobra.Command{
		Use:   "processor [name]",
		Short: "Create a new processor skeleton",
		Long: `Scaffold a new processor with boilerplate code.

Processor types:
  origin     - Consumes ledgers from Stellar, emits events
  transform  - Consumes events, emits transformed events
  sink       - Consumes events, produces side effects

Example:
  nebu new processor my-filter --type transform
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			// Validate processor type
			validTypes := map[string]bool{
				"origin":    true,
				"transform": true,
				"sink":      true,
			}

			if !validTypes[processorType] {
				return fmt.Errorf("invalid processor type: %s (must be origin, transform, or sink)", processorType)
			}

			return scaffoldProcessor(name, processorType)
		},
	}

	cmd.Flags().StringVarP(&processorType, "type", "t", "origin", "Processor type (origin, transform, sink)")

	return cmd
}

func scaffoldProcessor(name, procType string) error {
	// Convert name to safe identifier
	safeName := strings.ReplaceAll(name, "-", "_")
	pkgName := strings.ToLower(safeName)

	// Create directory structure
	baseDir := filepath.Join("processors", "examples", name)
	protoDir := filepath.Join(baseDir, "proto")

	if err := os.MkdirAll(protoDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate files based on processor type
	var err error
	switch procType {
	case "origin":
		err = generateOriginProcessor(baseDir, protoDir, name, pkgName, safeName)
	case "transform":
		err = generateTransformProcessor(baseDir, protoDir, name, pkgName, safeName)
	case "sink":
		err = generateSinkProcessor(baseDir, name, pkgName, safeName)
	}

	if err != nil {
		return err
	}

	fmt.Printf("✓ Created processor: %s\n", baseDir)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. Edit %s/processor.go - implement your logic\n", baseDir)
	if procType != "sink" {
		fmt.Printf("  2. Edit %s/proto/*.proto - define your event schema\n", baseDir)
		fmt.Println("  3. Run: make proto (or scripts/gen-protos.sh)")
	}
	fmt.Printf("  4. Build and test your processor\n")

	return nil
}

func generateOriginProcessor(baseDir, protoDir, name, pkgName, safeName string) error {
	// Generate processor.go
	processorFile := filepath.Join(baseDir, "processor.go")
	if err := writeTemplate(processorFile, originProcessorTemplate, map[string]string{
		"PackageName":   pkgName,
		"ProcessorName": safeName,
		"Name":          name,
	}); err != nil {
		return err
	}

	// Generate proto file
	protoFile := filepath.Join(protoDir, pkgName+".proto")
	if err := writeTemplate(protoFile, protoTemplate, map[string]string{
		"PackageName": pkgName,
		"Name":        name,
	}); err != nil {
		return err
	}

	// Generate manifest
	manifestFile := filepath.Join(baseDir, "manifest.yaml")
	if err := writeTemplate(manifestFile, manifestTemplate, map[string]string{
		"Name": name,
		"Type": "origin",
	}); err != nil {
		return err
	}

	// Generate README
	readmeFile := filepath.Join(baseDir, "README.md")
	return writeTemplate(readmeFile, readmeTemplate, map[string]string{
		"Name": name,
		"Type": "origin",
	})
}

func generateTransformProcessor(baseDir, protoDir, name, pkgName, safeName string) error {
	// Generate processor.go
	processorFile := filepath.Join(baseDir, "processor.go")
	if err := writeTemplate(processorFile, transformProcessorTemplate, map[string]string{
		"PackageName":   pkgName,
		"ProcessorName": safeName,
		"Name":          name,
	}); err != nil {
		return err
	}

	// Generate proto file
	protoFile := filepath.Join(protoDir, pkgName+".proto")
	if err := writeTemplate(protoFile, protoTemplate, map[string]string{
		"PackageName": pkgName,
		"Name":        name,
	}); err != nil {
		return err
	}

	// Generate manifest
	manifestFile := filepath.Join(baseDir, "manifest.yaml")
	if err := writeTemplate(manifestFile, manifestTemplate, map[string]string{
		"Name": name,
		"Type": "transform",
	}); err != nil {
		return err
	}

	// Generate README
	readmeFile := filepath.Join(baseDir, "README.md")
	return writeTemplate(readmeFile, readmeTemplate, map[string]string{
		"Name": name,
		"Type": "transform",
	})
}

func generateSinkProcessor(baseDir, name, pkgName, safeName string) error {
	// Generate processor.go
	processorFile := filepath.Join(baseDir, "processor.go")
	if err := writeTemplate(processorFile, sinkProcessorTemplate, map[string]string{
		"PackageName":   pkgName,
		"ProcessorName": safeName,
		"Name":          name,
	}); err != nil {
		return err
	}

	// Generate manifest
	manifestFile := filepath.Join(baseDir, "manifest.yaml")
	if err := writeTemplate(manifestFile, manifestTemplate, map[string]string{
		"Name": name,
		"Type": "sink",
	}); err != nil {
		return err
	}

	// Generate README
	readmeFile := filepath.Join(baseDir, "README.md")
	return writeTemplate(readmeFile, readmeTemplate, map[string]string{
		"Name": name,
		"Type": "sink",
	})
}

func writeTemplate(filepath string, tmpl string, data map[string]string) error {
	t, err := template.New("template").Parse(tmpl)
	if err != nil {
		return err
	}

	f, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	return t.Execute(f, data)
}
