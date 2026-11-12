package ledgerread

import (
	"context"
	"testing"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/withObsrvr/nebu/pkg/processor"
)

const testPassphrase = network.TestNetworkPassphrase

// capture attaches a reporter that records every report, so a test can assert
// on what the loop said rather than only on what it yielded.
func capture(t *testing.T) (context.Context, *[]processor.ErrorReport) {
	t.Helper()

	reports := &[]processor.ErrorReport{}
	ctx := processor.WithReporter(context.Background(),
		processor.ReporterFunc(func(report processor.ErrorReport) {
			*reports = append(*reports, report)
		}))
	return ctx, reports
}

func testEnvelope(t *testing.T, seqNum xdr.SequenceNumber) xdr.TransactionEnvelope {
	t.Helper()

	source := xdr.MustAddress("GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF")
	return xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTx,
		V1: &xdr.TransactionV1Envelope{
			Tx: xdr.Transaction{
				SourceAccount: source.ToMuxedAccount(),
				Fee:           100,
				SeqNum:        seqNum,
				Operations:    []xdr.Operation{},
			},
		},
	}
}

func txHash(t *testing.T, envelope xdr.TransactionEnvelope) xdr.Hash {
	t.Helper()

	hash, err := network.HashTransactionInEnvelope(envelope, testPassphrase)
	require.NoError(t, err)
	return xdr.Hash(hash)
}

// ledger builds a LedgerCloseMeta from envelopes paired with the hash each is
// recorded under. Passing a hash that does not match its envelope is how a
// test reproduces an unreadable transaction: the reader cannot associate the
// recorded hash with any envelope in the set.
func ledger(sequence uint32, envelopes []xdr.TransactionEnvelope, hashes []xdr.Hash) xdr.LedgerCloseMeta {
	processing := make([]xdr.TransactionResultMeta, 0, len(hashes))
	for _, hash := range hashes {
		processing = append(processing, xdr.TransactionResultMeta{
			Result: xdr.TransactionResultPair{TransactionHash: hash},
		})
	}

	return xdr.LedgerCloseMeta{
		V: 0,
		V0: &xdr.LedgerCloseMetaV0{
			LedgerHeader: xdr.LedgerHeaderHistoryEntry{
				Header: xdr.LedgerHeader{LedgerSeq: xdr.Uint32(sequence)},
			},
			TxSet:        xdr.TransactionSet{Txs: envelopes},
			TxProcessing: processing,
		},
	}
}

func TestTransactionsYieldsEveryTransaction(t *testing.T) {
	first := testEnvelope(t, 1)
	second := testEnvelope(t, 2)
	lcm := ledger(100,
		[]xdr.TransactionEnvelope{first, second},
		[]xdr.Hash{txHash(t, first), txHash(t, second)})

	ctx, reports := capture(t)

	var got []int64
	for tx := range Transactions(ctx, "test", testPassphrase, lcm) {
		got = append(got, tx.Envelope.SeqNum())
	}

	assert.Equal(t, []int64{1, 2}, got)
	assert.Empty(t, *reports, "a clean ledger must report nothing")
}

func TestTransactionsEmptyLedgerReportsNothing(t *testing.T) {
	ctx, reports := capture(t)

	count := 0
	for range Transactions(ctx, "test", testPassphrase, ledger(100, nil, nil)) {
		count++
	}

	// End of ledger is not a failure. Reporting io.EOF as a warning would
	// make every clean run noisy.
	assert.Zero(t, count)
	assert.Empty(t, *reports)
}

func TestTransactionsReportsUnreadableTransactionAndContinues(t *testing.T) {
	bad := testEnvelope(t, 1)
	good := testEnvelope(t, 2)

	// The first recorded hash belongs to no envelope in the set, so reading
	// it fails. This is the case the old `if err != nil { break }` loops
	// treated as end-of-ledger: no warning, and every later transaction
	// dropped.
	lcm := ledger(100,
		[]xdr.TransactionEnvelope{bad, good},
		[]xdr.Hash{{0xde, 0xad, 0xbe, 0xef}, txHash(t, good)})

	ctx, reports := capture(t)

	var got []int64
	for tx := range Transactions(ctx, "test", testPassphrase, lcm) {
		got = append(got, tx.Envelope.SeqNum())
	}

	assert.Equal(t, []int64{2}, got, "the readable transaction must survive")
	require.Len(t, *reports, 1)
	report := (*reports)[0]
	assert.Equal(t, processor.SeverityWarning, report.Severity)
	assert.Equal(t, "test", report.Processor)
	assert.Contains(t, report.Err.Error(), "ledger 100")
	assert.Contains(t, report.Err.Error(), "read transaction")
}

func TestTransactionsStopsOnCancelledContext(t *testing.T) {
	first := testEnvelope(t, 1)
	lcm := ledger(100, []xdr.TransactionEnvelope{first}, []xdr.Hash{txHash(t, first)})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	count := 0
	for range Transactions(ctx, "test", testPassphrase, lcm) {
		count++
	}

	assert.Zero(t, count, "a cancelled context must yield nothing")
}

func TestTransactionsHonoursEarlyBreak(t *testing.T) {
	first := testEnvelope(t, 1)
	second := testEnvelope(t, 2)
	lcm := ledger(100,
		[]xdr.TransactionEnvelope{first, second},
		[]xdr.Hash{txHash(t, first), txHash(t, second)})

	ctx, reports := capture(t)

	count := 0
	for range Transactions(ctx, "test", testPassphrase, lcm) {
		count++
		break
	}

	assert.Equal(t, 1, count)
	assert.Empty(t, *reports, "closing after an early break must not warn")
}

func TestChangesReportsOpenFailureWithoutYielding(t *testing.T) {
	// A protocol version below 10 with fee processing present is the meta
	// version the change reader rejects outright.
	lcm := xdr.LedgerCloseMeta{
		V: 0,
		V0: &xdr.LedgerCloseMetaV0{
			LedgerHeader: xdr.LedgerHeaderHistoryEntry{
				Header: xdr.LedgerHeader{LedgerSeq: 100, LedgerVersion: 9},
			},
			TxSet: xdr.TransactionSet{Txs: []xdr.TransactionEnvelope{testEnvelope(t, 1)}},
			TxProcessing: []xdr.TransactionResultMeta{{
				Result:        xdr.TransactionResultPair{TransactionHash: txHash(t, testEnvelope(t, 1))},
				FeeProcessing: xdr.LedgerEntryChanges{{Type: xdr.LedgerEntryChangeTypeLedgerEntryState}},
				TxApplyProcessing: xdr.TransactionMeta{
					V: 1, V1: &xdr.TransactionMetaV1{},
				},
			}},
		},
	}

	ctx, reports := capture(t)

	count := 0
	for range Changes(ctx, "test", testPassphrase, lcm) {
		count++
	}

	assert.Zero(t, count)
	require.Len(t, *reports, 1)
	assert.Equal(t, processor.SeverityWarning, (*reports)[0].Severity)
	assert.Contains(t, (*reports)[0].Err.Error(), "open change reader")
}

func TestChangesYieldsUpgradeChanges(t *testing.T) {
	lcm := ledger(100, nil, nil)
	lcm.V0.UpgradesProcessing = []xdr.UpgradeEntryMeta{{
		// A State entry is only the "pre" half of a change and emits
		// nothing on its own; Created is the simplest arm that yields one.
		Changes: xdr.LedgerEntryChanges{{
			Type: xdr.LedgerEntryChangeTypeLedgerEntryCreated,
			Created: &xdr.LedgerEntry{
				Data: xdr.LedgerEntryData{
					Type: xdr.LedgerEntryTypeAccount,
					Account: &xdr.AccountEntry{
						AccountId: xdr.MustAddress("GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF"),
					},
				},
			},
		}},
	}}

	ctx, reports := capture(t)

	var got []ingest.Change
	for change := range Changes(ctx, "test", testPassphrase, lcm) {
		got = append(got, change)
	}

	require.Len(t, got, 1)
	assert.Equal(t, ingest.LedgerEntryChangeReasonUpgrade, got[0].Reason)
	assert.Empty(t, *reports)
}

func TestAllTransactionsReportsACompleteRead(t *testing.T) {
	first := testEnvelope(t, 1)
	second := testEnvelope(t, 2)
	lcm := ledger(100,
		[]xdr.TransactionEnvelope{first, second},
		[]xdr.Hash{txHash(t, first), txHash(t, second)})

	ctx, reports := capture(t)

	txs, ok := AllTransactions(ctx, "test", testPassphrase, lcm)

	assert.True(t, ok)
	require.Len(t, txs, 2)
	assert.Equal(t, int64(1), txs[0].Envelope.SeqNum())
	assert.Equal(t, int64(2), txs[1].Envelope.SeqNum())
	assert.Empty(t, *reports)
}

func TestAllTransactionsEmptyLedgerIsComplete(t *testing.T) {
	// No transactions is a complete read of a ledger that had none. Reporting
	// it as incomplete would make every empty ledger look like a failure.
	ctx, reports := capture(t)

	txs, ok := AllTransactions(ctx, "test", testPassphrase, ledger(100, nil, nil))

	assert.True(t, ok)
	assert.Empty(t, txs)
	assert.Empty(t, *reports)
}

func TestAllTransactionsReportsAnIncompleteRead(t *testing.T) {
	// The distinction from Transactions. An index built from a partial read
	// misses at lookup time, and a miss usually falls through to a default,
	// so the caller has to be able to tell that it cannot trust the result.
	good := testEnvelope(t, 1)
	bad := testEnvelope(t, 2)
	lcm := ledger(100,
		[]xdr.TransactionEnvelope{good, bad},
		[]xdr.Hash{txHash(t, good), {0xde, 0xad, 0xbe, 0xef}})

	ctx, reports := capture(t)

	txs, ok := AllTransactions(ctx, "test", testPassphrase, lcm)

	assert.False(t, ok, "a ledger that could not be read to the end is not complete")
	// What was read is still returned, for a caller that wants it.
	require.Len(t, txs, 1)
	assert.Equal(t, int64(1), txs[0].Envelope.SeqNum())

	require.Len(t, *reports, 1)
	assert.Equal(t, processor.SeverityWarning, (*reports)[0].Severity)
	assert.Contains(t, (*reports)[0].Err.Error(), "ledger 100")
}

func TestAllTransactionsDiffersFromTransactionsOnAPartialLedger(t *testing.T) {
	// Pins the contract that the two functions exist for: given the same
	// unreadable ledger, Transactions carries on past the bad transaction
	// and AllTransactions refuses to call the result complete.
	good := testEnvelope(t, 1)
	alsoGood := testEnvelope(t, 3)
	lcm := ledger(100,
		[]xdr.TransactionEnvelope{good, alsoGood},
		[]xdr.Hash{{0xde, 0xad, 0xbe, 0xef}, txHash(t, alsoGood)})

	lenientCtx, _ := capture(t)
	var lenient []int64
	for tx := range Transactions(lenientCtx, "test", testPassphrase, lcm) {
		lenient = append(lenient, tx.Envelope.SeqNum())
	}
	assert.Equal(t, []int64{3}, lenient, "Transactions skips the bad one and continues")

	strictCtx, _ := capture(t)
	_, ok := AllTransactions(strictCtx, "test", testPassphrase, lcm)
	assert.False(t, ok, "AllTransactions will not claim a partial read is complete")
}

func TestAllTransactionsTreatsCancellationAsIncomplete(t *testing.T) {
	// A shutdown must not be able to produce an index that looks whole.
	first := testEnvelope(t, 1)
	lcm := ledger(100, []xdr.TransactionEnvelope{first}, []xdr.Hash{txHash(t, first)})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	txs, ok := AllTransactions(ctx, "test", testPassphrase, lcm)

	assert.False(t, ok)
	assert.Empty(t, txs)
}

func TestAllTransactionsReportsAnUnopenableReader(t *testing.T) {
	// Protocol version below 10 with fee processing present is the meta
	// version the reader rejects outright.
	lcm := xdr.LedgerCloseMeta{
		V: 0,
		V0: &xdr.LedgerCloseMetaV0{
			LedgerHeader: xdr.LedgerHeaderHistoryEntry{
				Header: xdr.LedgerHeader{LedgerSeq: 100, LedgerVersion: 9},
			},
			TxSet: xdr.TransactionSet{Txs: []xdr.TransactionEnvelope{testEnvelope(t, 1)}},
			TxProcessing: []xdr.TransactionResultMeta{{
				Result:        xdr.TransactionResultPair{TransactionHash: txHash(t, testEnvelope(t, 1))},
				FeeProcessing: xdr.LedgerEntryChanges{{Type: xdr.LedgerEntryChangeTypeLedgerEntryState}},
				TxApplyProcessing: xdr.TransactionMeta{
					V: 1, V1: &xdr.TransactionMetaV1{},
				},
			}},
		},
	}

	ctx, reports := capture(t)

	txs, ok := AllTransactions(ctx, "test", testPassphrase, lcm)

	assert.False(t, ok)
	assert.Empty(t, txs)
	require.Len(t, *reports, 1)
	assert.Contains(t, (*reports)[0].Err.Error(), "open transaction reader")
}
