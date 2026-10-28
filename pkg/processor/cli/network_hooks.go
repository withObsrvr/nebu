package cli

import (
	"context"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/runtime"
)

// withNetworkResolverHook lets the resolver see ledgers that reach the origin
// through the runtime rather than through nebu's own decode loop — the RPC and
// archive paths. Without it, only stdin and file input would get a derived
// network label.
func withNetworkResolverHook(hooks []runtime.Hooks, resolver *networkResolver) []runtime.Hooks {
	if resolver == nil {
		return hooks
	}
	resolverHook := runtime.Hooks{
		BeforeLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
			resolver.observe(ctx, ledger)
			return nil
		},
	}
	return append(hooks, resolverHook)
}
