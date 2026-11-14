// Package checkpointentries streams the ledger entries held in a history
// archive checkpoint as newline-delimited JSON.
//
// It is nebu's answer to the Stellar ingest SDK's "Ledger Entry Statistics"
// example, which reads a checkpoint through ingest.CheckpointChangeReader and
// prints four totals: accounts, accounts holding trustlines, claimable
// balances and offers. Those four are useful, and they are also the only four
// that example can answer; computing a fifth means editing Go and reading
// forty million entries again.
//
// So this emits one event per entry instead of a tally, and the statistics
// become a query. The same run that produces the SDK example's numbers also
// answers which assets have the most trustlines, how balances distribute, or
// which accounts hold the most offers, because the output is a stream rather
// than a struct. The counting examples in examples/processors are the
// opposite shape on purpose: a per-ledger aggregate is small and a per-entry
// stream is not.
//
// A checkpoint is a full state snapshot, not a ledger range, so this is not a
// processor.Origin and does not go through nebu's origin CLI: there is no
// LedgerCloseMeta to process. It reads the bucket list the archive publishes
// every 64 ledgers.
//
// # Two bucket lists
//
// A checkpoint publishes two. The live bucket list holds the current state.
// The hot archive holds entries state archival has evicted: a persistent
// Soroban entry whose TTL lapsed leaves the live state and is recorded there,
// where it stays restorable. Reading only the live list and reporting "there
// are N contract data entries" is therefore a statement about what is live,
// not about what exists, and the difference grows with every eviction.
//
// The SDK exposes them through separate constructors — NewCheckpointChangeReader
// and NewHotArchiveIterator — with the same checkpoint and the same entry
// type, so one reader covers both and each event says which list it came
// from.
package checkpointentries

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/stellar/go-stellar-sdk/historyarchive"
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// SchemaID identifies the events this example emits.
const SchemaID = "nebu.checkpoint_entry.v1"

// Entry is one ledger entry from the checkpoint, flattened to the fields that
// are worth grouping by. Every optional field is omitted rather than emitted
// empty, so a reader can tell "this entry has no asset" from "this entry's
// asset is the empty string" — the distinction a filter needs.
type Entry struct {
	Schema           string `json:"_schema"`
	NebuVersion      string `json:"_nebu_version"`
	Network          string `json:"network"`
	CheckpointLedger uint32 `json:"checkpoint_ledger"`

	// BucketList is "live" or "hot": whether the entry is part of current
	// state or has been evicted by state archival. Every entry carries it, so
	// a file holding both is still analysable, and a count of one list cannot
	// be mistaken for a count of everything.
	BucketList string `json:"bucket_list"`

	EntryType          string `json:"entry_type"`
	LastModifiedLedger uint32 `json:"last_modified_ledger"`

	// AccountID is the account the entry belongs to: the account itself for an
	// account entry, the holder for a trustline, the seller for an offer, the
	// owner for a data entry. Absent for entries that belong to no account,
	// which is every Soroban entry type and the liquidity pools.
	AccountID string `json:"account_id,omitempty"`

	// Asset is canonical "CODE:ISSUER", or "native".
	Asset string `json:"asset,omitempty"`

	// Balance is in stroops: the account's balance, or the trustline's.
	Balance int64 `json:"balance,omitempty"`

	OfferID     int64  `json:"offer_id,omitempty"`
	BuyingAsset string `json:"buying_asset,omitempty"`
	BalanceID   string `json:"balance_id,omitempty"`
	PoolID      string `json:"pool_id,omitempty"`
	ContractID  string `json:"contract_id,omitempty"`

	// NumSubEntries is an account's sub-entry count: trustlines, offers, data
	// entries and signers. The SDK example counts accounts-with-trustlines by
	// building a set of account ids seen on trustline entries; this field
	// makes the cheaper version of that question answerable from the account
	// entries alone.
	NumSubEntries uint32 `json:"num_sub_entries,omitempty"`
}

// Bucket list names, as --bucket-list accepts them and as the events report
// them.
const (
	BucketListLive = "live"
	BucketListHot  = "hot"
)

// Reader streams entries out of one checkpoint.
type Reader struct {
	archive          historyarchive.ArchiveInterface
	networkLabel     string
	nebuVersion      string
	checkpointLedger uint32
	bucketList       string

	// progress is the fraction of the bucket list read so far. The live
	// reader reports it; the hot archive iterator exposes no equivalent, so
	// on that path it stays zero and the progress line reports entries only.
	progress float64
}

// Result reports what a Stream call produced.
type Result struct {
	// Written is the number of entries written.
	Written uint64

	// Complete is true only if the read reached the end of the bucket list.
	//
	// It matters more than it looks. Each bucket's hash is checked after its
	// entries have been yielded, so a read that stops early — at a --limit,
	// or on cancellation — never reaches the check for the bucket it stopped
	// in. Those entries are real XDR from the archive, but nothing has
	// verified that the bucket they came from is the bucket the checkpoint
	// claims. Only a complete read is hash-verified end to end.
	Complete bool
}

// NewReader opens the archive pool for a network. checkpointLedger may be 0,
// in which case Stream resolves the archive's latest checkpoint. bucketList is
// [BucketListLive] or [BucketListHot].
func NewReader(archiveURLs []string, networkPassphrase, networkLabel, nebuVersion string, checkpointLedger uint32, bucketList string) (*Reader, error) {
	if bucketList != BucketListLive && bucketList != BucketListHot {
		return nil, fmt.Errorf("unknown bucket list %q: use %q or %q", bucketList, BucketListLive, BucketListHot)
	}

	archive, err := historyarchive.NewArchivePool(archiveURLs, historyarchive.ArchiveOptions{
		NetworkPassphrase: networkPassphrase,
	})
	if err != nil {
		return nil, fmt.Errorf("opening the history archive pool: %w", err)
	}

	return &Reader{
		archive:          archive,
		networkLabel:     networkLabel,
		nebuVersion:      nebuVersion,
		checkpointLedger: checkpointLedger,
		bucketList:       bucketList,
	}, nil
}

// CheckpointFrequency is how often the archive publishes a checkpoint, in
// ledgers. It matches historyarchive's default.
const CheckpointFrequency = 64

// ValidateCheckpoint reports whether sequence is a checkpoint ledger, and if
// not, names the two nearest ones.
//
// The archive only holds snapshots at these sequences; asking for a ledger in
// between gets a 404 that reads like a fetch failure rather than the mistake
// it is.
func ValidateCheckpoint(sequence uint32) error {
	if (sequence+1)%CheckpointFrequency == 0 {
		return nil
	}
	previous := sequence - (sequence+1)%CheckpointFrequency
	return fmt.Errorf(
		"ledger %d is not a checkpoint: the archive publishes one every %d ledgers, at "+
			"sequences where (n+1) mod %d == 0. The nearest are %d and %d",
		sequence, CheckpointFrequency, CheckpointFrequency, previous, previous+CheckpointFrequency)
}

// Checkpoint returns the checkpoint this reader will read, resolving the
// archive's latest when none was given.
//
// A checkpoint sequence is not any ledger: the archive publishes one every 64
// ledgers, at sequences one less than a multiple of 64. Asking for a ledger
// in between gets a 404 from the archive, so an unset sequence resolves to
// the most recent published one rather than to "now".
func (r *Reader) Checkpoint() (uint32, error) {
	if r.checkpointLedger != 0 {
		return r.checkpointLedger, nil
	}
	has, err := r.archive.GetRootHAS()
	if err != nil {
		return 0, fmt.Errorf("asking the archive for its latest checkpoint: %w", err)
	}
	return has.CurrentLedger, nil
}

// Stream writes every entry in the chosen bucket list to out as NDJSON,
// stopping early at limit entries when limit is non-zero. progress, when
// non-nil, is called with the fraction read so far and the running count.
//
// An error means the entries already written must be discarded, not merely
// that the output is short: a bucket whose hash check failed, or never ran,
// is not known to belong to this checkpoint. The caller gets the count
// reached so it can say how much to throw away.
func (r *Reader) Stream(ctx context.Context, out io.Writer, limit uint64, progress func(fraction float64, written uint64)) (Result, error) {
	checkpoint, err := r.Checkpoint()
	if err != nil {
		return Result{}, err
	}

	encoder := json.NewEncoder(out)
	var result Result

	// emit writes one entry and reports whether to keep going.
	emit := func(ledgerEntry xdr.LedgerEntry) (bool, error) {
		if err := encoder.Encode(r.entry(checkpoint, ledgerEntry)); err != nil {
			return false, fmt.Errorf("writing entry %d: %w", result.Written+1, err)
		}
		result.Written++
		if progress != nil && result.Written%100_000 == 0 {
			progress(r.progress, result.Written)
		}
		return limit == 0 || result.Written < limit, nil
	}

	if r.bucketList == BucketListHot {
		err = r.streamHot(ctx, checkpoint, emit, &result)
	} else {
		err = r.streamLive(ctx, checkpoint, emit, &result)
	}
	return result, err
}

// streamLive reads the live bucket list, which holds current state.
func (r *Reader) streamLive(ctx context.Context, checkpoint uint32, emit func(xdr.LedgerEntry) (bool, error), result *Result) error {
	reader, err := ingest.NewCheckpointChangeReader(ctx, r.archive, checkpoint)
	if err != nil {
		return fmt.Errorf("opening checkpoint %d: %w", checkpoint, err)
	}
	defer func() {
		// Close cancels the reader's bucket-list streaming goroutine. It
		// reports the cancellation it just caused, which is not a failure of
		// this function, so the error is dropped deliberately.
		_ = reader.Close()
	}()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		r.progress = reader.Progress()

		change, err := reader.Read()
		if err == io.EOF {
			// Only here is every bucket hash checked.
			result.Complete = true
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading entry %d of checkpoint %d: %w", result.Written+1, checkpoint, err)
		}

		// A checkpoint is a snapshot, so every change is the creation of an
		// entry and Post is always set. Skip anything else rather than
		// dereference nil.
		if change.Post == nil {
			continue
		}

		keepGoing, err := emit(*change.Post)
		if err != nil {
			return err
		}
		if !keepGoing {
			return nil
		}
	}
}

// streamHot reads the hot archive bucket list, which holds the entries state
// archival has evicted.
//
// The SDK exposes this one as an iterator rather than a reader, and it yields
// only the archived entries: the hot archive also records entries that have
// been restored to live state, and those exist to suppress the older archived
// version of the same key rather than to be read as archived themselves.
func (r *Reader) streamHot(ctx context.Context, checkpoint uint32, emit func(xdr.LedgerEntry) (bool, error), result *Result) error {
	var iterationErr error
	stoppedEarly := false

	for ledgerEntry, err := range ingest.NewHotArchiveIterator(ctx, r.archive, checkpoint) {
		if err != nil {
			iterationErr = fmt.Errorf("reading entry %d of checkpoint %d's hot archive: %w",
				result.Written+1, checkpoint, err)
			break
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			iterationErr = ctxErr
			break
		}

		keepGoing, emitErr := emit(ledgerEntry)
		if emitErr != nil {
			iterationErr = emitErr
			break
		}
		if !keepGoing {
			stoppedEarly = true
			break
		}
	}

	if iterationErr != nil {
		return iterationErr
	}
	// The iterator ran to exhaustion, which is where its hash checks happen.
	result.Complete = !stoppedEarly
	return nil
}

// entry flattens one ledger entry into the emitted shape.
func (r *Reader) entry(checkpoint uint32, ledgerEntry xdr.LedgerEntry) Entry {
	entry := Entry{
		Schema:             SchemaID,
		NebuVersion:        r.nebuVersion,
		Network:            r.networkLabel,
		CheckpointLedger:   checkpoint,
		BucketList:         r.bucketList,
		EntryType:          entryTypeName(ledgerEntry.Data.Type),
		LastModifiedLedger: uint32(ledgerEntry.LastModifiedLedgerSeq),
	}

	switch ledgerEntry.Data.Type {
	case xdr.LedgerEntryTypeAccount:
		account := ledgerEntry.Data.MustAccount()
		entry.AccountID = account.AccountId.Address()
		entry.Balance = int64(account.Balance)
		entry.NumSubEntries = uint32(account.NumSubEntries)

	case xdr.LedgerEntryTypeTrustline:
		trustline := ledgerEntry.Data.MustTrustLine()
		entry.AccountID = trustline.AccountId.Address()
		entry.Balance = int64(trustline.Balance)
		entry.Asset = trustlineAsset(trustline.Asset)

	case xdr.LedgerEntryTypeOffer:
		offer := ledgerEntry.Data.MustOffer()
		entry.AccountID = offer.SellerId.Address()
		entry.OfferID = int64(offer.OfferId)
		entry.Asset = assetName(offer.Selling)
		entry.BuyingAsset = assetName(offer.Buying)
		entry.Balance = int64(offer.Amount)

	case xdr.LedgerEntryTypeData:
		entry.AccountID = ledgerEntry.Data.MustData().AccountId.Address()

	case xdr.LedgerEntryTypeClaimableBalance:
		balance := ledgerEntry.Data.MustClaimableBalance()
		entry.Asset = assetName(balance.Asset)
		entry.Balance = int64(balance.Amount)
		if id, err := xdr.MarshalHex(balance.BalanceId); err == nil {
			entry.BalanceID = id
		}

	case xdr.LedgerEntryTypeLiquidityPool:
		pool := ledgerEntry.Data.MustLiquidityPool()
		entry.PoolID = xdr.Hash(pool.LiquidityPoolId).HexString()

	case xdr.LedgerEntryTypeContractData:
		// ScAddress.String covers every arm, muxed contracts included, so a
		// CAP-84 address does not come out empty.
		if address, err := ledgerEntry.Data.MustContractData().Contract.String(); err == nil {
			entry.ContractID = address
		}

	case xdr.LedgerEntryTypeContractCode:
		entry.ContractID = ledgerEntry.Data.MustContractCode().Hash.HexString()
	}

	return entry
}

// entryTypeName is the lower-case name of an entry type, as the schema
// reports it. The XDR enum stringifies as "LedgerEntryTypeAccount", which is
// not a value worth grouping by in a query.
func entryTypeName(entryType xdr.LedgerEntryType) string {
	switch entryType {
	case xdr.LedgerEntryTypeAccount:
		return "account"
	case xdr.LedgerEntryTypeTrustline:
		return "trustline"
	case xdr.LedgerEntryTypeOffer:
		return "offer"
	case xdr.LedgerEntryTypeData:
		return "data"
	case xdr.LedgerEntryTypeClaimableBalance:
		return "claimable_balance"
	case xdr.LedgerEntryTypeLiquidityPool:
		return "liquidity_pool"
	case xdr.LedgerEntryTypeContractData:
		return "contract_data"
	case xdr.LedgerEntryTypeContractCode:
		return "contract_code"
	case xdr.LedgerEntryTypeConfigSetting:
		return "config_setting"
	case xdr.LedgerEntryTypeTtl:
		return "ttl"
	default:
		// A type this build does not know about. Report the number rather
		// than a guess, so an unknown type is visible in a GROUP BY instead
		// of silently joining another bucket.
		return fmt.Sprintf("unknown_%d", int32(entryType))
	}
}

// assetName renders an asset as "native" or "CODE:ISSUER".
func assetName(asset xdr.Asset) string {
	if asset.Type == xdr.AssetTypeAssetTypeNative {
		return "native"
	}
	code, issuer := "", ""
	switch asset.Type {
	case xdr.AssetTypeAssetTypeCreditAlphanum4:
		alphaNum4 := asset.MustAlphaNum4()
		code = strings.TrimRight(string(alphaNum4.AssetCode[:]), "\x00")
		issuer = alphaNum4.Issuer.Address()
	case xdr.AssetTypeAssetTypeCreditAlphanum12:
		alphaNum12 := asset.MustAlphaNum12()
		code = strings.TrimRight(string(alphaNum12.AssetCode[:]), "\x00")
		issuer = alphaNum12.Issuer.Address()
	default:
		return ""
	}
	return code + ":" + issuer
}

// trustlineAsset renders a trustline's asset, which may be a pool share
// rather than a credit asset.
func trustlineAsset(asset xdr.TrustLineAsset) string {
	if asset.Type == xdr.AssetTypeAssetTypePoolShare {
		if asset.LiquidityPoolId == nil {
			return "pool_share"
		}
		return "pool_share:" + xdr.Hash(*asset.LiquidityPoolId).HexString()
	}
	return assetName(asset.ToAsset())
}

// ReportProgress writes a progress line to stderr. Data goes to stdout and
// everything else to stderr, so the stream stays pipeable.
//
// fraction is zero on the hot archive path, which reports no progress, so the
// line falls back to the count alone rather than claiming 0%.
func ReportProgress(fraction float64, written uint64) {
	if fraction <= 0 {
		fmt.Fprintf(os.Stderr, "\r%d entries written", written)
		return
	}
	fmt.Fprintf(os.Stderr, "\rRead %.1f%% of the bucket list, %d entries written", fraction*100, written)
}
