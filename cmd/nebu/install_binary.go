package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	nebuErrors "github.com/withObsrvr/nebu/pkg/errors"
	"github.com/withObsrvr/nebu/pkg/registry"
)

// binaryHTTPClient downloads release artifacts. Prebuilt processor binaries
// can be tens of megabytes, so the timeout is generous.
var binaryHTTPClient = &http.Client{Timeout: 5 * time.Minute}

// expandInstallTemplate substitutes {version}, {os}, {arch}, and {exe} in
// an install URL template. os/arch use Go's GOOS/GOARCH naming, which is
// what the registry spec requires release artifacts to be named after;
// {exe} expands to ".exe" on windows and nothing elsewhere.
func expandInstallTemplate(tmpl, version, goos, goarch string) string {
	exe := ""
	if goos == "windows" {
		exe = ".exe"
	}
	r := strings.NewReplacer(
		"{version}", version,
		"{os}", goos,
		"{arch}", goarch,
		"{exe}", exe,
	)
	return r.Replace(tmpl)
}

// installProcessorBinary downloads a prebuilt processor binary, verifies it
// against the release's sha256 checksums file, installs it into installPath,
// and sanity-checks it by invoking --describe-json.
func installProcessorBinary(name string, cfg *registry.InstallConfig, installPath string) error {
	if cfg.URL == "" {
		return nebuErrors.WithSuggestion(
			fmt.Sprintf("Processor '%s' has an install block without a url", name),
			"The registry entry's install block must set url for kind: binary.",
		)
	}
	if cfg.Checksums == "" {
		return nebuErrors.WithSuggestion(
			fmt.Sprintf("Processor '%s' has a binary install block without checksums", name),
			"Binary installs require a sha256 checksums file. Ask the processor maintainer to publish one alongside the release artifacts.",
		)
	}

	artifactURL := expandInstallTemplate(cfg.URL, cfg.Version, runtime.GOOS, runtime.GOARCH)
	checksumsURL := expandInstallTemplate(cfg.Checksums, cfg.Version, runtime.GOOS, runtime.GOARCH)

	logInfo("Downloading %s", artifactURL)
	artifact, err := httpGetAll(artifactURL)
	if err != nil {
		return nebuErrors.WithSuggestion(
			fmt.Sprintf("Failed to download binary for '%s': %v", name, err),
			fmt.Sprintf("The release may not publish a %s/%s build. Check the processor's releases page.", runtime.GOOS, runtime.GOARCH),
		)
	}

	checksums, err := httpGetAll(checksumsURL)
	if err != nil {
		return nebuErrors.WithSuggestion(
			fmt.Sprintf("Failed to download checksums for '%s': %v", name, err),
			"Refusing to install an unverified binary. Check the processor's releases page for a checksums.txt.",
		)
	}

	if err := verifySHA256(path.Base(artifactURL), artifact, string(checksums)); err != nil {
		return nebuErrors.WithSuggestion(
			fmt.Sprintf("Checksum verification failed for '%s': %v", name, err),
			"The downloaded artifact does not match the published sha256. Do not run it; report this to the processor maintainer.",
		)
	}

	if err := os.MkdirAll(installPath, 0o755); err != nil {
		return nebuErrors.InstallFailed(name, err)
	}

	binaryName := name
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	installedPath := filepath.Join(installPath, binaryName)

	// Write to a temp file in the target dir, then rename into place so a
	// concurrent invocation never sees a half-written binary.
	tmp, err := os.CreateTemp(installPath, "."+binaryName+".*")
	if err != nil {
		return nebuErrors.InstallFailed(name, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename
	if _, err := tmp.Write(artifact); err != nil {
		_ = tmp.Close() // write error is the failure being reported
		return nebuErrors.InstallFailed(name, err)
	}
	if err := tmp.Close(); err != nil {
		return nebuErrors.InstallFailed(name, err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return nebuErrors.InstallFailed(name, err)
	}
	if err := os.Rename(tmpName, installedPath); err != nil {
		return nebuErrors.InstallFailed(name, err)
	}

	if err := verifyInstalledBinary(installedPath, name); err != nil {
		return err
	}

	logInfo("Installed: %s (sha256 verified)", installedPath)
	logInfo("")
	logInfo("You can now run:")
	logInfo("  %s --help", name)
	logInfo("  nebu fetch 60200000 60200100 | %s", name)

	return nil
}

func httpGetAll(url string) ([]byte, error) {
	resp, err := binaryHTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// verifySHA256 checks data against a sha256sum-format checksums file
// (lines of "<hex>  <filename>"). The artifact is matched by base name.
func verifySHA256(artifactName string, data []byte, checksums string) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])

	for line := range strings.SplitSeq(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		want, fname := fields[0], fields[1]
		// sha256sum prefixes binary-mode filenames with '*'
		if strings.TrimPrefix(path.Base(fname), "*") != artifactName {
			continue
		}
		if !strings.EqualFold(want, got) {
			return fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", artifactName, want, got)
		}
		return nil
	}
	return fmt.Errorf("no checksum entry found for %s", artifactName)
}

// verifyInstalledBinary runs `<binary> --describe-json` as a post-install
// sanity check: the contract requires it to exit 0 and print a JSON
// envelope. A name mismatch is a warning, not a failure — registries may
// alias processors.
func verifyInstalledBinary(installedPath, name string) error {
	out, err := exec.Command(installedPath, "--describe-json").Output()
	if err != nil {
		return nebuErrors.WithSuggestion(
			fmt.Sprintf("Installed binary for '%s' failed the --describe-json check: %v", name, err),
			"Every nebu processor must support --describe-json (see docs/PROCESSOR_CONTRACT.md). The downloaded artifact may be corrupt or not a nebu processor.",
		)
	}
	var envelope struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		return nebuErrors.WithSuggestion(
			fmt.Sprintf("Installed binary for '%s' printed invalid --describe-json output: %v", name, err),
			"The describe envelope must be a single JSON document on stdout (see docs/PROCESSOR_CONTRACT.md).",
		)
	}
	if envelope.Name != name {
		logInfo("Warning: binary describes itself as %q, registry entry is %q", envelope.Name, name)
	}
	return nil
}
