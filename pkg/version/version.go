// Package version provides version information for nebu.
package version

import "runtime/debug"

// Version is the current version of nebu.
//
// Set at build time via ldflags (see Makefile):
//
//	go build -ldflags "-X github.com/withObsrvr/nebu/pkg/version.Version=0.6.2"
//
// When built without ldflags (e.g., plain `go install
// github.com/withObsrvr/nebu/cmd/nebu@v0.6.2`), the init function below
// upgrades the default "dev" string to the module version recorded in
// the binary by the Go toolchain — so `nebu --version` shows a useful
// value regardless of how the binary was produced.
var Version = "dev"

func init() {
	if Version != "dev" {
		return // ldflags injected a concrete value; keep it
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		Version = v
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
