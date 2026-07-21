package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/withObsrvr/nebu/pkg/registry"
)

func TestExpandInstallTemplate(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		version string
		goos    string
		goarch  string
		want    string
	}{
		{
			name:    "all placeholders",
			tmpl:    "https://example.com/releases/v{version}/proc-{os}-{arch}",
			version: "1.2.3",
			goos:    "linux",
			goarch:  "amd64",
			want:    "https://example.com/releases/v1.2.3/proc-linux-amd64",
		},
		{
			name:    "no placeholders passes through",
			tmpl:    "https://example.com/static/proc",
			version: "1.0.0",
			goos:    "darwin",
			goarch:  "arm64",
			want:    "https://example.com/static/proc",
		},
		{
			name:    "repeated placeholder",
			tmpl:    "https://example.com/{version}/{version}/checksums.txt",
			version: "0.1.0",
			goos:    "linux",
			goarch:  "arm64",
			want:    "https://example.com/0.1.0/0.1.0/checksums.txt",
		},
		{
			name:    "exe expands on windows",
			tmpl:    "https://example.com/v{version}/proc-{os}-{arch}{exe}",
			version: "1.0.0",
			goos:    "windows",
			goarch:  "amd64",
			want:    "https://example.com/v1.0.0/proc-windows-amd64.exe",
		},
		{
			name:    "exe empty elsewhere",
			tmpl:    "https://example.com/v{version}/proc-{os}-{arch}{exe}",
			version: "1.0.0",
			goos:    "linux",
			goarch:  "amd64",
			want:    "https://example.com/v1.0.0/proc-linux-amd64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandInstallTemplate(tt.tmpl, tt.version, tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("expandInstallTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerifySHA256(t *testing.T) {
	data := []byte("processor binary bytes")
	sum := sha256.Sum256(data)
	goodHex := hex.EncodeToString(sum[:])

	tests := []struct {
		name      string
		artifact  string
		checksums string
		wantErr   string // substring; empty means success
	}{
		{
			name:      "matching entry",
			artifact:  "proc-linux-amd64",
			checksums: goodHex + "  proc-linux-amd64\n",
		},
		{
			name:      "matching entry among others",
			artifact:  "proc-linux-amd64",
			checksums: strings.Repeat("0", 64) + "  proc-darwin-arm64\n" + goodHex + "  proc-linux-amd64\n",
		},
		{
			name:      "binary-mode asterisk prefix",
			artifact:  "proc-linux-amd64",
			checksums: goodHex + " *proc-linux-amd64\n",
		},
		{
			name:      "uppercase hex accepted",
			artifact:  "proc-linux-amd64",
			checksums: strings.ToUpper(goodHex) + "  proc-linux-amd64\n",
		},
		{
			name:      "mismatched digest",
			artifact:  "proc-linux-amd64",
			checksums: strings.Repeat("a", 64) + "  proc-linux-amd64\n",
			wantErr:   "sha256 mismatch",
		},
		{
			name:      "no entry for artifact",
			artifact:  "proc-linux-amd64",
			checksums: goodHex + "  proc-windows-amd64.exe\n",
			wantErr:   "no checksum entry",
		},
		{
			name:      "empty checksums file",
			artifact:  "proc-linux-amd64",
			checksums: "",
			wantErr:   "no checksum entry",
		},
		{
			name:      "malformed lines skipped",
			artifact:  "proc-linux-amd64",
			checksums: "not a checksum line\n\n" + goodHex + "  proc-linux-amd64\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifySHA256(tt.artifact, data, tt.checksums)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("verifySHA256() unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("verifySHA256() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestInstallProcessorBinary exercises the full download-verify-install-check
// flow against a local HTTP server. The artifact is a shell script that
// implements just enough of the contract (--describe-json) to pass the
// post-install check.
func TestInstallProcessorBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test artifact is a shell script")
	}

	script := "#!/bin/sh\necho '{\"name\":\"test-proc\",\"type\":\"sink\",\"version\":\"0.1.0\"}'\n"
	sum := sha256.Sum256([]byte(script))
	artifactName := fmt.Sprintf("test-proc-%s-%s", runtime.GOOS, runtime.GOARCH)
	goodChecksums := hex.EncodeToString(sum[:]) + "  " + artifactName + "\n"

	tests := []struct {
		name      string
		checksums string // served checksums body; empty string means 404
		cfg       func(baseURL string) *registry.InstallConfig
		wantErr   string
	}{
		{
			name:      "successful install",
			checksums: goodChecksums,
			cfg: func(base string) *registry.InstallConfig {
				return &registry.InstallConfig{
					Kind:      "binary",
					URL:       base + "/v{version}/test-proc-{os}-{arch}",
					Checksums: base + "/v{version}/checksums.txt",
					Version:   "0.1.0",
				}
			},
		},
		{
			name:      "checksum mismatch refuses install",
			checksums: strings.Repeat("b", 64) + "  " + artifactName + "\n",
			cfg: func(base string) *registry.InstallConfig {
				return &registry.InstallConfig{
					Kind:      "binary",
					URL:       base + "/v{version}/test-proc-{os}-{arch}",
					Checksums: base + "/v{version}/checksums.txt",
					Version:   "0.1.0",
				}
			},
			wantErr: "Checksum verification failed",
		},
		{
			name:      "missing checksums file refuses install",
			checksums: "",
			cfg: func(base string) *registry.InstallConfig {
				return &registry.InstallConfig{
					Kind:      "binary",
					URL:       base + "/v{version}/test-proc-{os}-{arch}",
					Checksums: base + "/v{version}/checksums.txt",
					Version:   "0.1.0",
				}
			},
			wantErr: "Failed to download checksums",
		},
		{
			name:      "config without checksums url refuses install",
			checksums: goodChecksums,
			cfg: func(base string) *registry.InstallConfig {
				return &registry.InstallConfig{
					Kind:    "binary",
					URL:     base + "/v{version}/test-proc-{os}-{arch}",
					Version: "0.1.0",
				}
			},
			wantErr: "without checksums",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v0.1.0/" + artifactName:
					fmt.Fprint(w, script)
				case "/v0.1.0/checksums.txt":
					if tt.checksums == "" {
						http.NotFound(w, r)
						return
					}
					fmt.Fprint(w, tt.checksums)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			installPath := t.TempDir()
			err := installProcessorBinary("test-proc", tt.cfg(srv.URL), installPath)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("installProcessorBinary() error = %v, want containing %q", err, tt.wantErr)
				}
				if _, statErr := os.Stat(filepath.Join(installPath, "test-proc")); statErr == nil {
					t.Error("binary was installed despite failed verification")
				}
				return
			}

			if err != nil {
				t.Fatalf("installProcessorBinary() unexpected error: %v", err)
			}
			installed := filepath.Join(installPath, "test-proc")
			info, statErr := os.Stat(installed)
			if statErr != nil {
				t.Fatalf("installed binary missing: %v", statErr)
			}
			if info.Mode()&0o111 == 0 {
				t.Errorf("installed binary is not executable: mode %v", info.Mode())
			}
		})
	}
}
