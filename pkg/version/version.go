// Package version provides version information for nebu.
package version

import "runtime/debug"

// Version is the current version of nebu.
//
// Set at build time via ldflags (see Makefile):
//
//	go build -ldflags "-X github.com/withObsrvr/nebu/pkg/version.Version=0.6.3"
//
// When built without ldflags (e.g., plain `go install
// github.com/withObsrvr/nebu/cmd/nebu@v0.6.3`), the init function below
// upgrades the default "dev" string to the module version recorded in
// the binary by the Go toolchain — so `nebu --version` shows a useful
// value regardless of how the binary was produced.
var Version = "dev"

// nebuModulePath is the canonical Go module path for the nebu module.
// It's used in init to pick the correct version out of runtime build
// info when this package is compiled into a binary whose main module
// is NOT nebu (e.g., a processor submodule under examples/processors).
const nebuModulePath = "github.com/withObsrvr/nebu"

func init() {
	if Version != "dev" {
		return // ldflags injected a concrete value; keep it
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	// Case 1: compiled into the nebu main module itself (e.g., the
	// nebu CLI at cmd/nebu). info.Main.Version is the nebu version.
	if info.Main.Path == nebuModulePath {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			Version = v
		}
		return
	}

	// Case 2: compiled into a submodule (e.g., a processor binary
	// like token-transfer under examples/processors/). info.Main
	// holds the submodule's own pseudo-version, which is not what
	// we want — consumers of version.Version (including the
	// _nebu_version field every origin CLI helper writes into the
	// event envelope) expect the nebu release version. Scan Deps
	// for the actual nebu module entry.
	for _, dep := range info.Deps {
		if dep.Path == nebuModulePath {
			if v := dep.Version; v != "" && v != "(devel)" {
				Version = v
			}
			return
		}
	}
}

// SchemaVersions defines the current schema versions for each processor type.
var SchemaVersions = map[string]string{
	"token_transfer": "v1",
}

// GetSchemaVersion returns the schema version identifier for a processor.
// Format: nebu.<processor>.<version>
func GetSchemaVersion(processorName string) string {
	if version, ok := SchemaVersions[processorName]; ok {
		return "nebu." + processorName + "." + version
	}
	// Default for unknown processors
	return "nebu." + processorName + ".v1"
}
