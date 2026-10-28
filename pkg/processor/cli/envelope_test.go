package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/version"
)

func TestNetworkName(t *testing.T) {
	tests := []struct {
		name       string
		passphrase string
		want       string
	}{
		{name: "public passphrase", passphrase: network.PublicNetworkPassphrase, want: "pubnet"},
		{name: "test passphrase", passphrase: network.TestNetworkPassphrase, want: "testnet"},
		{
			// Never silently mislabel an unknown network as a known one.
			name:       "unknown passphrase is returned verbatim",
			passphrase: "Standalone Network ; February 2017",
			want:       "Standalone Network ; February 2017",
		},
		{name: "empty passphrase", passphrase: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, networkName(tt.passphrase))
		})
	}
}

func TestNetworkNameAcceptsNormalizedAliases(t *testing.T) {
	// The CLI normalizes aliases to full passphrases before the envelope is
	// written, so networkName only ever sees passphrases. Assert the two
	// stages compose.
	for _, alias := range []string{"mainnet", "pubnet", "public"} {
		assert.Equal(t, "pubnet", networkName(normalizeNetworkPassphrase(alias)), alias)
	}
	for _, alias := range []string{"testnet", "test"} {
		assert.Equal(t, "testnet", networkName(normalizeNetworkPassphrase(alias)), alias)
	}
}

// protoEnvelopeLine reproduces the envelope splice in RunProtoOriginCLI so the
// JSON it produces can be asserted without standing up a processor.
func protoEnvelopeLine(schemaID, netName string, payload []byte) string {
	prefix := fmt.Sprintf(`{"_schema":%q,"_nebu_version":%q,"network":%q`,
		schemaID, version.Version, netName)
	if len(payload) > 2 {
		return prefix + "," + string(payload[1:])
	}
	return prefix + "}"
}

func TestProtoEnvelopeIsValidJSON(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "payload with fields", payload: `{"meta":{"ledgerSequence":60200000}}`},
		// Regression: the previous splice emitted
		// {"_schema":"…","_nebu_version":"…",} for an empty payload — a
		// trailing comma, so the line was not parseable JSON at all.
		{name: "empty payload", payload: `{}`},
		{name: "payload with one field", payload: `{"a":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := protoEnvelopeLine("nebu.token_transfer.v1", "pubnet", []byte(tt.payload))

			var got map[string]any
			require.NoError(t, json.Unmarshal([]byte(line), &got),
				"envelope must be valid JSON, got: %s", line)
			assert.Equal(t, "nebu.token_transfer.v1", got["_schema"])
			assert.Equal(t, "pubnet", got["network"])
			assert.Equal(t, version.Version, got["_nebu_version"])
		})
	}
}

func TestProtoEnvelopeEscapesInterpolatedValues(t *testing.T) {
	// These values land inside a JSON document via Sprintf, so a quote or
	// backslash in any of them must not break the line.
	line := protoEnvelopeLine(`weird"schema\id`, `net"work`, []byte(`{"a":1}`))

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &got), "got: %s", line)
	assert.Equal(t, `weird"schema\id`, got["_schema"])
	assert.Equal(t, `net"work`, got["network"])
}

func TestProtoEnvelopePreservesPayloadFieldOrder(t *testing.T) {
	// The splice exists so protojson's field order survives; a map
	// round-trip would sort the keys alphabetically instead.
	payload := `{"zebra":1,"alpha":2}`
	line := protoEnvelopeLine("s", "pubnet", []byte(payload))
	assert.Contains(t, line, `"zebra":1,"alpha":2`)
}

// --- network derivation -----------------------------------------------------

// ledgerWithTx builds a LedgerCloseMeta containing one transaction whose
// recorded hash is computed under hashPassphrase. Deriving the network from it
// must recover hashPassphrase regardless of what was configured.
func ledgerWithTx(t *testing.T, hashPassphrase string) xdr.LedgerCloseMeta {
	t.Helper()

	source := xdr.MustAddress("GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF")
	envelope := xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTx,
		V1: &xdr.TransactionV1Envelope{
			Tx: xdr.Transaction{
				SourceAccount: source.ToMuxedAccount(),
				Fee:           100,
				SeqNum:        1,
				Operations:    []xdr.Operation{},
			},
		},
	}
	hash, err := network.HashTransactionInEnvelope(envelope, hashPassphrase)
	require.NoError(t, err)

	return xdr.LedgerCloseMeta{
		V: 0,
		V0: &xdr.LedgerCloseMetaV0{
			LedgerHeader: xdr.LedgerHeaderHistoryEntry{
				Header: xdr.LedgerHeader{LedgerSeq: 5429344},
			},
			TxSet: xdr.TransactionSet{Txs: []xdr.TransactionEnvelope{envelope}},
			TxProcessing: []xdr.TransactionResultMeta{{
				Result: xdr.TransactionResultPair{TransactionHash: xdr.Hash(hash)},
			}},
		},
	}
}

func TestDeriveNetworkPassphrase(t *testing.T) {
	candidates := []string{network.PublicNetworkPassphrase, network.TestNetworkPassphrase}

	for _, want := range candidates {
		got, ok := deriveNetworkPassphrase(ledgerWithTx(t, want), candidates)
		require.True(t, ok, "should derive %s", want)
		assert.Equal(t, want, got)
	}
}

func TestDeriveNetworkPassphraseUndecidable(t *testing.T) {
	// A ledger with no transactions carries no hash to fingerprint. It also
	// produces no events, so there is nothing to mislabel.
	empty := xdr.LedgerCloseMeta{V: 0, V0: &xdr.LedgerCloseMetaV0{
		LedgerHeader: xdr.LedgerHeaderHistoryEntry{Header: xdr.LedgerHeader{LedgerSeq: 1}},
	}}
	_, ok := deriveNetworkPassphrase(empty, []string{network.PublicNetworkPassphrase})
	assert.False(t, ok)

	// A network outside the candidate list must not be guessed at.
	_, ok = deriveNetworkPassphrase(
		ledgerWithTx(t, "Standalone Network ; February 2017"),
		[]string{network.PublicNetworkPassphrase, network.TestNetworkPassphrase})
	assert.False(t, ok, "must not match a passphrase it was not given")
}

// TestResolverLabelFollowsDataNotFlag is the bug this exists to prevent:
// `nebu fetch --network testnet | token-transfer` used to label its output
// "pubnet", because that is the processor's flag default.
func TestResolverLabelFollowsDataNotFlag(t *testing.T) {
	resolver := newNetworkResolver(network.PublicNetworkPassphrase) // flag says pubnet
	assert.Equal(t, "pubnet", resolver.label(), "before any ledger, the flag is all there is")

	resolver.observe(context.Background(), ledgerWithTx(t, network.TestNetworkPassphrase))

	assert.Equal(t, "testnet", resolver.label(), "label must follow the data, not the flag")
}

func TestResolverWarnsOnMismatchOnce(t *testing.T) {
	var reports []string
	ctx := processor.WithReporter(context.Background(),
		processor.ReporterFunc(func(report processor.ErrorReport) {
			reports = append(reports, report.Err.Error())
		}))

	resolver := newNetworkResolver(network.PublicNetworkPassphrase)
	ledger := ledgerWithTx(t, network.TestNetworkPassphrase)
	resolver.observe(ctx, ledger)
	resolver.observe(ctx, ledger)
	resolver.observe(ctx, ledger)

	require.Len(t, reports, 1, "a mismatch must be reported once, not per ledger")
	assert.Contains(t, reports[0], "testnet")
	assert.Contains(t, reports[0], "pubnet")
}

func TestResolverSilentWhenFlagMatchesData(t *testing.T) {
	var reports []string
	ctx := processor.WithReporter(context.Background(),
		processor.ReporterFunc(func(report processor.ErrorReport) {
			reports = append(reports, report.Err.Error())
		}))

	resolver := newNetworkResolver(network.TestNetworkPassphrase)
	resolver.observe(ctx, ledgerWithTx(t, network.TestNetworkPassphrase))

	assert.Empty(t, reports)
	assert.Equal(t, "testnet", resolver.label())
}

func TestResolverCachesFirstDerivation(t *testing.T) {
	resolver := newNetworkResolver(network.PublicNetworkPassphrase)
	resolver.observe(context.Background(), ledgerWithTx(t, network.TestNetworkPassphrase))
	// A later ledger cannot flip the label; passphrases do not change
	// mid-stream, and re-deriving every ledger would be waste.
	resolver.observe(context.Background(), ledgerWithTx(t, network.PublicNetworkPassphrase))
	assert.Equal(t, "testnet", resolver.label())
}

func TestResolverConcurrentObserveAndLabel(t *testing.T) {
	// observe runs on the ledger path, label on the event-writing goroutine.
	resolver := newNetworkResolver(network.PublicNetworkPassphrase)
	ledger := ledgerWithTx(t, network.TestNetworkPassphrase)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); resolver.observe(context.Background(), ledger) }()
		go func() { defer wg.Done(); _ = resolver.label() }()
	}
	wg.Wait()
	assert.Equal(t, "testnet", resolver.label())
}

func TestResolverNilIsSafe(t *testing.T) {
	var resolver *networkResolver
	assert.Equal(t, "", resolver.label())
}
