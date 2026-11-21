# Fetch ledgers

`nebu fetch` is the one source stage: it pulls a ledger range from Stellar RPC
or from GCS/S3 archive storage and writes raw ledger XDR to stdout or a file.
Users fetch once and process many ways, so a fetch that quietly returns part
of a range is worse than one that fails — the exit code is part of the
contract (`docs/EXIT_CODES.md`).

## Sub-features

- `fetch-rpc-bounded` fetches an explicit range from RPC.
- `fetch-archive` fetches the same range from the public S3 archive.
- `fetch-last` resolves `--last N` to a concrete range and reports which one.
- `fetch-output` writes to a file instead of stdout with `--output`.
- `fetch-pipe` pipes XDR straight into an origin processor.
- `fetch-incomplete` fails rather than returning a short range.
- `fetch-usage` refuses an invalid mode or range before any network call.
- `fetch-follow` streams continuously from the tip (`--follow`); drive with
  `nebu-verify bg`, and only when asked.

## How to get to it (user POV)

- `nebu fetch 60200000 60200100 > ledgers.xdr`
- `nebu fetch --last 100 | token-transfer`
- `nebu fetch --mode archive --datastore-type S3 --bucket-path "aws-public-blockchain/v1.1/stellar/ledgers/pubnet" --region us-east-2 62080000 62080100`
- `nebu fetch --output ledgers.xdr 60200000 60200004`
- `nebu fetch 60200000 --follow`
- Via environment: `NEBU_MODE`, `NEBU_BUCKET_PATH`, `NEBU_RPC_URL`,
  `NEBU_RPC_AUTH`, `NEBU_LAST`, `NEBU_NETWORK`.

## Driving it with nebu-verify

Preconditions:

- `nebu-verify up --published` completed; `doctor` passes.
- Network access to the public S3 archive and to one Stellar RPC endpoint.
- `E="$PWD/.verify-artifacts/$(cat .verify-artifacts/current)"` and
  `mkdir -p "$E/out"`.

- **Fetch from the archive to a file.** Run `nebu-verify fixture 60200000 60200004`.
  Exit 0, and the printed path is a non-empty `.xdr` file — about 7 MB for
  five mainnet ledgers. An exit-0 fetch that produced an empty file is a
  failure; the helper fails the step for exactly that case.
- **Fetch from RPC, bounded.** Run
  `nebu-verify run fetch-rpc -- nebu fetch --output "$E/out/rpc-60200000-60200002.xdr" 60200000 60200002`.
  Exit 0 and the file is non-empty.
- **Compare the two sources on the same range.** Fetch the fixture range over
  RPC as well, then compare what a processor *decodes* from each. Run
  `nebu-verify sh fetch-sources-agree -- "nebu fetch --output '$E/out/rpc-5.xdr' 60200000 60200004 && token-transfer --quiet < '$E/out/rpc-5.xdr' > '$E/out/ev-rpc.jsonl' && token-transfer --quiet < '$F' > '$E/out/ev-archive.jsonl' && cmp '$E/out/ev-rpc.jsonl' '$E/out/ev-archive.jsonl' && echo events_identical=\$(wc -l < '$E/out/ev-rpc.jsonl')"`.
  `cmp` is silent, exit 0, and stdout reports the event count (observed:
  `events_identical=4041`). This is the independent truth check for both
  source paths — compare the decoded events, **not** the raw XDR; see the
  gotcha below.
- **Resolve `--last`.** Run
  `nebu-verify sh fetch-last -- "nebu fetch --last 2 --output '$E/out/last2.xdr' 2>&1 >/dev/null | grep 'Resolved --last 2 to ledgers'"`.
  Stdout holds the resolved range line; the `.out` file records which ledgers
  were chosen, which nothing else in the run records.
- **Pipe into a processor.** Run
  `nebu-verify sh fetch-pipe -- "nebu fetch 60200000 60200000 | token-transfer --quiet | jq -se 'length > 0 and all(.[]; ._schema == \"nebu.token_transfer.v1\")'"`.
  Stdout is `true`.
- **Reject an invalid mode before any network call.** Run
  `nebu-verify run fetch-bad-mode -- nebu fetch --mode bogus 60200000 60200001`.
  The `.exit` file reads `2` and stderr names the invalid mode.
- **Reject an inverted range.** Run
  `nebu-verify run fetch-bad-range -- nebu fetch 60200100 60200000`. The
  `.exit` file reads `3`.
- **Prove coverage has no holes.** Run
  `nebu-verify sh fetch-coverage -- "token-transfer --quiet < '$F' | duckdb -c \"COPY (WITH seen AS (SELECT DISTINCT meta.ledgerSequence AS seq FROM read_json('/dev/stdin')), b AS (SELECT MIN(seq) lo, MAX(seq) hi FROM seen) SELECT COUNT(*) missing FROM b, range(b.lo, b.hi + 1) g(seq) WHERE g.seq NOT IN (SELECT seq FROM seen)) TO '/dev/stdout' (FORMAT CSV, HEADER FALSE)\""`.
  Stdout is `0`.
- **Proof.** The fixture file, `fetch-rpc.*`, `fetch-sources-agree.*`,
  `fetch-last.out`, `fetch-coverage.out`, and the two refusal steps' `.exit`
  files. Together they show the range arrived whole, from both sources, and
  that bad input was refused without a network call.

## Gotchas

- **`nebu fetch` infers quiet mode when stdout is not a terminal.** Everything
  informational, including the `Resolved --last N to ledgers …` line, goes to
  stderr. A recipe that captures only stdout loses the only record of which
  ledgers a relative range picked.
- **The default RPC endpoint 429s on the third `getLedgers` call in a burst**,
  recovering about a second later (`scripts/docs-smoke.sh`). Space RPC steps
  out, and use the archive for everything that is not an assertion about RPC.
- **Archive mode off EC2 needs `AWS_EC2_METADATA_DISABLED=true`** or every
  process waits out the instance metadata service (about ten seconds) before
  the anonymous fallback a public bucket needs (`docs/ARCHIVE_MODE.md`). The
  helper exports it; a hand-run command will not have it.
- **A short range is a failure, not a result.** A bounded fetch that ends
  early returns `incomplete fetch: stream ended after N of M ledgers`
  (`cmd/nebu/fetch.go:850-859`). Never accept a partial `.xdr` because the
  command "worked".
- **`--output` and a shell redirect are not interchangeable in evidence.**
  `--output` writes a file nebu owns and can report on; `> file` makes an
  empty file even when the fetch fails. Prefer `--output` in recipes, and
  always test the file is non-empty.
- **Archive flags are a set.** `--mode archive` alone defaults
  `--datastore-type` to `GCS` and `--region` to `us-east-1`, which silently
  fails against the public S3 bucket. The helper keeps the four flags together
  for this reason.
- **`--follow` and `--end-ledger 0` never terminate.** Only drive them through
  `nebu-verify bg`, only when the task explicitly asks for a live stream, and
  take the run `down` afterwards so the PID is killed.
- **RPC and archive XDR are not byte-identical, even for a settled range, and
  that is expected.** It is a reordering of ledger entry changes, not a
  content difference — `docs/ARCHIVE_MODE.md`, "The two sources agree on
  content, not on byte order". Measured here on 60200000..60200004: both
  sources return exactly 7,322,200 bytes, the first ledger matches, then about
  807,000 bytes differ from offset 171924 on, while the decoded event stream is
  identical (4,041 events, byte-for-byte). A `cmp` on the XDR files therefore
  fails on a perfectly healthy pair of sources; compare what a processor emits.
- **Decoded output that differs in *count* is a different problem entirely.**
  Reordering preserves every event; a source missing the Galexie/RPC backfill
  parameters emits *less* — absent diagnostic events, `Ext.V` of 0, missing
  classic events (`docs/ARCHIVE_MODE.md`, "What the core flags do and do not
  explain"). So read the comparison two ways: `cmp` silent means the sources
  agree; equal counts with differing content, or unequal counts, means a
  misconfigured self-hosted archive or RPC and is a finding, not noise. The
  measurements above are against the AWS public archive and
  `archive-rpc.lightsail.network`; your own infrastructure needs its own run.
- Ledger 60200000-60200004 is mainnet (`pubnet`) and settled, which is why
  the decoded streams can be compared at all. Near the tip the two sources
  legitimately disagree on availability; a recent range cannot be compared
  this way.
