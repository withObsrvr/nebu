package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/registry"
)

func newInstallCmd() *cobra.Command {
	var (
		installPath string
	)

	cmd := &cobra.Command{
		Use:   "install <processor-name>",
		Short: "Install a processor as a standalone binary",
		Long: `Install a processor as a standalone binary in your PATH.

This command builds the processor and installs it to a directory in your PATH
(default: $GOPATH/bin or $HOME/go/bin).

After installation, you can run the processor directly without the nebu prefix.

Examples:
  # Install token-transfer processor
  nebu install token-transfer

  # Install to custom location
  nebu install token-transfer --path /usr/local/bin

  # Use the installed processor
  token-transfer --start-ledger 60200000 --end-ledger 60200100
  nebu fetch 60200000 60200100 | token-transfer
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			processorName := args[0]

			// Load registry (embedded + external)
			reg, err := registry.LoadAll()
			if err != nil {
				return nebuErrors.RegistryLoadFailed(err)
			}

			proc, err := reg.FindProcessor(processorName)
			if err != nil {
				available := reg.ListProcessors("")
				var names []string
				for _, p := range available {
					names = append(names, p.Name)
				}
				return nebuErrors.ProcessorNotFound(processorName, names)
			}

			// Determine install path
			if installPath == "" {
				installPath = getDefaultInstallPath()
			}

			// Build and install
			return installProcessorSmart(proc, installPath)
		},
	}

	cmd.Flags().StringVar(&installPath, "path", "", "Installation directory (default: $GOPATH/bin)")

	return cmd
}

func getDefaultInstallPath() string {
	// Try GOPATH/bin first
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		return filepath.Join(gopath, "bin")
	}

	// Fall back to $HOME/go/bin
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, "go", "bin")
	}

	// Last resort
	return "/usr/local/bin"
}

// installProcessorSmart auto-detects whether to build locally or use go install
func installProcessorSmart(proc *registry.ProcessorEntry, installPath string) error {
	// Try local build first (dev mode with cloned repo)
	if proc.Location.Path != "" {
		if _, err := os.Stat(proc.Location.Path); err == nil {
			logInfo("Installing %s from local source...", proc.Name)
			return installProcessorLocal(proc.Name, proc.Location.Path, installPath)
		}
	}

	// Prebuilt binary release (language-agnostic; no Go toolchain needed)
	if proc.Install != nil {
		if proc.Install.Kind == "binary" {
			logInfo("Installing %s from prebuilt binary release...", proc.Name)
			return installProcessorBinary(proc.Name, proc.Install, installPath)
		}
		logInfo("Warning: unknown install kind %q for %s, falling back to go install", proc.Install.Kind, proc.Name)
	}

	// Fall back to go install (for users without cloned repo)
	if proc.Location.ModulePackage != "" {
		logInfo("Installing %s from Go module...", proc.Name)
		return installProcessorModule(proc.Name, proc.Location.ModulePackage, installPath)
	}

	return nebuErrors.WithSuggestion(
		fmt.Sprintf("Processor '%s' has no installable location", proc.Name),
		"The processor needs a local path, an install block, or a Go module package to install.",
	)
}

// installProcessorLocal builds a processor from local source.
// All processors must follow the cmd/processor-name/main.go structure.
func installProcessorLocal(name, processorPath, installPath string) error {
	// Build the binary from cmd/processor-name subdirectory
	cmdPath := filepath.Join(processorPath, "cmd", name)
	binaryName := name

	logInfo("Building %s from %s...", name, cmdPath)

	buildCmd := exec.Command("go", "build", "-o", filepath.Join(installPath, binaryName), "./"+cmdPath)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		return nebuErrors.BuildFailed(name, err)
	}

	installedPath := filepath.Join(installPath, binaryName)
	logInfo("Installed: %s", installedPath)
	logInfo("")
	logInfo("You can now run:")
	logInfo("  %s --help", binaryName)
	logInfo("  nebu fetch 60200000 60200100 | %s", binaryName)

	return nil
}

// installProcessorModule installs a processor using go install.
//
// We mirror `go install`'s stderr to both the live terminal and a
// buffer so (a) users watching interactively still see the compiler
// output as it streams, and (b) the buffer contents can be attached
// to the wrapped error for log-forwarded / CI contexts where the
// live stderr may be truncated or separated from the error message.
func installProcessorModule(name, modulePackage, installPath string) error {
	logInfo("Running: go install %s@latest", modulePackage)

	var stderrBuf bytes.Buffer
	cmd := exec.Command("go", "install", modulePackage+"@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if err := cmd.Run(); err != nil {
		return nebuErrors.InstallFailedWithOutput(name, err, stderrBuf.String())
	}

	installedPath := filepath.Join(installPath, name)
	logInfo("Installed: %s", installedPath)
	logInfo("")
	logInfo("You can now run:")
	logInfo("  %s --help", name)
	logInfo("  nebu fetch 60200000 60200100 | %s", name)

	return nil
}
