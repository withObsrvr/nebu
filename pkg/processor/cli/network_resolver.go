package cli

import (
	"context"
	"fmt"
	"sync"

	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/processor"
)

// networkResolver determines which Stellar network a ledger actually came
// from, so the "network" field on every emitted event states a derived fact
// rather than repeating whatever --network happened to be set to.
//
// The distinction matters because a processor reading ledgers from stdin has
// no idea where the bytes originated. Before this existed, the pipeline
//
//	nebu fetch --network testnet 1 2 | token-transfer
//
// labelled its output "pubnet", because that is the processor's own flag
// default. A label that asserts provenance while reporting a flag is worse
// than no label: it is confidently wrong, and it becomes durable once written
// into a sink.
//
// # How the network is derived
//
// A transaction's hash commits to the network passphrase — it is
// SHA-256(networkID || envelopeType || tx), where networkID is itself the
// SHA-256 of the passphrase. The ledger stores that authoritative hash in
// each TransactionResultPair. So hashing any envelope in the ledger under a
// candidate passphrase and finding the result among the stored hashes
// identifies the network conclusively, with no signature verification and no
// network access.
//
// This is a membership test rather than a positional comparison, so it does
// not depend on envelopes and results being in the same order.
//
// Derivation needs at least one transaction. That is not a practical
// limitation here: a ledger with no transactions produces no events, so any
// ledger that has something to label also has something to fingerprint.
//
// The result is cached for the run. Passphrases do not change mid-stream, and
// hashing every ledger would be waste.
type networkResolver struct {
	configured string

	mu      sync.Mutex
	derived string
	done    bool
	warned  bool
}

func newNetworkResolver(configuredPassphrase string) *networkResolver {
	return &networkResolver{configured: configuredPassphrase}
}

// observe derives the network from ledger if it has not been derived yet, and
// warns once if the derived network disagrees with the configured one.
//
// It never fails the run. A labelling problem must not stop a pipeline that
// is otherwise producing correct data (N-4, "streams never throw"), so an
// underivable ledger simply leaves the resolver unresolved and the next
// ledger gets a turn.
func (r *networkResolver) observe(ctx context.Context, ledger xdr.LedgerCloseMeta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return
	}

	passphrase, ok := deriveNetworkPassphrase(ledger, r.candidates())
	if !ok {
		return
	}

	r.derived = passphrase
	r.done = true

	if passphrase != r.configured && !r.warned {
		r.warned = true
		processor.ReportWarning(ctx, "network-resolver", fmt.Errorf(
			"ledger data is from %s but --network is set to %s; events are "+
				"labelled with the derived network, but this processor is "+
				"decoding with the configured passphrase, so any value "+
				"derived from it (Stellar Asset Contract IDs in particular) "+
				"is wrong — pass --network %s",
			networkName(passphrase), networkName(r.configured), networkName(passphrase)))
	}
}

// candidates lists the passphrases worth testing, configured first because it
// is usually right and a match ends the search.
func (r *networkResolver) candidates() []string {
	candidates := []string{r.configured}
	for _, known := range []string{network.PublicNetworkPassphrase, network.TestNetworkPassphrase} {
		if known != r.configured {
			candidates = append(candidates, known)
		}
	}
	return candidates
}

// label returns the network name to write into an event envelope: the derived
// one once known, otherwise the configured one.
func (r *networkResolver) label() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return networkName(r.derived)
	}
	return networkName(r.configured)
}

// deriveNetworkPassphrase returns the first candidate passphrase under which a
// transaction in ledger hashes to a hash the ledger itself records, and
// whether any candidate matched.
func deriveNetworkPassphrase(ledger xdr.LedgerCloseMeta, candidates []string) (passphrase string, ok bool) {
	// The xdr.LedgerCloseMeta accessors panic on an unsupported union
	// version or transaction-set phase. Deriving a label is not worth
	// crashing a run over, so an unexpected shape degrades to "underivable".
	defer func() {
		if recover() != nil {
			passphrase, ok = "", false
		}
	}()

	count := ledger.CountTransactions()
	if count == 0 {
		return "", false
	}

	stored := make(map[xdr.Hash]struct{}, count)
	for i := 0; i < count; i++ {
		stored[ledger.TransactionHash(i)] = struct{}{}
	}
	if len(stored) == 0 {
		return "", false
	}

	envelopes := ledger.TransactionEnvelopes()
	if len(envelopes) == 0 {
		return "", false
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		hash, err := network.HashTransactionInEnvelope(envelopes[0], candidate)
		if err != nil {
			continue
		}
		if _, found := stored[xdr.Hash(hash)]; found {
			return candidate, true
		}
	}
	return "", false
}
