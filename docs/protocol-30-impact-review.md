# Protocol 30 ("Bell") impact review

**Status:** **Accepted for nebu and the processor registry** (2026-11-14). The
[other-systems assessment](#systems-needing-owner-assessment) is **deferred by
owner decision** until nebu is complete, so STL-2 remains unmet for those rows
and this review is not yet complete as a whole.
**Protocol:** 30 ("Bell") · Stellar Core v30.0.0, released 2026-10-16
**CAPs:** [CAP-84](https://github.com/stellar/stellar-protocol/blob/master/core/cap-0084.md) muxed contract addresses · [CAP-87](https://github.com/stellar/stellar-protocol/blob/master/core/cap-0087.md) ML-DSA host functions · [CAP-88](https://github.com/stellar/stellar-protocol/blob/master/core/cap-0088.md) millisecond close times
**Source:** [Bell upgrade guide](https://stellar.org/blog/developers/bell-protocol-30-upgrade-guide)
**Owner:** Tillman Mosley III · **Written:** 2026-10-21 · **Updated:** 2026-11-14

Required by **STL-2** of the OBSRVR Engineering Standard: "Each Stellar protocol
upgrade MUST get an impact review before the network votes. The review lists the
processors, datasets, decoders and validator changes affected, with the plan and
test ledgers for each. The validator vote itself follows OBS-7."

## Vote schedule and gates

| Gate | Date | Days from 2026-11-14 |
|---|---|---|
| Stellar Core v30.0.0 released | 2026-10-16 | done |
| Horizon, RPC, Galexie, CLI available | 2026-10-23 | done |
| Testnet vote | 2026-11-03 17:00 UTC | **done — testnet is on protocol 30** |
| **Pubnet vote** | **2026-11-23 17:00 UTC** | **9** |

Current state, measured 2026-11-14 through each network's RPC `getLatestLedger`:

| Network | Ledger | `protocolVersion` |
|---|---|---|
| pubnet | 65,475,248 | **29** |
| testnet | 5,727,728 | **30** |

Protocol 30 ledgers now exist, which changes what this review can assert: the
rows that were gated on the testnet vote are measured below rather than
predicted. Pubnet is still protocol 29, and its vote is 9 days out.

## Summary

| CAP | Severity for OBSRVR | Nature |
|---|---|---|
| **CAP-88** millisecond close times | **High** | Hard decode failure on old SDKs, plus a silent correctness problem that an SDK bump does not fix |
| **CAP-84** muxed contract addresses | **Medium** | New address variant and strkey prefix; affects anything that parses, stores or displays addresses |
| **CAP-87** ML-DSA host functions | **None** | Additive, opt-in; no effect on read-only pipelines |

The decoder half of CAP-88 is mechanical and already done for nebu. The
**semantic half is the real work** and is not solved by a version bump. Its one
open decision — how to carry millisecond close times — was resolved on
2026-10-21 in favour of **Option A, an additive millisecond field**; see
[the decision](#decision-option-a--add-a-millisecond-field).

As of 2026-11-14 Option A is **implemented in nebu and adopted by 12 of the
registry's 14 origin processors**, and the two that have not adopted it are
named below. One of them was `token-transfer`, whose millisecond field was
present but **derived from a truncated value** — the most consequential finding
in this update, the kind an adoption count alone would have hidden, and fixed
the same day it was found.

## Method, and what this review does not cover

Findings for **nebu** and the **processor registry** are verified against source
and against real mainnet data (ledgers 60200000-60200050, 39,652 token-transfer
events). Registry findings come from code search across
`withObsrvr/nebu-processor-registry`; counts are stated so they can be rechecked.

Findings for **Obsrvr Lake, stellar-query-api, obsrvr-gateway, Prism, Radar,
obsrvr-console, flowctl, stellar-ingest, cdp-pipeline-workflow and the validator
fleet** are **not assessed here** — they are listed in
[Systems needing owner assessment](#systems-needing-owner-assessment) with the
specific question each must answer. This review is incomplete until those are
filled in; STL-2 requires the whole list.

## 1. Decoders affected

The mechanism is precise. `StellarValueExt` is a union discriminated by
`StellarValueType`:

| | SDK v0.7.3 and earlier | SDK v0.8.0 |
|---|---|---|
| Union arms | `LcValueSignature`, `ProposedValue` | + `SignedMsValue`, `ProposedMsValue` |
| `StellarValueType` | 0, 1, 2 | + **3** `SIGNED_MS`, **4** `EMPTY_TX_SET_MS` |

A Protocol 30 ledger carries discriminant 3 or 4. An SDK that has no arm for it
fails on `xdr.Unmarshal` of any `LedgerCloseMeta`. This is why the upgrade guide
says the SDK bump "is not optional this cycle."

`StellarValue` itself is **unchanged** — `CloseTime TimePoint`, still whole
seconds. The millisecond value lives in the new ext arms. That is why
`xdr.LedgerCloseMeta` appearing in nebu's stable surfaces causes no API drift.

### nebu — 5 decode paths

`pkg/processor/cli/origin.go:271`, `generic_origin.go:278`,
`proto_origin.go:263`, `cmd/nebu/stdin.go:27`, `cmd/compare_xdr.go:24`.

- **Plan:** upgrade `go-stellar-sdk` v0.7.3 → v0.8.0. Done in
  [PR #49](https://github.com/withObsrvr/nebu/pull/49). No source changes were
  required; the committed `.api/` snapshots are unchanged. `examples/hooks`
  moves v0.7.1 → v0.8.0 in the same change.
- **Verified:** `fetch` output byte-identical on the regression range; a
  39,652-event pipeline unchanged; nebu's own decoder correct on real XDR;
  `just ci`, `govulncheck`, `docs-smoke` and 9/9 integration tests pass.
- **Test ledgers:** regression on pubnet 60200000-60200050 (passed); Protocol 30
  verification is blocked until the testnet vote — see below.

### nebu `fetch` — not exposed

`writeRawLedgersWithCheckpoint` (`cmd/nebu/fetch.go:440`) iterates
`stream.RawLedgers()` yielding `[]byte` and derives sequences arithmetically. It
never decodes, and `TestWriteRawLedgersDoesNotDecodeRecords` asserts it by
pushing `{0xde, 0xad}` through successfully. Raw archival would have survived
the protocol change on the old SDK; only the decode paths would not.

**Consequence worth noting:** XDR already archived by `nebu fetch` is unaffected
and needs no migration. Anything that *re-reads* that XDR needs the new SDK.

### Processor registry — 13 origin decoders

`account-effects`, `contract-created`, `contract-events`,
`contract-invocation`, `contract-state`, `ledger-change-stats`,
`liquidity-pool-events`, `nft-events`, `soroban-tx-resources`,
`token-transfer`, `trade-extractor`, `ttl-tracker`, `validator-analytics`.

Plus nebu's two in-repo educational processors, `transaction-stats` and
`ledger-change-stats`, which build from this repo and move with PR #49.

- **Plan:** bump `go-stellar-sdk` to v0.8.0 across the registry's Go modules.
  The v0.6.11 changelog records the Protocol 28 precedent — 34 modules — so
  treat this as a repo-wide sweep, not 13 targeted edits. Modules also pin nebu
  itself; take the nebu release that carries PR #49 at the same time.
- **Gate:** every module must be released before **2026-11-03** for testnet
  users and before **2026-11-23** for pubnet users. A processor left on v0.7.x
  does not degrade — it stops decoding entirely.
- **Test ledgers:** regression on pubnet 60200000-60200050 per processor,
  comparing event output byte-for-byte against the pre-bump build; then the
  Protocol 30 ledgers below.

## 2. CAP-88's semantic problem — the decision this review exists to force

Upgrading the SDK makes decoding work. It does **not** make timestamps correct.

Under CAP-88, consecutive ledgers may close **within the same whole second**.
Any consumer that uses whole-second close time for ordering, windowing, or
deduplication becomes ambiguous — and fails silently, producing wrong buckets
rather than an error.

Confirmed exposure:

- The registry's protos define **`int64 closed_at_unix`** — Unix **seconds** —
  and 15 processors define or read it: `account-effects`, `aggregator`,
  `contract-created`, `contract-invocation`, `contract-state`,
  `json-file-sink`, `ledger-change-stats`, `liquidity-pool-events`,
  `nft-events`, `soroban-tx-resources`, `swap-candidate`, `time-window`,
  `token-transfer`, `ttl-tracker`, `validator-analytics`.
- Two of them **filter on it**: `time-window` ("requires meta.closedAtUnix so
  filtering uses real ledger close time") and `aggregator`. These are the ones
  that will mis-bucket.
- The SDK's own convenience accessor truncates:
  `LedgerCloseMeta.ClosedAt()` is `time.Unix(LedgerCloseTime(), 0)`, and
  `LedgerCloseTime()` reads `ScpValue.CloseTime` in seconds. Reaching the
  millisecond value means reading `ScpValue.Ext.SignedMsValue.CloseTimeMs`
  (`TimePointMs`).

### Decision: Option A — add a millisecond field

**Decided 2026-10-21.** A new millisecond field is added beside
`closed_at_unix`; the existing field keeps its meaning and its units.

| Option | Change class (VER-3) | |
|---|---|---|
| **A. Add a millisecond field** beside `closed_at_unix` | Additive — minor release | **Chosen.** Existing consumers keep working; `time-window` and `aggregator` migrate to the new field deliberately |
| B. Change `closed_at_unix` to milliseconds | Breaking + **semantic** (VER-4) | Rejected. Same wire type, different meaning. VER-6 requires 90 days *and* two minor releases, which lands in late January — two months past the pubnet vote, so B cannot be done compliantly in the time available |
| C. Do nothing | — | Rejected. Silent mis-bucketing in `time-window` and `aggregator` from the first same-second ledger pair. Not acceptable under P-1 |

Grounding for the premise: across pubnet ledgers 60200000-60200060, all 61 have
distinct whole-second close times, spaced 5-7 seconds apart, with zero
collisions. `closed_at_unix` is **lossless for every ledger that exists today**,
which is why A is purely forward-looking — there is no historical ambiguity to
remediate, provided it lands before the pubnet vote.

This decision needs an ADR in the processor registry, which owns the schema.
DATA-3 of the standard also wants RFC 3339 in UTC rather than an epoch integer;
if the field is being revisited, that is the moment to fix both — tracked as
backlog item 2 of [nebu's engineering profile](engineering-profile.md).

### Implementing Option A: the union arm that will be missed

Reading a millisecond close time is not a one-liner, and the obvious
implementation is wrong. The SDK offers **no** helper —
`LedgerCloseMeta.ClosedAt()` truncates to seconds via
`time.Unix(LedgerCloseTime(), 0)`. The value must be read out of the
`StellarValueExt` union, whose arms map like this
(`xdr_generated.go:12693`):

| `StellarValueType` | Arm | Carries `CloseTimeMs`? |
|---|---|---|
| 0 `BASIC` | (void) | no |
| 1 `SIGNED` | `LcValueSignature` | no |
| 2 `EMPTY_TX_SET` | `ProposedValue` | no |
| **3 `SIGNED_MS`** | `SignedMsValue` | **yes** |
| **4 `EMPTY_TX_SET_MS`** | `ProposedMsValue` | **yes** |

**Two arms carry the millisecond value, not one.** An implementation that
reaches for `Ext.GetSignedMsValue()` alone — the natural first guess — silently
falls back to seconds on every empty Protocol 30 ledger, which is exactly the
case a fixture taken from a busy ledger never exercises. This is the same defect
as the missing `EMPTY_TX_SET_MS` test ledger, seen from the code side rather
than the fixture side.

Correct shape, with the pre-Protocol-30 fallback:

```go
scp := ledger.LedgerHeaderHistoryEntry().Header.ScpValue
if v, ok := scp.Ext.GetSignedMsValue(); ok {
    return uint64(v.CloseTimeMs)        // arm 3
}
if v, ok := scp.Ext.GetProposedMsValue(); ok {
    return uint64(v.CloseTimeMs)        // arm 4 — do not omit
}
return uint64(scp.CloseTime) * 1000     // arms 0-2: pre-P30, seconds
```

Thirteen origin processors need this. Thirteen independent copies of a
five-branch union read, where one branch is easy to miss and its absence is
silent, is the wrong shape. It belongs in one shared helper that nebu owns —
nebu already owns the processor contract — with a table test over all five
discriminants.

**Resolved, and shipped.** The helper is `processor.LedgerCloseTimeMs`, in
`pkg/processor`, which `docs/STABILITY.md` makes permanent. That was the right
side of the open question: a non-stable package would have had every external
processor importing something nebu could move, for a read whose shape is fixed
by the XDR union and will not change. Its table test covers all five
discriminants, so dropping arm 4 fails the build rather than quietly reverting
to seconds.

Rendering is shared too, as of 2026-11-14. Three processors had each solved it
separately — `"2006-01-02T15:04:05.000Z07:00"` in one, `"...05.000Z"` in
another, and `time.RFC3339Nano` is the tempting third answer and the wrong one,
because it drops trailing zeros and the field's shape then varies with its
value. `processor.LedgerCloseTimeRFC3339` and `processor.FormatCloseTimeRFC3339`
render it one way, three digits always, which is the form DATA-3 asks for.

### Verified against real Protocol 30 ledgers

The helper could not be checked against real data until testnet voted. It can
now. Measured 2026-11-14 over **400 consecutive testnet ledgers, 5727300 to
5727699**, all `ProtocolVersion() == 30`:

| What | Result |
|---|---|
| `LedgerCloseTimeMs` returns true milliseconds | **Yes.** Ledger 5727700 closes at `1794662144916`, where the whole-second value is `1794662144` — a real 916 ms remainder, not seconds × 1000 |
| Millisecond remainders vary across ledgers | **Yes.** 901–905 ms across the 400-ledger window, so the value carries genuine sub-second information |
| `EMPTY_TX_SET_MS` (arm 4) ledgers | **None.** 400 of 400 are arm 3 `SIGNED_MS`; every ledger carried transactions, 11–25 of them. The arm is covered by unit test only |
| Same-second ledger pairs | **None.** Every gap is exactly 5 seconds, 399 of 399 |

Two of those rows matter for the plan.

**The arm-4 fixture still does not exist**, which is exactly what this review
predicted from the code side: a fixture taken from a busy ledger exercises arm 3
only, and testnet has not produced an empty Protocol 30 ledger in the sampled
window. The unit test is the only thing covering it.

**The same-second pair still does not exist either.** Testnet closes at a steady
5-second cadence, so CAP-88's ambiguity is permitted but not yet observable.
That strengthens rather than weakens Option A: it remains purely
forward-looking, there is still no historical ambiguity to remediate, and the
window to land it before pubnet is 9 days. It also means the correctness of
`time-window` and `aggregator` cannot be demonstrated failing yet — the first
same-second pair is when they start mis-bucketing, and it will arrive without
an error.

### Adoption, measured

| | Count | Which |
|---|---|---|
| Origins reading `processor.LedgerCloseTimeMs` | **12 of 14** | `account-effects`, `contract-created`, `contract-events`, `contract-invocation`, `contract-state`, `ledger-change-stats`, `liquidity-pool-events`, `nft-events`, `soroban-tx-resources`, `trade-extractor`, `ttl-tracker`, `validator-analytics` |
| Processors still calling the truncating SDK accessors (`LedgerCloseTime()`, `ClosedAt()`) | **0** | — |
| Origins not reading it | **2** | `token-transfer`, `soroswap-pool-indexer` |

Neither of the two is an oversight, and only one of them is fine.

**`soroswap-pool-indexer` has no ledger to read.** It queries the getEvents RPC
rather than streaming `LedgerCloseMeta`, so `LedgerCloseTimeMs` does not apply.
Its timestamp comes from the RPC's `LedgerClosedAt` string, parsed as RFC 3339
and reduced to `.Unix()` — seconds. If that string carries milliseconds, they
are being discarded at the parse; if it does not, the RPC is the one truncating.
Worth one measurement before the pubnet vote.

**`token-transfer`'s millisecond field is derived from a truncated value.** This
is the finding that an adoption count hides, because the field is present and
correctly shaped:

```
$ token-transfer --rpc-url https://soroban-testnet.stellar.org --network testnet \
    --start-ledger 5727700 --end-ledger 5727702 -q | jq -c .meta
{"closedAtUnix":"1794662144","closedAt":"2026-11-14T13:15:44.000Z"}
```

The true close time of ledger 5727700 is `1794662144916`. The emitted
`closed_at` says `.000`.

The cause is upstream of the registry. The SDK's own token transfer processor
builds its event meta with

```go
ClosedAt: timestamppb.New(tx.Ledger.ClosedAt()),   // processors/token_transfer/token_transfer_event.go:100
```

and `LedgerCloseMeta.ClosedAt()` is `time.Unix(LedgerCloseTime(), 0)` — whole
seconds. The registry's processor then formats that already-truncated timestamp
faithfully. So **anything built on `token_transfer.EventsFromLedger` reports
second-resolution close times on Protocol 30**, regardless of its own care, and
the `.000` suffix makes the loss look like a precise answer.

That is worse than a missing field: a consumer that migrates from
`closed_at_unix` to `closed_at`, exactly as the proto comment tells it to, gets
the same ambiguity it was migrating away from, now wearing three decimal places.

**Fixed 2026-11-14.** The processor reads the close time once from the ledger it
is already holding in `ProcessLedger`, through
`processor.LedgerCloseTimeMs`, and hands it to every event built from that
ledger. Same ledger, same command, before and after:

```
before  {"closedAtUnix":"1794662144","closedAt":"2026-11-14T13:15:44.000Z"}
after   {"closedAtUnix":"1794662144","closedAt":"2026-11-14T13:15:44.916Z"}
```

`closed_at_unix` is byte-identical, so Option A's promise holds: existing
consumers see no change. A pre-Protocol-30 pubnet ledger is byte-identical in
both fields, checked against the old binary. The tests pass a deliberately
truncated timestamp in the SDK event meta, so a regression that goes back to
reading it fails rather than silently re-truncating.

The SDK itself is unchanged and still truncates, so this is a local workaround
for an upstream defect — see the last open question.

## 3. CAP-84 — muxed contract addresses

New `SCAddress` variant `SC_ADDRESS_TYPE_MUXED_CONTRACT`, combining a contract
ID with a 64-bit multiplexing ID, and a new **`W`** strkey prefix (SEP-23). The
guide: "Every application that validates, displays, stores or routes addresses
must handle the new variant and prefix." It appears in the Stellar Asset
Contract `transfer` function's `to` argument, in ledger metadata, and in
transfer and mint events.

Confirmed exposure in the registry:

- **9 processors parse strkey:** `contract-created`, `contract-events`,
  `contract-invocation`, `contract-state`, `nft-events`,
  `soroswap-pool-indexer`, `soroswap-pool-transform`, `trade-extractor`,
  `validator-identity-enricher`.
- **5 handle `ScAddress` directly:** `contract-created`, `contract-events`,
  `contract-invocation`, `nft-events`, `soroswap-pool-indexer`.
- **No processor references `ToMuxedInfo` or `MuxedAccount`** (0 hits). The
  guide's advice to "use existing `to_muxed_id` field" is therefore
  unimplemented everywhere, even though the SDK's `EventMeta` has carried
  `ToMuxedInfo` since CAP-67.

nebu itself passes SDK-produced address strings through without re-encoding, so
it needs no change — and DATA-4 requires it stays that way: "Stellar
identifiers ... MUST be used as the network defines them, never re-encoded."

- **Plan:** audit the 9 strkey-parsing processors for assumptions that an
  address is 56 characters or starts with `G`/`C`. Decide per processor whether
  a muxed contract address is preserved, rejected with a clear error, or
  decomposed into contract ID + mux ID. Rejecting is acceptable; silently
  truncating or mangling is not.
- **Test ledgers:** requires a CAP-84 transaction, which will not exist until
  after the testnet vote and may need to be crafted deliberately. See below.

## 4. CAP-87 — ML-DSA host functions

Three new Soroban host functions (`verify_sig_ml_dsa_44/65/87`). Additive,
opt-in, no backwards incompatibilities. **No action** for read-only pipelines.
Relevant only if OBSRVR ships Soroban contracts, which would then need a
rebuild against the Protocol 30 Soroban SDK and a matching Stellar CLI.

## Test ledgers

STL-2 requires test ledgers per affected item. When this review was written no
Protocol 30 ledger existed; testnet voted on 2026-11-03, so three of the four
gated rows are now measurable and two of them have been measured.

### Available now — regression only

| Purpose | Network | Range | Status |
|---|---|---|---|
| Decoder and pipeline regression | pubnet | 60200000-60200050 | **Passing.** 39,652 token-transfer events; `fetch` output byte-identical pre/post SDK bump |
| Protocol-29 tip reference | pubnet | 65,059,209 | Recorded 2026-10-21 |
| Protocol-29 tip reference | testnet | 5,311,654 | Recorded 2026-10-21 (testnet is protocol 30 as of the vote) |
| Protocol-30 close-time verification | testnet | 5727300-5727699 | **Passing.** 400 ledgers, all arm 3; millisecond remainders 901-905; zero same-second pairs |
| Protocol-30 truncation evidence | testnet | 5727700 | True close `1794662144916`; `token-transfer` emits `...44.000Z` |

### Protocol 30 ledgers, since the testnet vote

| Purpose | Status 2026-11-14 | Blocks |
|---|---|---|
| **Protocol 30 ledgers** | **Available.** Testnet 5727300-5727699 verified `ProtocolVersion() == 30`; millisecond close times read correctly through `processor.LedgerCloseTimeMs` | Decoder verification — **done for nebu**, outstanding for the registry's origins as a set |
| **Same-second ledger pair** | **Does not exist yet.** 399 of 399 consecutive gaps in the sampled window are exactly 5 seconds | The correctness demonstration for `time-window` and `aggregator`. Their mis-bucketing cannot be reproduced until the network produces a pair |
| **`EMPTY_TX_SET_MS` ledger** | **Does not exist yet.** 400 of 400 sampled ledgers are arm 3 with 11-25 transactions each | Arm 4, which remains covered by unit test only — exactly the gap this review predicted |
| **Muxed contract address** | **Not found; needs crafting.** No CAP-84 address has been observed in the sampled window | The 9 strkey-parsing processors |

Two of these cannot be waited out. A same-second pair needs the network to close
two ledgers inside one second, which a steady 5-second cadence never does, and
an empty Protocol 30 ledger needs a quiet moment testnet has not had. Both are
**constructible** — a hand-built `LedgerCloseMeta` with the arm and the close
times set is what the unit test already does — and that is the honest answer for
the fixture set: capture the real ledgers for what the network produces, and
construct the two cases it does not. A constructed fixture that is labelled as
constructed is worth more than a gap labelled as a plan.

Fixtures should be checked in per TEST-1, which nebu still has none of — backlog
item 5 of the engineering profile, and this upgrade remains its natural forcing
function.

## Systems needing owner assessment

**Deferred by owner decision, 2026-11-14:** these rows are not being assessed
until nebu is complete. The deferral is recorded rather than left implicit
because it has a cost — STL-2 is unmet for every row below, and the pubnet vote
is 9 days away, so any of these that does decode `LedgerCloseMeta` or key on a
whole-second close time will meet Protocol 30 without a review. The validator
row carries its own lead time and is the one least able to absorb a late start.

This review is **not complete** until each row is answered. Listed per §13 of
the standard.

| System | Question it must answer |
|---|---|
| **Validator fleet** (obsrvr-ops) | Core v30.0.0 rollout plan. Per OBS-7: pre-checks, one node at a time with a health gate between nodes, and a tested rollback. Also the vote itself — how each validator's `PROTOCOL_VERSION` is set and recorded |
| **Obsrvr Lake** | Which Bronze/Silver tables carry whole-second close times, and whether a dataset version bump is needed (DATA-10). Does any table key or partition on close time? Historical and live transforms share one path (DATA-12), so both move together |
| **stellar-query-api** | Does any endpoint expose or filter on close time at second resolution? Any cached response keyed on it? |
| **flowctl** | Events carry ledger and cursor; do they carry close time, and at what resolution? Version-matrix implications for a rolling upgrade (VER-9) |
| **Prism / Radar** | Address display and validation for the `W` prefix (CAP-84). Contract pages showing SEP-46/55/58 provenance may surface muxed addresses |
| **obsrvr-gateway, obsrvr-console** | Any address input validation that would reject a `W` strkey |
| **stellar-ingest** | Its own XDR decode path and SDK pin — same CAP-88 decode failure applies |
| **cdp-pipeline-workflow** | Legacy, being replaced, but still running console pipelines. Does it decode `LedgerCloseMeta`? If so it needs the bump or a documented end-of-life before 2026-11-23 |

## Plan and sequencing

### Done

1. **nebu released on SDK v0.8.0.** The decode path shipped in v0.7.0 and the
   line has since reached v0.7.6.
2. **The shared millisecond helper exists**: `processor.LedgerCloseTimeMs` in
   `pkg/processor`, with a table test over all five union discriminants, plus
   `LedgerCloseTimeRFC3339` so the rendering is shared and not re-solved per
   processor.
3. **The registry is bumped**: all 36 modules on SDK v0.8.0 and nebu v0.7.5,
   each built, vetted and tested; regression-tested against pubnet
   60200000-60200050.
4. **Option A is adopted by 12 of 14 origins**, and no processor still calls the
   truncating SDK accessors.
5. **The helper is verified against real Protocol 30 ledgers** — 400 testnet
   ledgers, measured, with the limits of that sample stated rather than glossed.

6. **`token-transfer`'s close time is fixed** and verified against the
   Protocol 30 ledger that exposed it: `...44.000Z` became `...44.916Z` with
   `closed_at_unix` unchanged. The schema did not change, so no ID bump.

### Remaining before the pubnet vote, 9 days

1. **Measure `soroswap-pool-indexer`'s timestamp.** One check: does the
   getEvents RPC's `LedgerClosedAt` carry milliseconds? If it does, the
   processor is discarding them at the parse and the fix is local; if it does
   not, record that the RPC is the limit and that this processor's timestamps
   are second-resolution by construction.
2. **Report the SDK truncation upstream.** `processors/token_transfer` builds
   event meta from `Ledger.ClosedAt()`, so every consumer of
   `EventsFromLedger` gets seconds on Protocol 30. Worth a Stellar issue
   whether or not the registry works around it locally.
3. **Construct the two fixtures the network has not produced** — a same-second
   pair and an `EMPTY_TX_SET_MS` ledger — and label them as constructed
   (TEST-1).
4. **Audit the 9 strkey-parsing processors** against a crafted muxed-address
   transaction (CAP-84). Still outstanding; no CAP-84 address has appeared on
   testnet to test against.
5. **The owner-assessment rows**, deferred by decision above. The validator
   fleet row is the one with real lead time before 2026-11-23.

### Ride-along

STL-1 requires each processor to name the CAP or SEP it implements in its schema
metadata. Those schemas are being edited anyway; naming CAP-84, CAP-87 and
CAP-88 while they are open is nearly free and closes backlog item 7.

## Open questions

- Does any OBSRVR dataset or API treat ledger close time as a unique or ordering
  key? If yes, CAP-88 is a correctness incident waiting on a same-second pair,
  not a formatting nit. **Still open — and now narrower:** within nebu and the
  registry, two processors filter on close time (`time-window`, `aggregator`)
  and neither can mis-bucket until the network produces a same-second pair,
  which it has not.
- Is the registry's `closed_at_unix` consumed outside the registry — by Lake,
  the console, or a customer pipeline? That determines whether Option A's
  migration is internal or needs a published deprecation window (VER-6: 90 days
  and two minor releases, which does **not** fit before the pubnet vote).
  **Still open**, and part of the deferred assessment.
- Do any archived Galexie or Lake partitions need re-reading with the new SDK,
  or is archived XDR only ever read forward? **Still open.**
- **New:** how long does the SDK's truncation stay? If `EventsFromLedger`'s meta
  keeps carrying whole seconds, every downstream reader of the SDK's token
  transfer processor inherits it, and each one works around it separately. A
  local fix in the registry is correct regardless, but the upstream answer
  decides whether that workaround is permanent.
