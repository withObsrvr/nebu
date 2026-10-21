# Protocol 30 ("Bell") impact review

**Status:** Draft — needs owner sign-off before the testnet vote
**Protocol:** 30 ("Bell") · Stellar Core v30.0.0, released 2026-10-16
**CAPs:** [CAP-84](https://github.com/stellar/stellar-protocol/blob/master/core/cap-0084.md) muxed contract addresses · [CAP-87](https://github.com/stellar/stellar-protocol/blob/master/core/cap-0087.md) ML-DSA host functions · [CAP-88](https://github.com/stellar/stellar-protocol/blob/master/core/cap-0088.md) millisecond close times
**Source:** [Bell upgrade guide](https://stellar.org/blog/developers/bell-protocol-30-upgrade-guide)
**Owner:** Tillman Mosley III · **Written:** 2026-10-21

Required by **STL-2** of the OBSRVR Engineering Standard: "Each Stellar protocol
upgrade MUST get an impact review before the network votes. The review lists the
processors, datasets, decoders and validator changes affected, with the plan and
test ledgers for each. The validator vote itself follows OBS-7."

## Vote schedule and gates

| Gate | Date | Days from 2026-10-21 |
|---|---|---|
| Stellar Core v30.0.0 released | 2026-10-16 | done |
| Horizon, RPC, Galexie, CLI available | 2026-10-23 | 2 |
| **Testnet vote** | **2026-11-03 17:00 UTC** | **13** |
| **Pubnet vote** | **2026-11-23 17:00 UTC** | **33** |

Current state, measured 2026-10-21: **pubnet protocol 29** (ledger 65,059,209),
**testnet protocol 29** (ledger 5,311,654). No Protocol 30 ledger exists yet on
either network, which bounds what can be tested before 2026-11-03 (see
[Test ledgers](#test-ledgers)).

## Summary

| CAP | Severity for OBSRVR | Nature |
|---|---|---|
| **CAP-88** millisecond close times | **High** | Hard decode failure on old SDKs, plus a silent correctness problem that an SDK bump does not fix |
| **CAP-84** muxed contract addresses | **Medium** | New address variant and strkey prefix; affects anything that parses, stores or displays addresses |
| **CAP-87** ML-DSA host functions | **None** | Additive, opt-in; no effect on read-only pipelines |

The decoder half of CAP-88 is mechanical and already done for nebu. The
**semantic half is the real work**, it is not solved by a version bump, and it
has one decision that should be made before the testnet vote rather than after.

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

### Decision required

| Option | Change class (VER-3) | Notes |
|---|---|---|
| **A. Add a millisecond field** beside `closed_at_unix` | Additive — minor release | Existing consumers keep working; `time-window` and `aggregator` migrate to the new field deliberately. **Recommended.** |
| B. Change `closed_at_unix` to milliseconds | Breaking + **semantic** (VER-4) | Same wire type, different meaning — the worst kind of change to ship quietly. Needs an ADR, a schema-ID bump, a parity test and a replay decision (DATA-10). |
| C. Do nothing | — | Silent mis-bucketing in `time-window` and `aggregator` from the first same-second ledger pair. Not acceptable under P-1. |

Option A is recommended, and the window to choose it cheaply closes at the
pubnet vote. This is a **registry-owned schema decision** and needs its own ADR
in that repo. DATA-3 of the standard also wants RFC 3339 in UTC rather than an
epoch integer; if the field is being revisited anyway, that is the moment to fix
both — tracked as backlog item 2 of
[nebu's engineering profile](engineering-profile.md).

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

STL-2 requires test ledgers per affected item. Protocol 30 ledgers do not exist
yet, so this splits into what can be tested now and what is gated on the vote.

### Available now — regression only

| Purpose | Network | Range | Status |
|---|---|---|---|
| Decoder and pipeline regression | pubnet | 60200000-60200050 | **Passing.** 39,652 token-transfer events; `fetch` output byte-identical pre/post SDK bump |
| Protocol-29 tip reference | pubnet | 65,059,209 | Recorded 2026-10-21 |
| Protocol-29 tip reference | testnet | 5,311,654 | Recorded 2026-10-21 |

### Gated on the testnet vote (2026-11-03)

Selection criteria rather than fixed numbers, since the ledgers do not exist.
`LedgerCloseMeta.ProtocolVersion()` (SDK `xdr/ledger_close_meta.go:41`) is the
discriminator.

| Purpose | How to select | Blocks |
|---|---|---|
| **First Protocol 30 ledger** | First testnet ledger where `ProtocolVersion() == 30`. Capture as a fixture (TEST-1) with the range recorded | Decoder verification for nebu and all 13 origins |
| **Same-second ledger pair** | Consecutive testnet ledgers `N`, `N+1` where `ScpValue` seconds are equal but `CloseTimeMs` differ | The CAP-88 decision; `time-window` and `aggregator` correctness |
| **Muxed contract address** | A testnet transaction with an `SC_ADDRESS_TYPE_MUXED_CONTRACT` `to` argument — likely needs crafting rather than waiting | The 9 strkey-parsing processors |
| **`EMPTY_TX_SET_MS` ledger** | A testnet ledger closing with no transactions under Protocol 30 (discriminant 4) | The second new union arm, which the first-P30-ledger fixture probably will not cover |

The last row is easy to miss: a fixture taken from a busy ledger exercises arm 3
only. Arm 4 needs an empty ledger, and an untested union arm is exactly where a
decoder breaks in production.

All four should be checked in as fixtures per TEST-1, which nebu currently has
none of — this upgrade is a natural forcing function for backlog item 5 of the
engineering profile.

## Systems needing owner assessment

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

1. **Now — merge [PR #49](https://github.com/withObsrvr/nebu/pull/49)** and cut
   a nebu release. It is the dependency for every registry module.
2. **Now — answer the owner-assessment table above.** It is the part of STL-2
   this draft cannot complete, and the validator row has its own lead time.
3. **Now — decide CAP-88 option A vs B** and write the ADR in the registry repo.
   Deciding before the testnet vote keeps option A available.
4. **By 2026-10-31 — bump the registry modules** to SDK v0.8.0 and the new nebu
   release; regression-test each against pubnet 60200000-60200050.
5. **2026-11-03, immediately post-vote — capture the four testnet fixtures** and
   run the decode verification. This is the first moment real verification is
   possible, and it leaves 20 days before the pubnet vote.
6. **By 2026-11-10 — audit the 9 strkey-parsing processors** against a crafted
   muxed-address transaction.
7. **By 2026-11-20 — validator fleet on Core v30.0.0** per OBS-7, with the vote
   recorded.
8. **Ride-along:** STL-1 requires each processor to name the CAP or SEP it
   implements in its schema metadata. These schemas are being edited anyway;
   naming CAP-84, CAP-87 and CAP-88 now is nearly free and closes backlog item 7.

## Open questions

- Does any OBSRVR dataset or API treat ledger close time as a unique or ordering
  key? If yes, CAP-88 is a correctness incident waiting on a same-second pair,
  not a formatting nit.
- Is the registry's `closed_at_unix` consumed outside the registry — by Lake,
  the console, or a customer pipeline? That determines whether option A's
  migration is internal or needs a published deprecation window (VER-6: 90 days
  and two minor releases, which does **not** fit before the pubnet vote).
- Do any archived Galexie or Lake partitions need re-reading with the new SDK,
  or is archived XDR only ever read forward?
