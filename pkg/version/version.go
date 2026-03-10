// Package version provides version information for nebu.
package version

// Version is the current version of nebu.
// This will be overridden at build time via ldflags:
//   go build -ldflags "-X github.com/withObsrvr/nebu/pkg/version.Version=0.4.1"
var Version = "dev"

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
