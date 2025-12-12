package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
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

			// Load registry
			reg, err := registry.LoadDefault()
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}

			proc, err := reg.FindProcessor(processorName)
			if err != nil {
				return fmt.Errorf("processor '%s' not found in registry", processorName)
			}

			// Determine install path
			if installPath == "" {
				installPath = getDefaultInstallPath()
			}

			// Build and install
			return installProcessor(proc.Name, proc.Location.Path, installPath)
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

func installProcessor(name, processorPath, installPath string) error {
	logInfo("Installing %s...", name)

	// Build the binary
	cmdPath := filepath.Join(processorPath, "cmd")
	binaryName := name

	logInfo("Building %s from %s...", name, cmdPath)

	buildCmd := exec.Command("go", "build", "-o", filepath.Join(installPath, binaryName), "./"+cmdPath)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build %s: %w", name, err)
	}

	installedPath := filepath.Join(installPath, binaryName)
	logInfo("Installed: %s", installedPath)
	logInfo("")
	logInfo("You can now run:")
	logInfo("  %s --help", binaryName)
	logInfo("  nebu fetch 60200000 60200100 | %s", binaryName)

	return nil
}
