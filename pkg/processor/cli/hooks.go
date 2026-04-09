package cli

import (
	"github.com/withObsrvr/nebu/pkg/runtime"
)

// attachHooks installs each Hooks bundle on the given runtime in the
// order it appears. This is the shared bridge between the per-config
// Hooks slice declared by processor authors and the runtime's
// internal Use mechanism.
//
// Used by RunOriginCLI, RunGenericOriginCLI, and RunProtoOriginCLI
// just before they call rt.RunOrigin. Transforms and sinks do not
// have a hook surface in v0.6.x — their stdin/stdout loops don't
// own a runtime instance.
func attachHooks(rt *runtime.Runtime, hooks []runtime.Hooks) {
	for _, h := range hooks {
		rt.Use(h)
	}
}
